package dev.wasabules.dukto.ui

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.widget.Toast
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.core.net.toUri
import coil.compose.AsyncImage
import coil.request.ImageRequest
import dev.wasabules.dukto.ActivityEntry

/**
 * Detail/preview view for a single activity entry.
 *
 * Layout:
 *  - TextReceived → the text body verbatim with Copy/Share buttons.
 *  - FilesReceived → a grid of thumbnails (image MIME types render via Coil;
 *    everything else falls back to a generic icon tile). Tap a tile to launch
 *    the system viewer (`ACTION_VIEW` with the mime type).
 *  - Sent / Error → the activity row's contents, no actions.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PreviewScreen(
    entry: ActivityEntry,
    onClose: () -> Unit,
) {
    val ctx = LocalContext.current
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(titleFor(entry)) },
                navigationIcon = {
                    IconButton(onClick = onClose) {
                        Text("←", style = MaterialTheme.typography.titleLarge)
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.surface,
                ),
            )
        },
    ) { padding ->
        when (entry) {
            is ActivityEntry.TextReceived -> TextPreview(entry, ctx, Modifier.padding(padding))
            is ActivityEntry.FilesReceived -> FilesPreview(entry, ctx, Modifier.padding(padding))
            is ActivityEntry.Sent, is ActivityEntry.Error -> {
                Column(modifier = Modifier
                    .padding(padding)
                    .padding(16.dp)
                    .fillMaxSize()) {
                    Text("Nothing to preview here.", style = MaterialTheme.typography.bodyMedium)
                }
            }
        }
    }
}

@Composable
private fun TextPreview(entry: ActivityEntry.TextReceived, ctx: Context, modifier: Modifier) {
    Column(
        modifier = modifier
            .fillMaxSize()
            .padding(16.dp)
            .verticalScroll(rememberScrollState()),
    ) {
        Text(
            "From ${entry.from}",
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.height(12.dp))
        Card(
            colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceVariant),
            shape = RoundedCornerShape(12.dp),
            modifier = Modifier.fillMaxWidth(),
        ) {
            Text(entry.text, modifier = Modifier.padding(16.dp), style = MaterialTheme.typography.bodyLarge)
        }
        Spacer(Modifier.height(12.dp))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(onClick = {
                val cm = ctx.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                cm.setPrimaryClip(ClipData.newPlainText("Dukto", entry.text))
                Toast.makeText(ctx, "Copied", Toast.LENGTH_SHORT).show()
            }) { Text("Copy") }
            OutlinedButton(onClick = {
                ctx.startActivity(Intent.createChooser(Intent(Intent.ACTION_SEND).apply {
                    type = "text/plain"
                    putExtra(Intent.EXTRA_TEXT, entry.text)
                }, "Share text"))
            }) { Text("Share") }
        }
    }
}

@Composable
private fun FilesPreview(entry: ActivityEntry.FilesReceived, ctx: Context, modifier: Modifier) {
    Column(modifier = modifier.fillMaxSize().padding(16.dp)) {
        Text(
            "${entry.fileCount} file(s) from ${entry.from}",
            style = MaterialTheme.typography.titleMedium,
        )
        Text(
            entry.location,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
        Spacer(Modifier.height(12.dp))
        if (entry.fileUris.isEmpty()) {
            Text(
                "No file URIs were captured for this transfer.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            return@Column
        }
        LazyVerticalGrid(
            columns = GridCells.Fixed(3),
            modifier = Modifier.fillMaxSize(),
            verticalArrangement = Arrangement.spacedBy(8.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            items(entry.fileUris) { FileTile(uri = it) }
        }
    }
}

@Composable
private fun FileTile(uri: String) {
    val ctx = LocalContext.current
    val parsed = remember(uri) { runCatching { uri.toUri() }.getOrNull() }
    val mime = remember(uri) { mimeForUri(ctx, parsed) }
    val display = remember(uri) { displayNameForUri(ctx, parsed) ?: parsed?.lastPathSegment ?: "file" }
    val isImage = mime?.startsWith("image/") == true

    Card(
        modifier = Modifier
            .aspectRatio(1f)
            .clickable {
                if (parsed != null) openInExternalViewer(ctx, parsed, mime)
            },
        shape = RoundedCornerShape(12.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceVariant),
    ) {
        Box(modifier = Modifier.fillMaxSize()) {
            if (isImage && parsed != null) {
                AsyncImage(
                    model = ImageRequest.Builder(ctx).data(parsed).crossfade(true).build(),
                    contentDescription = display,
                    contentScale = ContentScale.Crop,
                    modifier = Modifier.fillMaxSize(),
                )
            } else {
                Column(
                    modifier = Modifier.fillMaxSize().padding(8.dp),
                    verticalArrangement = Arrangement.Center,
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    Text("📄", style = MaterialTheme.typography.headlineSmall)
                    Spacer(Modifier.height(4.dp))
                    Text(
                        display,
                        style = MaterialTheme.typography.labelSmall,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            }
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .background(MaterialTheme.colorScheme.scrim.copy(alpha = 0.6f))
                    .align(Alignment.BottomCenter)
                    .padding(horizontal = 6.dp, vertical = 2.dp),
            ) {
                Text(
                    display,
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.surface,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
    }
}

// ── helpers ──────────────────────────────────────────────────────────────────

private fun titleFor(entry: ActivityEntry): String = when (entry) {
    is ActivityEntry.TextReceived -> "Text from ${entry.from}"
    is ActivityEntry.FilesReceived -> "Files from ${entry.from}"
    is ActivityEntry.Sent -> "Sent to ${entry.to}"
    is ActivityEntry.Error -> "Error: ${entry.peer}"
}

private fun mimeForUri(ctx: Context, uri: Uri?): String? {
    uri ?: return null
    if (uri.scheme == "content") {
        return ctx.contentResolver.getType(uri)
    }
    val ext = uri.lastPathSegment?.substringAfterLast('.', "")?.lowercase()
    return when (ext) {
        "png" -> "image/png"
        "jpg", "jpeg" -> "image/jpeg"
        "gif" -> "image/gif"
        "webp" -> "image/webp"
        "mp4" -> "video/mp4"
        "mp3" -> "audio/mpeg"
        "txt" -> "text/plain"
        "pdf" -> "application/pdf"
        else -> null
    }
}

private fun displayNameForUri(ctx: Context, uri: Uri?): String? {
    uri ?: return null
    if (uri.scheme != "content") return uri.lastPathSegment
    return ctx.contentResolver.query(
        uri, arrayOf(android.provider.OpenableColumns.DISPLAY_NAME), null, null, null,
    )?.use { c -> if (c.moveToFirst()) c.getString(0) else null }
}

private fun openInExternalViewer(ctx: Context, uri: Uri, mime: String?) {
    val intent = Intent(Intent.ACTION_VIEW).apply {
        setDataAndType(uri, mime ?: "*/*")
        addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
    }
    runCatching { ctx.startActivity(Intent.createChooser(intent, "Open with")) }
        .onFailure { Toast.makeText(ctx, "No app can open this", Toast.LENGTH_SHORT).show() }
}
