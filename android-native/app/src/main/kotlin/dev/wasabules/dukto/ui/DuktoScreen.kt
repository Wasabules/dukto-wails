package dev.wasabules.dukto.ui

import android.net.Uri
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import dev.wasabules.dukto.ActivityEntry
import dev.wasabules.dukto.InflightTransfer
import dev.wasabules.dukto.Profile
import dev.wasabules.dukto.discovery.Peer
import java.text.DateFormat
import java.util.Date

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DuktoScreen(
    profile: Profile,
    destLabel: String,
    peers: List<Peer>,
    activity: List<ActivityEntry>,
    inflight: InflightTransfer?,
    pendingShare: List<Uri>,
    onBuddyNameChange: (String) -> Unit,
    onPickDestFolder: () -> Unit,
    onClearDestFolder: () -> Unit,
    onSendText: (Peer, String) -> Unit,
    onSendFiles: (Peer) -> Unit,
    onSendFolder: (Peer) -> Unit,
    onCancelInflight: () -> Unit,
) {
    var settingsOpen by remember { mutableStateOf(false) }
    var sendSheetPeer by remember { mutableStateOf<Peer?>(null) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Dukto Native") },
                actions = {
                    IconButton(onClick = { settingsOpen = true }) {
                        Text("⚙", style = MaterialTheme.typography.titleLarge)
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.surface,
                ),
            )
        },
    ) { paddingValues ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(paddingValues)
                .padding(horizontal = 16.dp),
        ) {
            if (pendingShare.isNotEmpty()) {
                ShareBanner(count = pendingShare.size)
            }
            inflight?.let {
                TransferBar(it, onCancel = onCancelInflight)
            }

            SectionHeader("Peers on your network")
            if (peers.isEmpty()) {
                EmptyHint("Looking for peers… open Dukto on another device on the same Wi-Fi.")
            } else {
                LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    items(peers, key = { it.key }) { peer ->
                        PeerRow(peer, onClick = { sendSheetPeer = peer })
                    }
                }
            }

            Spacer(Modifier.height(16.dp))
            HorizontalDivider()
            Spacer(Modifier.height(16.dp))

            SectionHeader("Recent activity")
            if (activity.isEmpty()) {
                EmptyHint("Sent and received items will show up here.")
            } else {
                LazyColumn(
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                    contentPadding = PaddingValues(bottom = 16.dp),
                ) {
                    items(activity, key = { it.at }) { entry ->
                        ActivityRow(entry)
                    }
                }
            }
        }

        if (settingsOpen) {
            SettingsSheet(
                profile = profile,
                destLabel = destLabel,
                onBuddyNameChange = onBuddyNameChange,
                onPickDestFolder = onPickDestFolder,
                onClearDestFolder = onClearDestFolder,
                onDismiss = { settingsOpen = false },
            )
        }

        sendSheetPeer?.let { peer ->
            SendSheet(
                peer = peer,
                hasPendingShare = pendingShare.isNotEmpty(),
                onSendText = { text ->
                    onSendText(peer, text)
                    sendSheetPeer = null
                },
                onSendFiles = {
                    onSendFiles(peer)
                    sendSheetPeer = null
                },
                onSendFolder = {
                    onSendFolder(peer)
                    sendSheetPeer = null
                },
                onDismiss = { sendSheetPeer = null },
            )
        }
    }
}

// ── pieces ───────────────────────────────────────────────────────────────────

@Composable
private fun PeerRow(peer: Peer, onClick: () -> Unit) {
    Card(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick),
        shape = RoundedCornerShape(16.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceVariant),
    ) {
        Row(
            modifier = Modifier.padding(16.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Avatar(seedText = peer.signature)
            Spacer(Modifier.size(16.dp))
            Column(Modifier.weight(1f)) {
                Text(peer.signature, style = MaterialTheme.typography.titleMedium)
                Text(
                    peer.address.hostAddress.orEmpty(),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun Avatar(seedText: String) {
    val initials = seedText.takeWhile { it.isLetter() || it == ' ' }.split(' ')
        .filter { it.isNotEmpty() }.take(2).joinToString("") { it.first().uppercase() }
        .ifEmpty { "?" }
    Box(
        modifier = Modifier
            .size(40.dp)
            .background(MaterialTheme.colorScheme.primary, CircleShape),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            text = initials,
            color = MaterialTheme.colorScheme.onPrimary,
            style = MaterialTheme.typography.titleMedium,
            fontWeight = FontWeight.Bold,
        )
    }
}

@Composable
private fun ActivityRow(entry: ActivityEntry) {
    val time = DateFormat.getTimeInstance(DateFormat.SHORT).format(Date(entry.at))
    val (title, body) = when (entry) {
        is ActivityEntry.TextReceived -> "Text from ${entry.from}" to entry.text
        is ActivityEntry.FilesReceived ->
            "${entry.fileCount} file(s) from ${entry.from}" to entry.location
        is ActivityEntry.Sent -> "Sent ${formatBytes(entry.bytes)} to ${entry.to}" to ""
        is ActivityEntry.Error -> "Error: ${entry.peer}" to entry.message
    }
    Card(
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(12.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
    ) {
        Column(Modifier.padding(12.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    title,
                    style = MaterialTheme.typography.titleSmall,
                    modifier = Modifier.weight(1f),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                Text(time, style = MaterialTheme.typography.labelSmall)
            }
            if (body.isNotEmpty()) {
                Spacer(Modifier.height(4.dp))
                Text(body, style = MaterialTheme.typography.bodyMedium, maxLines = 4, overflow = TextOverflow.Ellipsis)
            }
        }
    }
}

@Composable
private fun TransferBar(t: InflightTransfer, onCancel: () -> Unit) {
    val ratio = if (t.totalBytes <= 0L) 0f else (t.bytesDone.toFloat() / t.totalBytes).coerceIn(0f, 1f)
    Card(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 8.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.primaryContainer),
        shape = RoundedCornerShape(12.dp),
    ) {
        Column(Modifier.padding(12.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    if (t.isReceive) "Receiving from ${t.peer}" else "Sending to ${t.peer}",
                    style = MaterialTheme.typography.titleSmall,
                    modifier = Modifier.weight(1f),
                )
                TextButton(onClick = onCancel) { Text("Cancel") }
            }
            Spacer(Modifier.height(4.dp))
            LinearProgressIndicator(
                progress = { ratio },
                modifier = Modifier.fillMaxWidth(),
            )
            Spacer(Modifier.height(4.dp))
            Text(
                "${formatBytes(t.bytesDone)} / ${formatBytes(t.totalBytes)}",
                style = MaterialTheme.typography.labelSmall,
            )
        }
    }
}

@Composable
private fun ShareBanner(count: Int) {
    Card(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 8.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.tertiaryContainer),
        shape = RoundedCornerShape(12.dp),
    ) {
        Text(
            text = "$count file(s) ready to send — pick a peer below.",
            modifier = Modifier.padding(12.dp),
            style = MaterialTheme.typography.bodyMedium,
        )
    }
}

@Composable
private fun SectionHeader(title: String) {
    Text(
        title,
        modifier = Modifier.padding(vertical = 8.dp),
        style = MaterialTheme.typography.titleMedium,
        fontWeight = FontWeight.SemiBold,
    )
}

@Composable
private fun EmptyHint(text: String) {
    Text(
        text,
        modifier = Modifier.padding(vertical = 16.dp),
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SendSheet(
    peer: Peer,
    hasPendingShare: Boolean,
    onSendText: (String) -> Unit,
    onSendFiles: () -> Unit,
    onSendFolder: () -> Unit,
    onDismiss: () -> Unit,
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    var text by remember { mutableStateOf("") }
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = sheetState) {
        Column(modifier = Modifier.fillMaxWidth().padding(16.dp)) {
            Text("Send to ${peer.signature}", style = MaterialTheme.typography.titleMedium)
            Spacer(Modifier.height(16.dp))
            OutlinedTextField(
                value = text,
                onValueChange = { text = it },
                label = { Text("Text snippet") },
                modifier = Modifier.fillMaxWidth(),
                minLines = 2,
                maxLines = 6,
            )
            Spacer(Modifier.height(8.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(
                    onClick = { onSendText(text) },
                    enabled = text.isNotBlank(),
                ) { Text("Send text") }
                OutlinedButton(onClick = onSendFiles) {
                    Text(if (hasPendingShare) "Send shared files" else "Pick file(s)")
                }
                OutlinedButton(onClick = onSendFolder) { Text("Pick folder") }
            }
            Spacer(Modifier.height(16.dp))
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SettingsSheet(
    profile: Profile,
    destLabel: String,
    onBuddyNameChange: (String) -> Unit,
    onPickDestFolder: () -> Unit,
    onClearDestFolder: () -> Unit,
    onDismiss: () -> Unit,
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    var name by remember { mutableStateOf(profile.buddyName) }
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = sheetState) {
        Column(modifier = Modifier.fillMaxWidth().padding(16.dp)) {
            Text("Settings", style = MaterialTheme.typography.titleLarge)
            Spacer(Modifier.height(16.dp))

            // Display name
            OutlinedTextField(
                value = name,
                onValueChange = { name = it },
                label = { Text("Display name") },
                modifier = Modifier.fillMaxWidth(),
                singleLine = true,
            )
            Spacer(Modifier.height(4.dp))
            Text(
                "Empty = use the device name. Changes take effect on the next discovery broadcast.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.height(20.dp))
            HorizontalDivider()
            Spacer(Modifier.height(20.dp))

            // Destination folder
            Text("Destination folder", style = MaterialTheme.typography.titleSmall)
            Spacer(Modifier.height(4.dp))
            Text(
                destLabel,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
            Spacer(Modifier.height(8.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = onPickDestFolder) { Text("Pick folder") }
                OutlinedButton(onClick = onClearDestFolder) { Text("Reset to default") }
            }
            Spacer(Modifier.height(4.dp))
            Text(
                "Default: app private external storage (visible only via this app). " +
                    "Pick a folder to make received files browsable from any file manager.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )

            Spacer(Modifier.height(24.dp))
            Button(onClick = {
                onBuddyNameChange(name)
                onDismiss()
            }, modifier = Modifier.fillMaxWidth()) { Text("Save") }
            Spacer(Modifier.height(16.dp))
        }
    }
}

private fun formatBytes(b: Long): String {
    if (b < 0L) return "?"
    val units = listOf("B", "KB", "MB", "GB")
    var v = b.toDouble()
    var unit = 0
    while (v >= 1024.0 && unit < units.lastIndex) {
        v /= 1024.0; unit++
    }
    return if (unit == 0) "$b B" else "%.1f %s".format(v, units[unit])
}
