package dev.wasabules.dukto.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
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
import androidx.compose.ui.draw.clip
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import coil.compose.AsyncImage
import coil.request.ImageRequest
import dev.wasabules.dukto.Profile
import dev.wasabules.dukto.audit.AuditLog
import dev.wasabules.dukto.settings.Settings
import dev.wasabules.dukto.settings.ThemeMode
import java.text.DateFormat
import java.util.Date

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsSheet(
    profile: Profile,
    destLabel: String,
    settings: Settings,
    audit: List<AuditLog.Entry>,
    avatarBytes: ByteArray,
    hasCustomAvatar: Boolean,
    onPickAvatar: () -> Unit,
    onClearAvatar: () -> Unit,
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
    activityCount: Int,
    onMaxActivityChange: (Int) -> Unit,
    onClearActivity: () -> Unit,
    onThemeModeChange: (ThemeMode) -> Unit,
    biometricAvailable: Boolean,
    onBiometricLockChange: (Boolean) -> Unit,
    fingerprint: String,
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
    var maxActivity by remember(settings.maxActivityEntries) {
        mutableStateOf(if (settings.maxActivityEntries == 0) "" else settings.maxActivityEntries.toString())
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
                AvatarRow(
                    bytes = avatarBytes,
                    hasCustom = hasCustomAvatar,
                    onPick = onPickAvatar,
                    onClear = onClearAvatar,
                )
                Spacer(Modifier.height(12.dp))
                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it },
                    label = { Text("Display name") },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true,
                )
                Hint("Empty = use the device name. Changes take effect on the next discovery broadcast.")
                if (fingerprint.isNotEmpty()) {
                    Spacer(Modifier.height(12.dp))
                    FingerprintRow(fingerprint)
                }
            }

            // — Appearance
            Section("Appearance") {
                Text(
                    "Theme",
                    style = MaterialTheme.typography.bodyMedium,
                )
                Spacer(Modifier.height(6.dp))
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    ThemeModeChip("System", settings.themeMode == ThemeMode.System) { onThemeModeChange(ThemeMode.System) }
                    ThemeModeChip("Light", settings.themeMode == ThemeMode.Light) { onThemeModeChange(ThemeMode.Light) }
                    ThemeModeChip("Dark", settings.themeMode == ThemeMode.Dark) { onThemeModeChange(ThemeMode.Dark) }
                }
                Hint("System follows your device's dark/light mode. Light/Dark force a specific theme regardless of the system setting.")
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
                ToggleRow(
                    title = "Biometric unlock",
                    subtitle = if (biometricAvailable)
                        "Verify your fingerprint or face every time the app comes to the foreground."
                    else
                        "Disabled — no fingerprint or face is enrolled on this device.",
                    checked = settings.biometricLockEnabled && biometricAvailable,
                    onCheckedChange = { v ->
                        if (biometricAvailable) onBiometricLockChange(v)
                    },
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

            // — Recent activity
            Section("Recent activity") {
                Text(
                    "$activityCount entr${if (activityCount == 1) "y" else "ies"} kept",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.height(8.dp))
                OutlinedTextField(
                    value = maxActivity,
                    onValueChange = {
                        maxActivity = it.filter { c -> c.isDigit() }
                        onMaxActivityChange(maxActivity.toIntOrNull() ?: 0)
                    },
                    label = { Text("Max entries") },
                    placeholder = { Text("0 = unlimited") },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true,
                )
                Hint("Older entries are dropped first when the cap is reached.")
                Spacer(Modifier.height(8.dp))
                TextButton(
                    onClick = onClearActivity,
                    enabled = activityCount > 0,
                ) { Text("Clear recent activity") }
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

/** Identity fingerprint row. A 16-char base32 code (XXXX-XXXX-XXXX-XXXX) +
 *  Copy button. Surfaces the long-term Ed25519 key — same string the Wails
 *  desktop shows in its General settings. Used today as a stable per-install
 *  identifier (survives buddy-name renames); will anchor encrypted transfers
 *  in a future release. See docs/SECURITY_v2.md. */
@Composable
private fun FingerprintRow(fingerprint: String) {
    val ctx = LocalContext.current
    Text("Identity fingerprint", style = MaterialTheme.typography.titleSmall)
    Spacer(Modifier.height(6.dp))
    Row(verticalAlignment = Alignment.CenterVertically) {
        Box(
            modifier = Modifier
                .weight(1f)
                .clip(RoundedCornerShape(8.dp))
                .background(MaterialTheme.colorScheme.surfaceVariant)
                .padding(horizontal = 12.dp, vertical = 8.dp),
        ) {
            Text(
                fingerprint,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
        Spacer(Modifier.size(8.dp))
        TextButton(onClick = {
            val cm = ctx.getSystemService(android.content.Context.CLIPBOARD_SERVICE)
                as android.content.ClipboardManager
            cm.setPrimaryClip(android.content.ClipData.newPlainText("Dukto fingerprint", fingerprint))
        }) { Text("Copy") }
    }
    Hint(
        "Long-term Ed25519 key generated for this install. Used today only as a stable " +
            "ID; will anchor encrypted transfers in a future release.",
    )
}

/** Compact toggle button for the theme triplet. Material 3's FilterChip felt
 *  too heavy here — these need to fit on one line on phones in portrait. */
@Composable
private fun ThemeModeChip(label: String, selected: Boolean, onClick: () -> Unit) {
    if (selected) {
        Button(onClick = onClick) { Text(label) }
    } else {
        OutlinedButton(onClick = onClick) { Text(label) }
    }
}

@Composable
private fun AvatarRow(
    bytes: ByteArray,
    hasCustom: Boolean,
    onPick: () -> Unit,
    onClear: () -> Unit,
) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Box(
            modifier = Modifier
                .size(56.dp)
                .clip(RoundedCornerShape(10.dp))
                .background(MaterialTheme.colorScheme.surfaceVariant),
            contentAlignment = Alignment.Center,
        ) {
            // Coil's data() handles ByteArray directly. The PNG bytes are
            // re-keyed on every change (clearing the StateFlow → new array
            // identity → fresh request), so previous custom uploads don't
            // linger in the in-memory cache.
            AsyncImage(
                model = ImageRequest.Builder(LocalContext.current)
                    .data(bytes)
                    .crossfade(true)
                    .build(),
                contentDescription = "Your avatar",
                contentScale = ContentScale.Crop,
                modifier = Modifier.fillMaxSize(),
            )
        }
        Spacer(Modifier.size(16.dp))
        Column(modifier = Modifier.weight(1f)) {
            Button(onClick = onPick) { Text("Pick image…") }
            if (hasCustom) {
                Spacer(Modifier.height(4.dp))
                TextButton(onClick = onClear) { Text("Reset to initials") }
            }
        }
    }
    Spacer(Modifier.height(4.dp))
    Text(
        if (hasCustom)
            "Custom image — peers fetch it via the avatar HTTP endpoint."
        else
            "Auto-generated from your buddy name. Pick a PNG / JPEG / GIF / WebP to override.",
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
