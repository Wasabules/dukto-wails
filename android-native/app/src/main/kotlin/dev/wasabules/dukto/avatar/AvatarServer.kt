package dev.wasabules.dukto.avatar

import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.graphics.Typeface
import android.util.Log
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.BufferedReader
import java.io.ByteArrayOutputStream
import java.io.InputStreamReader
import java.io.OutputStream
import java.net.ServerSocket
import java.net.Socket

/**
 * Tiny HTTP server that serves a 64×64 PNG at `/dukto/avatar`. Bound to
 * `0.0.0.0:udpPort + 1` (4645 by default), matching the Qt client's
 * out-of-band avatar side-channel (see docs/PROTOCOL.md §6).
 *
 * The avatar is generated from the current signature's initials so each peer
 * sees something rather than the "unknown" placeholder. A real photo picker
 * can plug in later by overriding [bitmapProvider].
 */
class AvatarServer(
    private val port: Int,
    private val signatureProvider: () -> String,
    private val bitmapProvider: (String) -> ByteArray = ::defaultAvatarPng,
) {
    private val scope = CoroutineScope(Dispatchers.IO)
    private var serverSocket: ServerSocket? = null
    private var acceptJob: Job? = null

    fun start() {
        if (serverSocket != null) return
        val s = ServerSocket(port).apply { reuseAddress = true }
        serverSocket = s
        acceptJob = scope.launch { loop(s) }
    }

    fun stop() {
        runCatching { serverSocket?.close() }
        serverSocket = null
        acceptJob?.cancel()
        scope.cancel()
    }

    private suspend fun loop(s: ServerSocket) = withContext(Dispatchers.IO) {
        while (true) {
            val client = try {
                s.accept()
            } catch (e: Exception) {
                if (s.isClosed) return@withContext
                continue
            }
            scope.launch { handle(client) }
        }
    }

    private fun handle(client: Socket) {
        try {
            client.use { sock ->
                val reader = BufferedReader(InputStreamReader(sock.getInputStream(), Charsets.US_ASCII))
                val requestLine = reader.readLine() ?: return
                // Drain headers (we don't care about them).
                while (true) {
                    val line = reader.readLine() ?: break
                    if (line.isEmpty()) break
                }
                val out = sock.getOutputStream()
                if (!requestLine.startsWith("GET ")) {
                    writeStatus(out, 405, "Method Not Allowed")
                    return
                }
                val path = requestLine.split(' ').getOrNull(1).orEmpty()
                if (path != "/dukto/avatar") {
                    writeStatus(out, 404, "Not Found")
                    return
                }
                val png = try {
                    bitmapProvider(signatureProvider())
                } catch (e: Exception) {
                    writeStatus(out, 500, "Internal Server Error")
                    return
                }
                val header = buildString {
                    append("HTTP/1.1 200 OK\r\n")
                    append("Content-Type: image/png\r\n")
                    append("Content-Length: ${png.size}\r\n")
                    append("Connection: close\r\n")
                    append("\r\n")
                }
                out.write(header.toByteArray(Charsets.US_ASCII))
                out.write(png)
                out.flush()
            }
        } catch (e: Exception) {
            Log.v(TAG, "avatar request: ${e.message}")
        }
    }

    private fun writeStatus(out: OutputStream, code: Int, msg: String) {
        val body = "$code $msg\n".toByteArray(Charsets.US_ASCII)
        val hdr = "HTTP/1.1 $code $msg\r\nContent-Length: ${body.size}\r\nConnection: close\r\n\r\n"
        out.write(hdr.toByteArray(Charsets.US_ASCII))
        out.write(body)
        out.flush()
    }

    private companion object { const val TAG = "DuktoAvatar" }
}

/**
 * Renders a 64×64 PNG with the buddy's initials on a deterministic colour
 * background derived from the signature hash. Self-contained — no resources
 * or runtime dependencies beyond the platform Canvas.
 */
internal fun defaultAvatarPng(signature: String): ByteArray {
    val initials = signature.takeWhile { it.isLetter() || it == ' ' }
        .split(' ').filter { it.isNotEmpty() }
        .take(2)
        .joinToString("") { it.first().uppercase() }
        .ifEmpty { "?" }
    val hue = ((signature.hashCode() and 0x7FFFFFFF) % 360).toFloat()
    val bg = Color.HSVToColor(floatArrayOf(hue, 0.5f, 0.85f))

    val size = 64
    val bmp = Bitmap.createBitmap(size, size, Bitmap.Config.ARGB_8888)
    val canvas = Canvas(bmp)
    canvas.drawColor(bg)

    val paint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        color = Color.WHITE
        textSize = 30f
        textAlign = Paint.Align.CENTER
        typeface = Typeface.create(Typeface.DEFAULT, Typeface.BOLD)
    }
    val baseline = size / 2f - (paint.descent() + paint.ascent()) / 2f
    canvas.drawText(initials, size / 2f, baseline, paint)

    val baos = ByteArrayOutputStream(2048)
    bmp.compress(Bitmap.CompressFormat.PNG, 100, baos)
    return baos.toByteArray()
}
