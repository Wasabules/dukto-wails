package dev.wasabules.dukto.ui

import android.content.Context
import android.net.Uri
import android.provider.OpenableColumns
import android.webkit.MimeTypeMap
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.platform.LocalContext
import androidx.core.net.toUri
import java.io.File

/**
 * Minimal metadata for a received file URI: display name, byte size, and a
 * best-effort MIME type for thumbnail and ACTION_VIEW routing.
 */
data class FileMeta(
    val uri: Uri,
    val name: String,
    val size: Long,
    val mime: String?,
) {
    val isImage: Boolean get() = mime?.startsWith("image/") == true
}

/**
 * Resolve [FileMeta] for a given URI string. Memoized per (uri, context) so
 * scrolling activity rows doesn't re-query the ContentResolver on every frame.
 */
@Composable
fun rememberFileMeta(uri: String): FileMeta? {
    val ctx = LocalContext.current
    return remember(uri) { resolveFileMeta(ctx, uri) }
}

fun resolveFileMeta(ctx: Context, uriString: String): FileMeta? {
    val uri = runCatching { uriString.toUri() }.getOrNull() ?: return null
    val name: String
    val size: Long
    val mime: String?
    when (uri.scheme) {
        "content" -> {
            val cr = ctx.contentResolver
            var n: String? = null
            var s: Long = 0L
            cr.query(
                uri,
                arrayOf(OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE),
                null, null, null,
            )?.use { c ->
                if (c.moveToFirst()) {
                    n = c.getString(0)
                    if (!c.isNull(1)) s = c.getLong(1)
                }
            }
            name = n?.takeIf { it.isNotBlank() } ?: uri.lastPathSegment.orEmpty()
            size = s
            mime = cr.getType(uri) ?: mimeFromExt(name)
        }
        "file" -> {
            val f = uri.path?.let { File(it) }
            name = f?.name ?: uri.lastPathSegment.orEmpty()
            size = f?.length() ?: 0L
            mime = mimeFromExt(name)
        }
        else -> {
            name = uri.lastPathSegment.orEmpty()
            size = 0L
            mime = mimeFromExt(name)
        }
    }
    if (name.isEmpty()) return null
    return FileMeta(uri, name, size, mime)
}

private fun mimeFromExt(name: String): String? {
    val ext = name.substringAfterLast('.', "").lowercase()
    if (ext.isEmpty()) return null
    return MimeTypeMap.getSingleton().getMimeTypeFromExtension(ext)
}

/** Pretty-print bytes (matches the existing Wails desktop format). */
fun formatBytesShort(b: Long): String {
    if (b <= 0L) return "?"
    val units = listOf("B", "KB", "MB", "GB")
    var v = b.toDouble()
    var unit = 0
    while (v >= 1024.0 && unit < units.lastIndex) { v /= 1024.0; unit++ }
    return if (unit == 0) "$b B" else "%.1f %s".format(v, units[unit])
}
