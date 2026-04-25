package dev.wasabules.dukto.transfer

import android.content.Context
import android.net.Uri
import android.os.Environment
import android.util.Log
import androidx.core.net.toUri
import androidx.documentfile.provider.DocumentFile
import dev.wasabules.dukto.protocol.DEFAULT_PORT
import dev.wasabules.dukto.protocol.DIRECTORY_SIZE_MARKER
import dev.wasabules.dukto.protocol.ElementHeader
import dev.wasabules.dukto.protocol.SessionHeader
import dev.wasabules.dukto.protocol.TEXT_ELEMENT_NAME
import dev.wasabules.dukto.protocol.readElementHeader
import dev.wasabules.dukto.protocol.readSessionHeader
import dev.wasabules.dukto.protocol.writeElementHeader
import dev.wasabules.dukto.protocol.writeSessionHeader
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
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
    data class FilesReceived(val from: InetAddress, val rootDescription: String, val fileCount: Int) : TransferEvent
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
class Server(
    private val context: Context,
    private val port: Int = DEFAULT_PORT,
    private val destTreeUriProvider: () -> String? = { null },
) {
    private val scope = CoroutineScope(Dispatchers.IO)
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
            client.use { sock ->
                val input = BufferedInputStream(sock.getInputStream())
                val header = readSessionHeader(input)
                _events.emit(TransferEvent.Started(from, header.totalElements, header.totalSize, isReceive = true))

                if (header.totalElements == 1L) {
                    val first = readElementHeader(input)
                    if (first.isText) {
                        receiveText(input, from, first.size)
                        return@use
                    }
                    receiveFiles(input, from, header, first)
                    return@use
                }
                receiveFiles(input, from, header, firstElement = null)
            }
        } catch (e: Exception) {
            Log.w(TAG, "session from $from failed: ${e.message}")
            _events.emit(TransferEvent.Failed(from, e.message ?: e.javaClass.simpleName, isReceive = true))
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
        _events.emit(TransferEvent.FilesReceived(from, sink.description, fileCount))
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

    private companion object {
        const val TAG = "DuktoServer"
        const val MAX_TEXT_BYTES = 8L * 1024L * 1024L
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// Receive sinks
// ─────────────────────────────────────────────────────────────────────────────

private interface ReceiveSink {
    /** Human-readable destination (path or tree URI) for activity logs. */
    val description: String

    /** Create directories along [relPath] (relative, '/' separators). Idempotent. */
    fun mkdirs(relPath: String)

    /** Open a write stream for the file at [relPath] (parent dirs created if needed). */
    fun writeFile(relPath: String, expectedSize: Long, body: (OutputStream) -> Unit)

    fun close()
}

/** Direct-on-disk sink under the app's private external storage. */
private class FileSink(private val root: File) : ReceiveSink {
    override val description: String = root.absolutePath

    override fun mkdirs(relPath: String) {
        target(relPath).also { it.mkdirs() }
    }

    override fun writeFile(relPath: String, expectedSize: Long, body: (OutputStream) -> Unit) {
        val file = target(relPath)
        file.parentFile?.mkdirs()
        BufferedOutputStream(FileOutputStream(file)).use(body)
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
    }

    override fun close() {}

    private fun ensureDir(relPath: String): DocumentFile {
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
class Sender(private val context: Context) {

    private val scope = CoroutineScope(Dispatchers.IO)
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
        val sock = try {
            Socket(to.address, to.port).apply { tcpNoDelay = true }
        } catch (e: Exception) {
            _events.tryEmit(TransferEvent.Failed(to.address, e.message ?: e.javaClass.simpleName, isReceive))
            return
        }
        activeSend.set(sock)
        try {
            sock.use { s ->
                val out = BufferedOutputStream(s.getOutputStream())
                val total = body(out)
                _events.tryEmit(TransferEvent.Sent(to, total))
            }
        } catch (e: Exception) {
            Log.w(TAG, "send to ${to.address} failed: ${e.message}")
            _events.tryEmit(TransferEvent.Failed(to.address, e.message ?: e.javaClass.simpleName, isReceive))
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

    private companion object { const val TAG = "DuktoSender" }
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
