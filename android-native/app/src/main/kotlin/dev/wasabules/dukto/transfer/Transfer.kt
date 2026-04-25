package dev.wasabules.dukto.transfer

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Environment
import android.util.Log
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
import kotlin.math.min

/**
 * Per-session reception result emitted by [Server] on its events flow.
 *
 * For the MVP we keep things simple: text snippets surface as [TextReceived],
 * file transfers (incl. directories) as [FilesReceived] with the on-disk root.
 */
sealed interface TransferEvent {
    data class Started(val from: InetAddress, val totalElements: Long, val totalSize: Long) : TransferEvent
    data class Progress(val from: InetAddress, val bytesReceived: Long, val totalSize: Long) : TransferEvent
    data class TextReceived(val from: InetAddress, val text: String) : TransferEvent
    data class FilesReceived(val from: InetAddress, val rootDir: File, val fileCount: Int) : TransferEvent
    data class Failed(val from: InetAddress?, val reason: String) : TransferEvent
    data class Sent(val to: Peer, val totalSize: Long) : TransferEvent
    data class SendFailed(val to: Peer, val reason: String) : TransferEvent
}

/**
 * Lightweight Peer alias for callers; full discovery type lives in
 * [dev.wasabules.dukto.discovery.Peer]. Re-exported here so transfer code
 * doesn't pull in discovery dependencies.
 */
data class Peer(val address: InetAddress, val port: Int = DEFAULT_PORT)

/**
 * TCP server that accepts incoming Dukto sessions on [port] and emits events
 * for each session it handles.
 *
 * Implementation choices kept simple for the MVP:
 *  - Files are received under [destDir] (defaults to the app's external
 *    Downloads dir, which is writable without runtime permissions on
 *    Android 10+ and visible to file manager apps).
 *  - Each session runs in its own coroutine on Dispatchers.IO; the server
 *    accepts up to N concurrent receives without throttling.
 *  - No security gates (whitelist / blocklist / size cap) yet — those can be
 *    layered on later by replacing [shouldAccept].
 */
class Server(
    private val context: Context,
    private val port: Int = DEFAULT_PORT,
    private val destDirOverride: File? = null,
) {
    private val scope = CoroutineScope(Dispatchers.IO)
    private val _events = MutableSharedFlow<TransferEvent>(extraBufferCapacity = 64)
    val events: SharedFlow<TransferEvent> = _events.asSharedFlow()

    private var serverSocket: ServerSocket? = null
    private var acceptJob: Job? = null

    /** Resolved on demand so `getExternalFilesDir` is called only when needed. */
    private val destDir: File
        get() = destDirOverride
            ?: context.getExternalFilesDir(Environment.DIRECTORY_DOWNLOADS)
            ?: File(context.filesDir, "Downloads").apply { mkdirs() }

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
        try {
            client.use { sock ->
                val input = BufferedInputStream(sock.getInputStream())
                val header = readSessionHeader(input)
                _events.emit(TransferEvent.Started(from, header.totalElements, header.totalSize))

                // First element decides whether this is a text snippet or a
                // file/dir batch (text uses a magic name + must be the only
                // element).
                if (header.totalElements == 1L) {
                    val first = readElementHeader(input)
                    if (first.isText) {
                        receiveText(input, from, first.size)
                        return@use
                    }
                    // Single file/dir — fall through to the multi-element path
                    // by replaying it.
                    receiveFiles(input, from, header, first)
                    return@use
                }
                receiveFiles(input, from, header, firstElement = null)
            }
        } catch (e: Exception) {
            Log.w(TAG, "session from $from failed: ${e.message}")
            _events.emit(TransferEvent.Failed(from, e.message ?: e.javaClass.simpleName))
        }
    }

    private suspend fun receiveText(input: InputStream, from: InetAddress, size: Long) {
        if (size < 0 || size > MAX_TEXT_BYTES) {
            throw IllegalStateException("text snippet size out of range: $size")
        }
        val out = ByteArrayOutputStream(size.toInt().coerceAtLeast(64))
        copy(input, out, size, onProgress = { _events.tryEmit(TransferEvent.Progress(from, it, size)) })
        _events.emit(TransferEvent.TextReceived(from, out.toString(Charsets.UTF_8)))
    }

    private suspend fun receiveFiles(
        input: InputStream,
        from: InetAddress,
        header: SessionHeader,
        firstElement: ElementHeader?,
    ) {
        // Drop into a fresh subdir to keep concurrent transfers separate.
        val sessionDir = File(destDir, "dukto-${System.currentTimeMillis()}-${from.hostAddress?.replace('.', '_')}")
        if (!sessionDir.mkdirs() && !sessionDir.isDirectory) {
            throw IllegalStateException("cannot create $sessionDir")
        }
        var fileCount = 0
        var bytesSoFar = 0L
        var elementsLeft = header.totalElements
        var firstEl = firstElement
        while (elementsLeft > 0L) {
            val el = firstEl ?: readElementHeader(input)
            firstEl = null
            elementsLeft--

            val target = sessionDir.safeChildOf(el.name)
            when {
                el.size == DIRECTORY_SIZE_MARKER -> {
                    if (!target.mkdirs() && !target.isDirectory) {
                        throw IllegalStateException("cannot mkdir $target")
                    }
                }
                else -> {
                    target.parentFile?.mkdirs()
                    BufferedOutputStream(FileOutputStream(target)).use { out ->
                        copy(
                            input, out, el.size,
                            onProgress = { delta ->
                                bytesSoFar += delta - bytesSoFar // bytes written for this element
                            },
                        )
                    }
                    fileCount++
                    _events.tryEmit(
                        TransferEvent.Progress(from, bytesSoFar.coerceAtMost(header.totalSize), header.totalSize)
                    )
                }
            }
        }
        _events.emit(TransferEvent.FilesReceived(from, sessionDir, fileCount))
    }

    private companion object {
        const val TAG = "DuktoServer"
        const val MAX_TEXT_BYTES = 8L * 1024L * 1024L
    }
}

/**
 * One-shot sender. Handles three kinds of payload, all of which the existing
 * Qt/Wails clients understand:
 *  - [sendText] → 1-element session with the magic text name
 *  - [sendFiles] → flat list of files (no directory traversal yet for the MVP;
 *    nested directories are TODO once the UI supports tree picking)
 */
class Sender(private val context: Context) {

    private val scope = CoroutineScope(Dispatchers.IO)
    private val _events = MutableSharedFlow<TransferEvent>(extraBufferCapacity = 32)
    val events: SharedFlow<TransferEvent> = _events.asSharedFlow()

    /** Send a text snippet. */
    fun sendText(to: Peer, text: String) {
        scope.launch { runSend(to) { out ->
            val bytes = text.toByteArray(Charsets.UTF_8)
            writeSessionHeader(out, SessionHeader(totalElements = 1, totalSize = bytes.size.toLong()))
            writeElementHeader(out, ElementHeader(TEXT_ELEMENT_NAME, bytes.size.toLong()))
            out.write(bytes)
            out.flush()
            bytes.size.toLong()
        } }
    }

    /**
     * Send the given content URIs (typically from ACTION_GET_CONTENT or
     * ACTION_SEND). Each URI is sent as a top-level file element; the original
     * filename is taken from [Uri.getLastPathSegment] or, when available, the
     * `_display_name` column.
     */
    fun sendFiles(to: Peer, uris: List<Uri>) {
        scope.launch { runSend(to) { out ->
            // Pre-pass: resolve sizes so the session header is accurate.
            val plan = uris.mapNotNull { uri ->
                val name = displayNameOf(uri) ?: return@mapNotNull null
                val size = sizeOf(uri) ?: return@mapNotNull null
                Triple(uri, name, size)
            }
            val totalSize = plan.sumOf { it.third }
            writeSessionHeader(out, SessionHeader(totalElements = plan.size.toLong(), totalSize = totalSize))
            for ((uri, name, size) in plan) {
                writeElementHeader(out, ElementHeader(name, size))
                context.contentResolver.openInputStream(uri)?.use { input ->
                    copySync(input, out, size)
                } ?: throw IllegalStateException("cannot open $uri")
            }
            out.flush()
            totalSize
        } }
    }

    private inline fun runSend(to: Peer, body: (OutputStream) -> Long) {
        try {
            Socket(to.address, to.port).use { sock ->
                sock.tcpNoDelay = true
                val out = BufferedOutputStream(sock.getOutputStream())
                val total = body(out)
                _events.tryEmit(TransferEvent.Sent(to, total))
            }
        } catch (e: Exception) {
            Log.w(TAG, "send to ${to.address} failed: ${e.message}")
            _events.tryEmit(TransferEvent.SendFailed(to, e.message ?: e.javaClass.simpleName))
        }
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
        // Fallback: stream once to count bytes. Avoid for huge files.
        return null
    }

    fun close() = scope.cancel()

    private companion object { const val TAG = "DuktoSender" }
}

// ── byte-copy helpers ────────────────────────────────────────────────────────

private suspend fun copy(
    src: InputStream,
    dst: OutputStream,
    bytes: Long,
    onProgress: (Long) -> Unit,
) = withContext(Dispatchers.IO) {
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

private fun copySync(src: InputStream, dst: OutputStream, bytes: Long) {
    val buf = ByteArray(64 * 1024)
    var remaining = bytes
    while (remaining > 0L) {
        val want = min(buf.size.toLong(), remaining).toInt()
        val n = src.read(buf, 0, want)
        if (n < 0) throw java.io.EOFException("source ended early, $remaining bytes missing")
        dst.write(buf, 0, n)
        remaining -= n
    }
}

/**
 * Resolve [name] under this directory while refusing path-escape segments.
 * Names from the wire use '/' as separator regardless of host OS.
 */
private fun File.safeChildOf(name: String): File {
    val parts = name.split('/').filter { it.isNotEmpty() && it != "." && it != ".." }
    if (parts.isEmpty()) throw IllegalArgumentException("invalid element name: $name")
    var cur = this
    for (p in parts) cur = File(cur, p)
    val root = this.canonicalPath
    val resolved = cur.canonicalPath
    if (!resolved.startsWith(root)) throw IllegalStateException("path escape: $name")
    return cur
}
