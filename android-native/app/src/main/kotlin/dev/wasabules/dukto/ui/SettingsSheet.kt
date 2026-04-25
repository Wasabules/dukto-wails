package dev.wasabules.dukto.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import dev.wasabules.dukto.Profile
import dev.wasabules.dukto.audit.AuditLog
import dev.wasabules.dukto.settings.Settings
import java.text.DateFormat
import java.util.Date

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsSheet(
    profile: Profile,
    destLabel: String,
    settings: Settings,
    audit: List<AuditLog.Entry>,
    onBuddyNameChange: (String) -> Unit,
    onPickDestFolder: () -> Unit,
    onClearDestFolder: () -> Unit,
    onReceivingEnabledChange: (Boolean) -> Unit,
    onConfirmUnknownPeersChange: (Boolean) -> Unit,
    onBlockedExtensionsChange: (Set<String>) -> Unit,
    onMaxSessionSizeChange: (Int) -> Unit,
    onUnblockPeer: (String) -> Unit,
    onForgetApprovals: () -> Unit,
    onClearAudit: () -> Unit,
    onDismiss: () -> Unit,
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    var name by remember { mutableStateOf(profile.buddyName) }
    var blockedExt by remember(settings.blockedExtensions) {
        mutableStateOf(settings.blockedExtensions.joinToString(", "))
    }
    var maxSize by remember(settings.maxSessionSizeMB) {
        mutableStateOf(if (settings.maxSessionSizeMB == 0) "" else settings.maxSessionSizeMB.toString())
    }

    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = sheetState) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp, vertical = 8.dp),
        ) {
            Text("Settings", style = MaterialTheme.typography.titleLarge)
            Spacer(Modifier.height(16.dp))

            // — Profile
            Section("Profile") {
                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it },
                    label = { Text("Display name") },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true,
                )
                Hint("Empty = use the device name. Changes take effect on the next discovery broadcast.")
            }

            // — Destination
            Section("Destination folder") {
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
                Hint("Default: app private external storage. Pick a folder to make received files browsable from any file manager.")
            }

            // — Security
            Section("Security") {
                ToggleRow(
                    title = "Accept incoming transfers",
                    subtitle = "Master switch. When off, all sessions are dropped immediately.",
                    checked = settings.receivingEnabled,
                    onCheckedChange = onReceivingEnabledChange,
                )
                ToggleRow(
                    title = "Confirm new peers",
                    subtitle = "Asks before accepting a session from a buddy you haven't approved yet (60 s timeout).",
                    checked = settings.confirmUnknownPeers,
                    onCheckedChange = onConfirmUnknownPeersChange,
                )
                Spacer(Modifier.height(8.dp))
                OutlinedTextField(
                    value = blockedExt,
                    onValueChange = {
                        blockedExt = it
                        onBlockedExtensionsChange(it.split(',').map { e -> e.trim() }.filter { e -> e.isNotEmpty() }.toSet())
                    },
                    label = { Text("Blocked extensions") },
                    placeholder = { Text("exe, bat, cmd, …") },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true,
                )
                Hint("Comma-separated, no dot. Sessions containing any matching element are aborted.")
                Spacer(Modifier.height(8.dp))
                OutlinedTextField(
                    value = maxSize,
                    onValueChange = {
                        maxSize = it.filter { c -> c.isDigit() }
                        onMaxSessionSizeChange(maxSize.toIntOrNull() ?: 0)
                    },
                    label = { Text("Max session size (MB)") },
                    placeholder = { Text("0 = no limit") },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true,
                )
                Spacer(Modifier.height(8.dp))
                if (settings.approvedPeers.isNotEmpty()) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(
                            "${settings.approvedPeers.size} approved peer(s)",
                            style = MaterialTheme.typography.bodyMedium,
                            modifier = Modifier.weight(1f),
                        )
                        TextButton(onClick = onForgetApprovals) { Text("Forget approvals") }
                    }
                }
                if (settings.blockedPeers.isNotEmpty()) {
                    Spacer(Modifier.height(4.dp))
                    Text("Blocked peers", style = MaterialTheme.typography.titleSmall)
                    settings.blockedPeers.forEach { sig ->
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text(
                                sig,
                                style = MaterialTheme.typography.bodySmall,
                                modifier = Modifier.weight(1f),
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                            )
                            TextButton(onClick = { onUnblockPeer(sig) }) { Text("Unblock") }
                        }
                    }
                }
            }

            // — Audit
            Section("Audit log") {
                if (audit.isEmpty()) {
                    Hint("Accept / reject / policy decisions will appear here.")
                } else {
                    Text(
                        "${audit.size} entr${if (audit.size == 1) "y" else "ies"}",
                        style = MaterialTheme.typography.bodySmall,
                    )
                    Spacer(Modifier.height(8.dp))
                    audit.takeLast(15).asReversed().forEach { e ->
                        AuditRow(e)
                    }
                    Spacer(Modifier.height(4.dp))
                    TextButton(onClick = onClearAudit) { Text("Clear log") }
                }
            }

            Spacer(Modifier.height(8.dp))
            Button(
                onClick = {
                    onBuddyNameChange(name)
                    onDismiss()
                },
                modifier = Modifier.fillMaxWidth(),
            ) { Text("Save") }
            Spacer(Modifier.height(24.dp))
        }
    }
}

@Composable
private fun Section(title: String, content: @Composable () -> Unit) {
    Text(title, style = MaterialTheme.typography.titleMedium)
    Spacer(Modifier.height(8.dp))
    content()
    Spacer(Modifier.height(20.dp))
    HorizontalDivider()
    Spacer(Modifier.height(20.dp))
}

@Composable
private fun ToggleRow(
    title: String,
    subtitle: String,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
) {
    Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.bodyMedium)
            Text(
                subtitle,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Switch(checked = checked, onCheckedChange = onCheckedChange)
    }
    Spacer(Modifier.height(8.dp))
}

@Composable
private fun Hint(text: String) {
    Spacer(Modifier.height(4.dp))
    Text(
        text,
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
}

@Composable
private fun AuditRow(e: AuditLog.Entry) {
    val color = when (e.level) {
        AuditLog.Level.Info -> MaterialTheme.colorScheme.onSurface
        AuditLog.Level.Warn -> MaterialTheme.colorScheme.tertiary
        AuditLog.Level.Reject -> MaterialTheme.colorScheme.error
    }
    val time = DateFormat.getTimeInstance(DateFormat.MEDIUM).format(Date(e.at))
    Column(modifier = Modifier.padding(vertical = 4.dp)) {
        Row {
            Text(time, style = MaterialTheme.typography.labelSmall)
            Spacer(Modifier.height(0.dp))
            Spacer(Modifier.padding(horizontal = 4.dp))
            Text(e.kind, style = MaterialTheme.typography.labelMedium, color = color)
        }
        if (e.peer.isNotBlank() || e.detail.isNotBlank()) {
            Text(
                listOf(e.peer, e.detail).filter { it.isNotBlank() }.joinToString(" — "),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}
