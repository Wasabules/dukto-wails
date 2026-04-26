package dev.wasabules.dukto.transfer

import android.content.Context
import android.net.Uri
import android.os.Environment
import android.util.Log
import androidx.core.net.toUri
import androidx.documentfile.provider.DocumentFile
import dev.wasabules.dukto.policy.Decision
import dev.wasabules.dukto.policy.SessionPolicy
import dev.wasabules.dukto.protocol.DEFAULT_PORT
import dev.wasabules.dukto.protocol.DIRECTORY_SIZE_MARKER
import dev.wasabules.dukto.protocol.ElementHeader
import dev.wasabules.dukto.protocol.SessionHeader
import dev.wasabules.dukto.protocol.TEXT_ELEMENT_NAME
import dev.wasabules.dukto.protocol.readElementHeader
import dev.wasabules.dukto.protocol.readSessionHeader
import dev.wasabules.dukto.protocol.writeElementHeader
import dev.wasabules.dukto.protocol.writeSessionHeader
import kotlinx.coroutines.CoroutineExceptionHandler
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.BufferedInputStream
import java.io.BufferedOutputStream
import java.io.ByteArrayOutputStream
import java.io.File
import java.io.FileOutputStream
import java.io.InputStream
import java.io.OutputStream
import java.net.InetAddress
import java.net.ServerSocket
import java.net.Socket
import java.util.concurrent.atomic.AtomicReference
import kotlin.math.min

// ─────────────────────────────────────────────────────────────────────────────
// Public types
// ─────────────────────────────────────────────────────────────────────────────

sealed interface TransferEvent {
    data class Started(
        val from: InetAddress,
        val totalElements: Long,
        val totalSize: Long,
        val isReceive: Boolean,
    ) : TransferEvent
    data class Progress(
        val peer: InetAddress,
        val bytesDone: Long,
        val totalBytes: Long,
        val isReceive: Boolean,
    ) : TransferEvent
    data class TextReceived(val from: InetAddress, val text: String) : TransferEvent
    data class FilesReceived(
        val from: InetAddress,
        val rootDescription: String,
        val fileCount: Int,
        /**
         * URIs of received files (DocumentFile content URIs when using SAF,
         * file:// URIs otherwise). Used by the UI to render thumbnails and
         * to launch previews via ACTION_VIEW.
         */
        val fileUris: List<String>,
    ) : TransferEvent
    data class Failed(val from: InetAddress?, val reason: String, val isReceive: Boolean) : TransferEvent
    data class Sent(val to: Peer, val totalSize: Long) : TransferEvent
}

data class Peer(val address: InetAddress, val port: Int = DEFAULT_PORT)

/** Cancel handle returned by send / set on receive — closing it aborts the in-flight session. */
fun interface Cancellable {
    fun cancel()
}

// ─────────────────────────────────────────────────────────────────────────────
// Server (receive side)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * Accept loop on TCP port 4644. Each session runs in its own coroutine.
 *
 * Where files land:
 *  - If [destTreeUriProvider] returns a non-null SAF URI (an
 *    `OpenDocumentTree` result the user has granted persistent access to),
 *    files are written there via [DocumentFile] and visible to system file
 *    managers.
 *  - Otherwise, fallback to `getExternalFilesDir(DIRECTORY_DOWNLOADS)` —
 *    works without any runtime permission but only readable by us.
 *
 * The active receive socket (if any) is exposed via [activeReceive] so the UI
 * can cancel it.
 */
/**
 * Optional v2 tunnel hooks. When [v2Identity] is non-null the server peeks
 * the first 8 bytes of every accepted connection; if they match the v2
 * magic it runs a Noise XX responder handshake before reading the legacy
 * SessionHeader off the resulting Session. [onSessionMode] is fired
 * once per session so the UI can flag the audit/history entry as
 * encrypted vs cleartext.
 */
class Server(
    private val context: Context,
    private val port: Int = DEFAULT_PORT,
    private val destTreeUriProvider: () -> String? = { null },
    private val policy: SessionPolicy? = null,
    private val signatureLookup: (InetAddress) -> String? = { null },
    private val v2Identity: dev.wasabules.dukto.identity.Identity? = null,
    private val onSessionMode: (Boolean) -> Unit = {},
    /** Called once per inbound v2 handshake. Returns the one-shot PSK if
     *  a pairing is currently armed (XXpsk2 path), or null for plain XX.
     *  The returned PSK is consumed atomically by the implementation. */
    private val pendingPskProvider: () -> ByteArray? = { null },
    /** Called after a v2 handshake completes with the peer's X25519
     *  pubkey; non-null when the handshake used a PSK so the engine
     *  can auto-pin the peer's identity. */
    private val onPskHandshake: (InetAddress, ByteArray) -> Unit = { _, _ -> },
    /** Drop every legacy / unpaired v2 session when this returns true. */
    private val refuseCleartextProvider: () -> Boolean = { false },
    /** Whether the X25519 pubkey [first arg] corresponds to a pinned
     *  peer in our TOFU table. Used by the refuseCleartext gate. */
    private val isPubKeyPinned: (ByteArray) -> Boolean = { false },
    /** Called when an inbound v2 handshake produces a remote_static
     *  that doesn't match the X25519 derived from the peer's already-
     *  pinned Ed25519 fingerprint. Used to surface the UI alert. */
    private val onTofuMismatch: (InetAddress, oldFp: String, newFp: String) -> Unit = { _, _, _ -> },
    /** Look up an existing TOFU mismatch for [first arg]'s remote_static
     *  + the Ed25519 pubkey advertised over UDP. Returns the old and
     *  new fingerprints when they disagree, null otherwise. */
    private val tofuMismatchProvider: (InetAddress, ByteArray) -> Pair<String, String>? = { _, _ -> null },
) {
    // SupervisorJob: a session-level crash (e.g. an IOException escaping
    // upgradeIfV2 because RefuseCleartext rejected an unpaired peer)
    // must not cancel the parent scope and tear the listen-loop down.
    private val scope = CoroutineScope(Dispatchers.IO + SupervisorJob() + CoroutineExceptionHandler { _, t ->
        Log.w(TAG, "uncaught in Server scope: ${t.message}", t)
    })
    private val _events = MutableSharedFlow<TransferEvent>(extraBufferCapacity = 64)
    val events: SharedFlow<TransferEvent> = _events.asSharedFlow()

    private var serverSocket: ServerSocket? = null
    private var acceptJob: Job? = null

    /** Single-slot reference to the currently in-flight receive socket. */
    private val activeReceive = AtomicReference<Socket?>(null)

    /** Hook the UI calls to cancel the current receive (idempotent). */
    fun cancelActiveReceive() {
        activeReceive.get()?.let { runCatching { it.close() } }
    }

    fun start() {
        if (serverSocket != null) return
        val s = ServerSocket(port).apply { reuseAddress = true }
        serverSocket = s
        acceptJob = scope.launch { acceptLoop(s) }
    }

    fun stop() {
        runCatching { serverSocket?.close() }
        serverSocket = null
        acceptJob?.cancel()
        cancelActiveReceive()
        scope.cancel()
    }

    private suspend fun acceptLoop(s: ServerSocket) = withContext(Dispatchers.IO) {
        while (true) {
            val client: Socket = try {
                s.accept()
            } catch (e: Exception) {
                if (s.isClosed) return@withContext
                Log.w(TAG, "accept error: ${e.message}")
                continue
            }
            scope.launch { handleSession(client) }
        }
    }

    private suspend fun handleSession(client: Socket) = withContext(Dispatchers.IO) {
        val from = client.inetAddress
        activeReceive.compareAndSet(null, client)
        try {
            // Pre-session gate: master switch + block list + confirm-unknown.
            policy?.let { p ->
                val pre = p.preSession(from) { signatureLookup(from) }
                if (pre is Decision.Reject) {
                    _events.emit(TransferEvent.Failed(from, pre.reason, isReceive = true))
                    runCatching { client.close() }
                    return@withContext
                }
            }

            client.use { sock ->
                // Magic-prefix peek + optional Noise XX upgrade. When v2
                // is enabled and the prefix matches we replace the raw
                // streams with the Session's encrypted streams; otherwise
                // the peek is replayed and the legacy parser sees an
                // unmodified byte stream. The receiver doesn't write
                // back during a session, so we drop v2.output.
                val v2 = upgradeIfV2(sock)
                val input = BufferedInputStream(v2.input)
                onSessionMode(v2.encrypted)
                val header = readSessionHeader(input)
                // Stage 4: session size cap.
                policy?.checkSessionSize(from, header.totalSize)?.let {
                    if (it is Decision.Reject) {
                        _events.emit(TransferEvent.Failed(from, it.reason, isReceive = true))
                        return@use
                    }
                }
                _events.emit(TransferEvent.Started(from, header.totalElements, header.totalSize, isReceive = true))

                if (header.totalElements == 1L) {
                    val first = readElementHeader(input)
                    // Stage 5: extension check (single-element fast path).
                    if (!first.isText) {
                        policy?.checkElement(from, first.name)?.let {
                            if (it is Decision.Reject) {
                                _events.emit(TransferEvent.Failed(from, it.reason, isReceive = true))
                                return@use
                            }
                        }
                    }
                    if (first.isText) {
                        receiveText(input, from, first.size)
                        return@use
                    }
                    receiveFiles(input, from, header, first)
                    return@use
                }
                receiveFiles(input, from, header, firstElement = null)
            }
        } catch (t: Throwable) {
            // Catch Throwable (not just Exception) so Errors and stray
            // CancellationException-wrapped IOExceptions can't bubble out
            // and tear the parent scope down. tryEmit instead of emit so
            // a closed-channel race here can't itself throw.
            Log.w(TAG, "session from $from failed: ${t.message}", t)
            _events.tryEmit(
                TransferEvent.Failed(from, t.message ?: t.javaClass.simpleName, isReceive = true),
            )
            // Make sure the underlying socket is closed even if `client.use`
            // didn't get a chance to (e.g. an exception thrown before we
            // entered the use-block).
            runCatching { client.close() }
        } finally {
            activeReceive.compareAndSet(client, null)
        }
    }

    private suspend fun receiveText(input: InputStream, from: InetAddress, size: Long) {
        if (size < 0 || size > MAX_TEXT_BYTES) {
            throw IllegalStateException("text snippet size out of range: $size")
        }
        val out = ByteArrayOutputStream(size.toInt().coerceAtLeast(64))
        copyBytes(input, out, size) { done ->
            _events.tryEmit(TransferEvent.Progress(from, done, size, isReceive = true))
        }
        _events.emit(TransferEvent.TextReceived(from, out.toString(Charsets.UTF_8)))
    }

    private suspend fun receiveFiles(
        input: InputStream,
        from: InetAddress,
        header: SessionHeader,
        firstElement: ElementHeader?,
    ) {
        val sink: ReceiveSink = openSink(from)
        var fileCount = 0
        var bytesSoFar = 0L
        var elementsLeft = header.totalElements
        var firstEl = firstElement
        try {
            while (elementsLeft > 0L) {
                val el = firstEl ?: readElementHeader(input)
                firstEl = null
                elementsLeft--

                if (el.size == DIRECTORY_SIZE_MARKER) {
                    sink.mkdirs(el.name)
                } else {
                    // Stage 5: extension check, per element.
                    policy?.checkElement(from, el.name)?.let {
                        if (it is Decision.Reject) {
                            throw IllegalStateException(it.reason)
                        }
                    }
                    sink.writeFile(el.name, el.size) { stream ->
                        copyBytes(input, stream, el.size) { delta ->
                            val total = (bytesSoFar + delta).coerceAtMost(header.totalSize)
                            _events.tryEmit(TransferEvent.Progress(from, total, header.totalSize, isReceive = true))
                        }
                    }
                    bytesSoFar += el.size
                    fileCount++
                }
            }
        } finally {
            sink.close()
        }
        _events.emit(TransferEvent.FilesReceived(from, sink.description, fileCount, sink.uris.toList()))
    }

    // ── sinks: SAF tree if configured, else app-private external storage ──

    private fun openSink(from: InetAddress): ReceiveSink {
        val treeUriStr = destTreeUriProvider()
        val sessionLabel = "dukto-${System.currentTimeMillis()}-${from.hostAddress?.replace('.', '_')}"
        if (treeUriStr != null) {
            val tree = DocumentFile.fromTreeUri(context, treeUriStr.toUri())
            if (tree != null && tree.canWrite()) {
                val sessionDoc = tree.createDirectory(sessionLabel)
                    ?: throw IllegalStateException("cannot create session dir under $treeUriStr")
                return DocumentSink(context, sessionDoc, "${tree.uri} / $sessionLabel")
            }
            Log.w(TAG, "tree URI $treeUriStr not writable; falling back to private storage")
        }
        val privRoot = context.getExternalFilesDir(Environment.DIRECTORY_DOWNLOADS)
            ?: File(context.filesDir, "Downloads").apply { mkdirs() }
        val sessionDir = File(privRoot, sessionLabel)
        if (!sessionDir.mkdirs() && !sessionDir.isDirectory) {
            throw IllegalStateException("cannot create $sessionDir")
        }
        return FileSink(sessionDir)
    }

    /**
     * Holder for the (input, output, encrypted, remoteStatic) tuple
     * produced by [upgradeIfV2]. Mirrors the Go side's transfer.Server.Upgrade
     * return shape.
     */
    private data class StreamPair(
        val input: InputStream,
        val output: OutputStream,
        val encrypted: Boolean,
        val remoteStatic: ByteArray? = null,
    )

    private fun upgradeIfV2(sock: Socket): StreamPair {
        val refuse = refuseCleartextProvider()
        val identity = v2Identity
        if (identity == null) {
            if (refuse) throw java.io.IOException("refuseCleartext: no v2 identity loaded")
            return StreamPair(sock.getInputStream(), sock.getOutputStream(), encrypted = false)
        }
        val peeked = dev.wasabules.dukto.tunnel.peekMagic(sock.getInputStream())
        if (!peeked.isV2) {
            if (refuse) throw java.io.IOException("refuseCleartext: peer used legacy session header")
            return StreamPair(peeked.stream, sock.getOutputStream(), encrypted = false)
        }
        val psk = pendingPskProvider()
        val session = dev.wasabules.dukto.tunnel.handshake(
            peeked.stream, sock.getOutputStream(),
            dev.wasabules.dukto.tunnel.HandshakeRole.Responder,
            identity.x25519Private(), identity.x25519Public(),
            psk = psk,
            closer = {},
        )
        if (psk != null) {
            // PSK proves both ends know the same passphrase; safe to auto-
            // pin the peer's identity from this point on.
            onPskHandshake(sock.inetAddress, session.remoteStatic)
        } else {
            // TOFU mismatch detector: pinned peer's Ed25519 fingerprint
            // points to a different X25519 than the remote_static we
            // just received → kill the session and surface the alert.
            val mismatch = tofuMismatchProvider(sock.inetAddress, session.remoteStatic)
            if (mismatch != null) {
                runCatching { session.close() }
                onTofuMismatch(sock.inetAddress, mismatch.first, mismatch.second)
                throw java.io.IOException("v2 fingerprint mismatch: ${mismatch.second}")
            }
            if (refuse && !isPubKeyPinned(session.remoteStatic)) {
                runCatching { session.close() }
                throw java.io.IOException("refuseCleartext: peer not paired")
            }
        }
        return StreamPair(SessionInput(session), SessionOutput(session), encrypted = true, remoteStatic = session.remoteStatic)
    }

    private companion object {
        const val TAG = "DuktoServer"
        const val MAX_TEXT_BYTES = 8L * 1024L * 1024L
    }
}

/** Adapter exposing a [Session]'s read() loop as a java.io.InputStream. */
private class SessionInput(private val s: dev.wasabules.dukto.tunnel.Session) : InputStream() {
    private val one = ByteArray(1)
    override fun read(): Int {
        val n = s.read(one, 0, 1)
        return if (n <= 0) -1 else (one[0].toInt() and 0xFF)
    }
    override fun read(b: ByteArray, off: Int, len: Int): Int = s.read(b, off, len)
    override fun close() = s.close()
}

/** Adapter exposing a [Session]'s write() as a java.io.OutputStream. */
private class SessionOutput(private val s: dev.wasabules.dukto.tunnel.Session) : OutputStream() {
    override fun write(b: Int) {
        s.write(byteArrayOf((b and 0xFF).toByte()))
    }
    override fun write(b: ByteArray, off: Int, len: Int) {
        s.write(b, off, len)
    }
    override fun close() = s.close()
}

// ─────────────────────────────────────────────────────────────────────────────
// Receive sinks
// ─────────────────────────────────────────────────────────────────────────────

private interface ReceiveSink {
    /** Human-readable destination (path or tree URI) for activity logs. */
    val description: String

    /** URIs of files written so far — used by the UI to render previews. */
    val uris: List<String>

    /** Create directories along [relPath] (relative, '/' separators). Idempotent. */
    fun mkdirs(relPath: String)

    /** Open a write stream for the file at [relPath] (parent dirs created if needed). */
    fun writeFile(relPath: String, expectedSize: Long, body: (OutputStream) -> Unit)

    fun close()
}

/** Direct-on-disk sink under the app's private external storage. */
private class FileSink(private val root: File) : ReceiveSink {
    override val description: String = root.absolutePath
    private val _uris = mutableListOf<String>()
    override val uris: List<String> get() = _uris

    override fun mkdirs(relPath: String) {
        target(relPath).also { it.mkdirs() }
    }

    override fun writeFile(relPath: String, expectedSize: Long, body: (OutputStream) -> Unit) {
        val file = target(relPath)
        file.parentFile?.mkdirs()
        BufferedOutputStream(FileOutputStream(file)).use(body)
        _uris += android.net.Uri.fromFile(file).toString()
    }

    override fun close() {}

    private fun target(rel: String): File {
        val parts = sanitisePathSegments(rel)
        var cur = root
        for (p in parts) cur = File(cur, p)
        val rootPath = root.canonicalPath
        if (!cur.canonicalPath.startsWith(rootPath)) {
            throw IllegalStateException("path escape: $rel")
        }
        return cur
    }
}

/** SAF DocumentFile sink — files land where the user picked. */
private class DocumentSink(
    private val context: Context,
    private val sessionDoc: DocumentFile,
    override val description: String,
) : ReceiveSink {
    /** Cache directories so lookups don't pay an O(N) findFile each time. */
    private val dirCache = mutableMapOf<String, DocumentFile>("" to sessionDoc)
    private val _uris = mutableListOf<String>()
    override val uris: List<String> get() = _uris

    override fun mkdirs(relPath: String) {
        ensureDir(relPath)
    }

    override fun writeFile(relPath: String, expectedSize: Long, body: (OutputStream) -> Unit) {
        val parts = sanitisePathSegments(relPath)
        require(parts.isNotEmpty()) { "empty file path" }
        val parentDir = ensureDir(parts.dropLast(1).joinToString("/"))
        val name = parts.last()
        val mime = "application/octet-stream"
        // Drop pre-existing same-named file so we don't write a "(1)"-suffixed dupe.
        parentDir.findFile(name)?.delete()
        val file = parentDir.createFile(mime, name)
            ?: throw IllegalStateException("cannot create $relPath under ${parentDir.uri}")
        val out = context.contentResolver.openOutputStream(file.uri, "w")
            ?: throw IllegalStateException("cannot open ${file.uri} for write")
        BufferedOutputStream(out).use(body)
        _uris += file.uri.toString()
    }

    override fun close() {}

    private fun ensureDir(relPath: String): DocumentFile {
        // Empty relPath means "the session root" — files sent without any
        // wrapping directory (a desktop drop of a single file) hit this path.
        if (relPath.isEmpty()) return sessionDoc

        val parts = sanitisePathSegments(relPath)
        val key = parts.joinToString("/")
        dirCache[key]?.let { return it }

        var current = sessionDoc
        val acc = StringBuilder()
        for (p in parts) {
            if (acc.isNotEmpty()) acc.append('/')
            acc.append(p)
            val cached = dirCache[acc.toString()]
            if (cached != null) {
                current = cached
            } else {
                val existing = current.findFile(p)
                current = if (existing != null && existing.isDirectory) {
                    existing
                } else {
                    current.createDirectory(p)
                        ?: throw IllegalStateException("cannot create dir $acc")
                }
                dirCache[acc.toString()] = current
            }
        }
        return current
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// Sender (send side)
// ─────────────────────────────────────────────────────────────────────────────

/**
 * One-shot operations: text snippet, file URIs, and recursive folder send via
 * a SAF tree URI.
 */
/**
 * @param v2Identity when set, the sender will attempt the v2 Noise XX
 *                   handshake on connections to peers that
 *                   [pinnedX25519For] returns a non-null pubkey for.
 *                   Cleartext fallback happens implicitly for unpinned
 *                   peers and v1 receivers.
 * @param pinnedX25519For look-up: peer InetAddress → expected X25519
 *                       pubkey (32 bytes) of that peer, or null when no
 *                       pinning record exists for the address.
 */
class Sender(
    private val context: Context,
    private val v2Identity: dev.wasabules.dukto.identity.Identity? = null,
    private val pinnedX25519For: (InetAddress) -> ByteArray? = { null },
    /** When non-null and returns true, refuse to dial peers without a
     *  pinning record so the cleartext fallback never runs. */
    private val refuseCleartextProvider: () -> Boolean = { false },
) {

    private val scope = CoroutineScope(Dispatchers.IO + SupervisorJob() + CoroutineExceptionHandler { _, t ->
        Log.w(TAG, "uncaught in Sender scope: ${t.message}", t)
    })
    private val _events = MutableSharedFlow<TransferEvent>(extraBufferCapacity = 64)
    val events: SharedFlow<TransferEvent> = _events.asSharedFlow()

    /** Single-slot reference to the currently in-flight send socket. */
    private val activeSend = AtomicReference<Socket?>(null)

    fun cancelActiveSend() {
        activeSend.get()?.let { runCatching { it.close() } }
    }

    fun close() {
        cancelActiveSend()
        scope.cancel()
    }

    // ── text ─────────────────────────────────────────────────────────────────

    fun sendText(to: Peer, text: String) {
        scope.launch {
            runSend(to, isReceive = false) { out ->
                val bytes = text.toByteArray(Charsets.UTF_8)
                writeSessionHeader(out, SessionHeader(totalElements = 1, totalSize = bytes.size.toLong()))
                writeElementHeader(out, ElementHeader(TEXT_ELEMENT_NAME, bytes.size.toLong()))
                out.write(bytes)
                out.flush()
                emitProgress(to.address, bytes.size.toLong(), bytes.size.toLong(), false)
                bytes.size.toLong()
            }
        }
    }

    // ── files (flat list of file URIs, e.g. from OpenMultipleDocuments) ──────

    fun sendFiles(to: Peer, uris: List<Uri>) {
        scope.launch {
            runSend(to, isReceive = false) { out ->
                val plan = uris.mapNotNull { uri ->
                    val name = displayNameOf(uri) ?: return@mapNotNull null
                    val size = sizeOf(uri) ?: return@mapNotNull null
                    Triple(uri, name, size)
                }
                if (plan.isEmpty()) throw IllegalStateException("no readable file in selection")
                val totalSize = plan.sumOf { it.third }
                _events.emit(TransferEvent.Started(to.address, plan.size.toLong(), totalSize, isReceive = false))
                writeSessionHeader(out, SessionHeader(plan.size.toLong(), totalSize))
                var done = 0L
                for ((uri, name, size) in plan) {
                    writeElementHeader(out, ElementHeader(name, size))
                    context.contentResolver.openInputStream(uri)?.use { input ->
                        copyBytes(input, out, size) { delta ->
                            emitProgress(to.address, done + delta, totalSize, false)
                        }
                    } ?: throw IllegalStateException("cannot open $uri")
                    done += size
                }
                out.flush()
                totalSize
            }
        }
    }

    // ── folder (SAF tree URI) — recursive walk, directory + file headers ────

    fun sendFolder(to: Peer, treeUri: Uri) {
        scope.launch {
            runSend(to, isReceive = false) { out ->
                val tree = DocumentFile.fromTreeUri(context, treeUri)
                    ?: throw IllegalStateException("invalid tree URI")
                val rootName = tree.name?.takeIf { it.isNotBlank() } ?: "folder"
                val plan = walkTree(tree, prefix = rootName)
                if (plan.isEmpty()) throw IllegalStateException("folder is empty")
                val totalSize = plan.sumOf { if (it.size < 0L) 0L else it.size }
                _events.emit(TransferEvent.Started(to.address, plan.size.toLong(), totalSize, isReceive = false))
                writeSessionHeader(out, SessionHeader(plan.size.toLong(), totalSize))
                var done = 0L
                for (entry in plan) {
                    writeElementHeader(out, ElementHeader(entry.path, entry.size))
                    if (entry.size > 0L && entry.docUri != null) {
                        context.contentResolver.openInputStream(entry.docUri)?.use { input ->
                            copyBytes(input, out, entry.size) { delta ->
                                emitProgress(to.address, done + delta, totalSize, false)
                            }
                        } ?: throw IllegalStateException("cannot open ${entry.docUri}")
                        done += entry.size
                    }
                }
                out.flush()
                totalSize
            }
        }
    }

    private data class Entry(val path: String, val size: Long, val docUri: Uri?)

    private fun walkTree(root: DocumentFile, prefix: String): List<Entry> {
        // Pre-order: directory entry first, then its children. Mirrors the Qt
        // sender's file-list flattening rule.
        val out = mutableListOf<Entry>()
        out.add(Entry(prefix, DIRECTORY_SIZE_MARKER, null))
        val children = root.listFiles().sortedBy { it.name.orEmpty() }
        for (child in children) {
            val name = child.name?.takeIf { it.isNotBlank() } ?: continue
            val path = "$prefix/$name"
            if (child.isDirectory) {
                out.addAll(walkTree(child, path))
            } else {
                out.add(Entry(path, child.length(), child.uri))
            }
        }
        return out
    }

    // ── shared transport ─────────────────────────────────────────────────────

    private inline fun runSend(to: Peer, isReceive: Boolean, body: (OutputStream) -> Long) {
        if (refuseCleartextProvider() && pinnedX25519For(to.address) == null) {
            _events.tryEmit(TransferEvent.Failed(
                to.address,
                "refuseCleartext: peer not paired",
                isReceive,
            ))
            return
        }
        val sock = try {
            Socket(to.address, to.port).apply { tcpNoDelay = true }
        } catch (e: Exception) {
            _events.tryEmit(TransferEvent.Failed(to.address, e.message ?: e.javaClass.simpleName, isReceive))
            return
        }
        activeSend.set(sock)
        try {
            sock.use { s ->
                val v2 = senderUpgradeIfPinned(s, to.address)
                val out = BufferedOutputStream(v2)
                val total = body(out)
                _events.tryEmit(TransferEvent.Sent(to, total))
            }
        } catch (t: Throwable) {
            Log.w(TAG, "send to ${to.address} failed: ${t.message}", t)
            _events.tryEmit(TransferEvent.Failed(to.address, t.message ?: t.javaClass.simpleName, isReceive))
        } finally {
            activeSend.compareAndSet(sock, null)
        }
    }

    private fun emitProgress(addr: InetAddress, done: Long, total: Long, isReceive: Boolean) {
        _events.tryEmit(TransferEvent.Progress(addr, done.coerceAtMost(total), total, isReceive))
    }

    private fun displayNameOf(uri: Uri): String? {
        val cr = context.contentResolver
        cr.query(uri, arrayOf(android.provider.OpenableColumns.DISPLAY_NAME), null, null, null)?.use { c ->
            if (c.moveToFirst()) {
                val n = c.getString(0)
                if (!n.isNullOrEmpty()) return n
            }
        }
        return uri.lastPathSegment?.substringAfterLast('/')
    }

    private fun sizeOf(uri: Uri): Long? {
        val cr = context.contentResolver
        cr.query(uri, arrayOf(android.provider.OpenableColumns.SIZE), null, null, null)?.use { c ->
            if (c.moveToFirst() && !c.isNull(0)) return c.getLong(0)
        }
        return null
    }

    /**
     * If the peer at [addr] has a pinned X25519 pubkey, run the Noise XX
     * initiator on [sock]'s I/O streams, verify the remote_static, and
     * return an [OutputStream] that writes encrypted frames. Otherwise
     * return the raw socket output stream (cleartext fallback).
     */
    private fun senderUpgradeIfPinned(sock: Socket, addr: InetAddress): OutputStream {
        val identity = v2Identity ?: return sock.getOutputStream()
        val expected = pinnedX25519For(addr) ?: return sock.getOutputStream()
        val session = dev.wasabules.dukto.tunnel.handshake(
            sock.getInputStream(), sock.getOutputStream(),
            dev.wasabules.dukto.tunnel.HandshakeRole.Initiator,
            identity.x25519Private(), identity.x25519Public(),
            psk = null,
            closer = {},
        )
        if (!session.remoteStatic.contentEquals(expected)) {
            session.close()
            throw IllegalStateException("v2 fingerprint mismatch with ${addr.hostAddress}")
        }
        return SenderSessionOutput(session)
    }

    private companion object { const val TAG = "DuktoSender" }
}

/** Adapter exposing a [Session]'s write() as a java.io.OutputStream for
 *  the Sender side. Mirrors SessionOutput on the Server side. */
private class SenderSessionOutput(private val s: dev.wasabules.dukto.tunnel.Session) : OutputStream() {
    override fun write(b: Int) { s.write(byteArrayOf((b and 0xFF).toByte())) }
    override fun write(b: ByteArray, off: Int, len: Int) { s.write(b, off, len) }
    override fun close() = s.close()
}

// ─────────────────────────────────────────────────────────────────────────────
// helpers
// ─────────────────────────────────────────────────────────────────────────────

private fun copyBytes(
    src: InputStream,
    dst: OutputStream,
    bytes: Long,
    onProgress: (Long) -> Unit,
) {
    val buf = ByteArray(64 * 1024)
    var remaining = bytes
    var transferred = 0L
    while (remaining > 0L) {
        val want = min(buf.size.toLong(), remaining).toInt()
        val n = src.read(buf, 0, want)
        if (n < 0) throw java.io.EOFException("element ended early, $remaining bytes missing")
        dst.write(buf, 0, n)
        remaining -= n
        transferred += n
        onProgress(transferred)
    }
}

/** Drop empty / "." / ".." segments and reject leading-/, returning a clean list. */
private fun sanitisePathSegments(path: String): List<String> {
    val parts = path.split('/').filter { it.isNotEmpty() && it != "." && it != ".." }
    if (parts.isEmpty()) throw IllegalArgumentException("invalid path: $path")
    return parts
}
