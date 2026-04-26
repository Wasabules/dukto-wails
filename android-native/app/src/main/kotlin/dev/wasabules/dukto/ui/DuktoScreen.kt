package dev.wasabules.dukto.ui

import android.net.Uri
import android.content.Intent
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
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.foundation.border
import androidx.compose.ui.graphics.Color
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
import androidx.compose.ui.draw.clip
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import coil.compose.AsyncImage
import coil.request.ImageRequest
import dev.wasabules.dukto.ActivityEntry
import dev.wasabules.dukto.InflightTransfer
import dev.wasabules.dukto.Profile
import dev.wasabules.dukto.audit.AuditLog
import dev.wasabules.dukto.discovery.Peer
import dev.wasabules.dukto.policy.PeerChoice
import dev.wasabules.dukto.policy.PendingRequest
import dev.wasabules.dukto.settings.Settings
import java.text.DateFormat
import java.util.Date

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DuktoScreen(
    profile: Profile,
    settings: Settings,
    destLabel: String,
    audit: List<AuditLog.Entry>,
    peers: List<Peer>,
    activity: List<ActivityEntry>,
    inflight: InflightTransfer?,
    pendingShare: List<Uri>,
    pendingPeerRequests: List<PendingRequest>,
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
    onMaxActivityChange: (Int) -> Unit,
    onClearActivity: () -> Unit,
    onThemeModeChange: (dev.wasabules.dukto.settings.ThemeMode) -> Unit,
    biometricAvailable: Boolean,
    onBiometricLockChange: (Boolean) -> Unit,
    fingerprint: String,
    onResolvePeerRequest: (String, PeerChoice) -> Unit,
    onSendText: (Peer, String) -> Unit,
    onSendFiles: (Peer) -> Unit,
    onSendFolder: (Peer) -> Unit,
    onCancelInflight: () -> Unit,
    onOpenActivity: (ActivityEntry) -> Unit,
    /** Map fingerprint → true when the peer is in the local TOFU table.
     *  Empty when v2 is disabled. */
    pinnedFingerprints: Set<String> = emptySet(),
    /** Pin/unpin callbacks; null disables the trust button. */
    onPinPeer: ((Peer) -> Unit)? = null,
    onUnpinPeer: ((String) -> Unit)? = null,
    /** PSK pairing flow callbacks; null disables the 🤝 button. The
     *  pair-with-passphrase callback is suspend because Socket I/O is
     *  forbidden on the main thread — the caller dispatches it to
     *  Dispatchers.IO. */
    onStartPairing: (() -> String)? = null,
    onCancelPairing: (() -> Unit)? = null,
    onPairWithPassphrase: (suspend (Peer, String) -> Result<Unit>)? = null,
    /** Toggles the "refuse cleartext" backend flag (Settings sheet). */
    onRefuseCleartextChange: (Boolean) -> Unit = {},
    /** Toggles the "hide from discovery" backend flag (Settings sheet). */
    onHideFromDiscoveryChange: (Boolean) -> Unit = {},
    /** Manual-peer add: returns the canonical form on success, null on
     *  parse failure. The dialog uses the return to clear the input
     *  field or show a validation error. */
    onAddManualPeer: (String) -> String? = { null },
    /** Manual-peer remove by canonical form. */
    onRemoveManualPeer: (String) -> Unit = {},
) {
    var settingsOpen by remember { mutableStateOf(false) }
    var sendSheetPeer by remember { mutableStateOf<Peer?>(null) }
    var pairingPeer by remember { mutableStateOf<Peer?>(null) }
    var trustConfirmPeer by remember { mutableStateOf<Peer?>(null) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Dukto Native") },
                navigationIcon = {
                    Box(
                        modifier = Modifier
                            .padding(start = 12.dp)
                            .size(36.dp)
                            .clip(RoundedCornerShape(8.dp))
                            .background(MaterialTheme.colorScheme.surfaceVariant),
                    ) {
                        AsyncImage(
                            model = ImageRequest.Builder(LocalContext.current)
                                .data(avatarBytes)
                                .crossfade(true)
                                .build(),
                            contentDescription = "Your avatar",
                            contentScale = ContentScale.Crop,
                            modifier = Modifier.fillMaxSize(),
                        )
                    }
                },
                actions = {
                    if (!settings.receivingEnabled) {
                        Text(
                            "OFF",
                            color = MaterialTheme.colorScheme.error,
                            style = MaterialTheme.typography.labelSmall,
                            modifier = Modifier.padding(end = 8.dp),
                        )
                    }
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
            if (pendingShare.isNotEmpty()) ShareBanner(count = pendingShare.size)
            inflight?.let { TransferBar(it, onCancel = onCancelInflight) }

            SectionHeader("Peers on your network")
            if (peers.isEmpty()) {
                EmptyHint("Looking for peers… open Dukto on another device on the same Wi-Fi.")
            } else {
                LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    items(peers, key = { it.key }) { peer ->
                        val paired = peer.fingerprint.isNotEmpty() &&
                            peer.fingerprint in pinnedFingerprints
                        PeerRow(
                            peer = peer,
                            paired = paired,
                            onClick = { sendSheetPeer = peer },
                            // Paired → menu offers Unpair (direct, no
                            // confirmation; the user can re-pair anytime).
                            onTogglePair = if (paired && onUnpinPeer != null && peer.fingerprint.isNotEmpty()) {
                                { onUnpinPeer.invoke(peer.fingerprint) }
                            } else null,
                            // Unpaired → "Trust as-is" goes through the
                            // confirmation dialog so the user reads the
                            // TOFU caveats before pinning.
                            onStartTrustConfirm = if (
                                !paired && peer.v2Capable && peer.fingerprint.isNotEmpty() && onPinPeer != null
                            ) {
                                { trustConfirmPeer = peer }
                            } else null,
                            onStartPskPair = if (
                                !paired && peer.v2Capable && peer.fingerprint.isNotEmpty() &&
                                onStartPairing != null && onPairWithPassphrase != null
                            ) {
                                { pairingPeer = peer }
                            } else null,
                        )
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
                        ActivityRow(entry, onClick = { onOpenActivity(entry) })
                    }
                }
            }
        }

        if (settingsOpen) {
            SettingsSheet(
                profile = profile,
                destLabel = destLabel,
                settings = settings,
                audit = audit,
                avatarBytes = avatarBytes,
                hasCustomAvatar = hasCustomAvatar,
                onPickAvatar = onPickAvatar,
                onClearAvatar = onClearAvatar,
                onBuddyNameChange = onBuddyNameChange,
                onPickDestFolder = onPickDestFolder,
                onClearDestFolder = onClearDestFolder,
                onReceivingEnabledChange = onReceivingEnabledChange,
                onConfirmUnknownPeersChange = onConfirmUnknownPeersChange,
                onBlockedExtensionsChange = onBlockedExtensionsChange,
                onMaxSessionSizeChange = onMaxSessionSizeChange,
                onUnblockPeer = onUnblockPeer,
                onForgetApprovals = onForgetApprovals,
                onClearAudit = onClearAudit,
                activityCount = activity.size,
                onMaxActivityChange = onMaxActivityChange,
                onClearActivity = onClearActivity,
                onThemeModeChange = onThemeModeChange,
                biometricAvailable = biometricAvailable,
                onBiometricLockChange = onBiometricLockChange,
                fingerprint = fingerprint,
                onRefuseCleartextChange = onRefuseCleartextChange,
                onHideFromDiscoveryChange = onHideFromDiscoveryChange,
                onUnpinPeer = onUnpinPeer ?: {},
                onAddManualPeer = onAddManualPeer,
                onRemoveManualPeer = onRemoveManualPeer,
                onDismiss = { settingsOpen = false },
            )
        }

        sendSheetPeer?.let { peer ->
            val peerPaired = peer.fingerprint.isNotEmpty() &&
                peer.fingerprint in pinnedFingerprints
            SendSheet(
                peer = peer,
                paired = peerPaired,
                hasPendingShare = pendingShare.isNotEmpty(),
                onSendText = { text -> onSendText(peer, text); sendSheetPeer = null },
                onSendFiles = { onSendFiles(peer); sendSheetPeer = null },
                onSendFolder = { onSendFolder(peer); sendSheetPeer = null },
                onDismiss = { sendSheetPeer = null },
            )
        }

        // Stack pending-peer modals on top of everything else.
        pendingPeerRequests.firstOrNull()?.let { req ->
            PendingPeerDialog(request = req, onChoice = onResolvePeerRequest)
        }

        pairingPeer?.let { p ->
            val gen = onStartPairing
            val sub = onPairWithPassphrase
            if (gen != null && sub != null) {
                PairingDialog(
                    peerLabel = p.signature,
                    onGenerate = gen,
                    onSubmitCode = { code -> sub(p, code) },
                    onCancel = {
                        onCancelPairing?.invoke()
                        pairingPeer = null
                    },
                )
            }
        }

        trustConfirmPeer?.let { p ->
            TrustFingerprintDialog(
                peerLabel = p.signature,
                fingerprint = p.fingerprint,
                onConfirm = {
                    onPinPeer?.invoke(p)
                    trustConfirmPeer = null
                },
                onCancel = { trustConfirmPeer = null },
            )
        }
    }
}

// ── pieces ───────────────────────────────────────────────────────────────────

@Composable
private fun PeerRow(
    peer: Peer,
    paired: Boolean = false,
    onClick: () -> Unit,
    onTogglePair: (() -> Unit)? = null,
    onStartPskPair: (() -> Unit)? = null,
    onStartTrustConfirm: (() -> Unit)? = null,
) {
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
            // Encryption state is communicated by a colored ring around the
            // avatar (green = paired/encrypted, amber = pairable, none for
            // v1 peers). No emojis on the row itself — the actions live in
            // a single overflow menu so the card doesn't read like a row of
            // mystery glyphs.
            val ringColor = when {
                paired -> MaterialTheme.colorScheme.primary
                peer.v2Capable -> MaterialTheme.colorScheme.tertiary
                else -> Color.Transparent
            }
            Box(
                modifier = Modifier
                    .size(48.dp)
                    .border(
                        width = if (ringColor == Color.Transparent) 0.dp else 2.dp,
                        color = ringColor,
                        shape = CircleShape,
                    )
                    .padding(if (ringColor == Color.Transparent) 0.dp else 3.dp),
            ) {
                PeerAvatar(peer)
            }
            Spacer(Modifier.size(16.dp))
            Column(Modifier.weight(1f)) {
                Text(
                    peer.signature,
                    style = MaterialTheme.typography.titleMedium,
                )
                Text(
                    peer.address.hostAddress.orEmpty(),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                if (peer.v2Capable) {
                    val statusLabel = if (paired) "🔒 Encrypted" else "Encryption available"
                    val statusColor = if (paired)
                        MaterialTheme.colorScheme.primary
                    else
                        MaterialTheme.colorScheme.tertiary
                    Text(
                        statusLabel,
                        style = MaterialTheme.typography.labelMedium,
                        color = statusColor,
                    )
                    if (peer.fingerprint.isNotEmpty()) {
                        Text(
                            peer.fingerprint,
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
            if (onStartPskPair != null || onTogglePair != null || onStartTrustConfirm != null) {
                PeerActionMenu(
                    paired = paired,
                    onStartPskPair = onStartPskPair,
                    onStartTrustConfirm = onStartTrustConfirm,
                    onTogglePair = onTogglePair,
                )
            }
        }
    }
}

/**
 * Single overflow button replacing the previous 🤝 + 🔑 IconButton stack.
 * Items have explicit text labels so the action is obvious — no more
 * three-emoji-row guessing game.
 */
@Composable
private fun PeerActionMenu(
    paired: Boolean,
    onStartPskPair: (() -> Unit)?,
    onStartTrustConfirm: (() -> Unit)?,
    onTogglePair: (() -> Unit)?,
) {
    var open by remember { mutableStateOf(false) }
    Box {
        IconButton(onClick = { open = true }) {
            Icon(Icons.Default.MoreVert, contentDescription = "Encryption options")
        }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            if (paired) {
                if (onTogglePair != null) {
                    DropdownMenuItem(
                        text = { Text("Unpair") },
                        onClick = { open = false; onTogglePair() },
                    )
                }
            } else {
                if (onStartPskPair != null) {
                    DropdownMenuItem(
                        text = { Text("Pair via 5-word code…") },
                        onClick = { open = false; onStartPskPair() },
                    )
                }
                if (onStartTrustConfirm != null) {
                    DropdownMenuItem(
                        text = { Text("Trust fingerprint as-is") },
                        onClick = { open = false; onStartTrustConfirm() },
                    )
                }
            }
        }
    }
}

/**
 * Peer's avatar tile: try to fetch their HTTP side-channel
 * (`http://<peer>:<port+1>/dukto/avatar`); fall back to a deterministic
 * initials tile while loading, on network failure, or if the peer doesn't
 * run the avatar endpoint.
 *
 * The initials layer underneath stays visible until Coil flips the
 * AsyncImage on top — gives a smooth "initials → photo" transition with no
 * empty frame.
 */
@Composable
private fun PeerAvatar(peer: Peer) {
    val avatarPort = peer.port + 1
    val url = "http://${peer.address.hostAddress}:$avatarPort/dukto/avatar"
    Box(
        modifier = Modifier
            .size(40.dp)
            .clip(CircleShape),
        contentAlignment = Alignment.Center,
    ) {
        Avatar(seedText = peer.signature)
        AsyncImage(
            model = ImageRequest.Builder(LocalContext.current)
                .data(url)
                // Re-fetch when the peer renames themselves (proxy for
                // "they may have changed their avatar"). Without this,
                // Coil's URL cache keeps serving the stale image.
                .memoryCacheKey("avatar:${peer.address.hostAddress}:$avatarPort:${peer.signature}")
                .diskCacheKey("avatar:${peer.address.hostAddress}:$avatarPort:${peer.signature}")
                .crossfade(true)
                .build(),
            contentDescription = "${peer.signature} avatar",
            contentScale = ContentScale.Crop,
            modifier = Modifier.fillMaxSize(),
        )
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
private fun ActivityRow(entry: ActivityEntry, onClick: () -> Unit) {
    val time = DateFormat.getTimeInstance(DateFormat.SHORT).format(Date(entry.at))
    val tappable = entry is ActivityEntry.TextReceived || entry is ActivityEntry.FilesReceived
    Card(
        modifier = Modifier
            .fillMaxWidth()
            .let { if (tappable) it.clickable(onClick = onClick) else it },
        shape = RoundedCornerShape(12.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
    ) {
        Column(Modifier.padding(12.dp)) {
            // Header: title + time. Same shape for every entry kind.
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    text = activityTitle(entry),
                    style = MaterialTheme.typography.titleSmall,
                    modifier = Modifier.weight(1f),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                if (entry.encrypted) {
                    Text(
                        "🔒",
                        style = MaterialTheme.typography.labelSmall,
                        modifier = Modifier.padding(horizontal = 4.dp),
                    )
                }
                Text(time, style = MaterialTheme.typography.labelSmall)
            }

            when (entry) {
                is ActivityEntry.TextReceived -> {
                    Spacer(Modifier.height(4.dp))
                    Text(
                        entry.text,
                        style = MaterialTheme.typography.bodyMedium,
                        maxLines = 4,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
                is ActivityEntry.FilesReceived -> {
                    Spacer(Modifier.height(8.dp))
                    FilesReceivedBody(entry)
                }
                is ActivityEntry.Sent -> Unit
                is ActivityEntry.Error -> {
                    Spacer(Modifier.height(4.dp))
                    Text(
                        entry.message,
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.error,
                        maxLines = 4,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            }
        }
    }
}

private fun activityTitle(entry: ActivityEntry): String = when (entry) {
    is ActivityEntry.TextReceived -> "Text from ${entry.from}"
    is ActivityEntry.FilesReceived -> "${entry.fileCount} file(s) from ${entry.from}"
    is ActivityEntry.Sent -> "Sent ${formatBytes(entry.bytes)} to ${entry.to}"
    is ActivityEntry.Error -> "Error: ${entry.peer}"
}

/** Shows up to [INLINE_FILE_LIMIT] files with thumbnail + name + size. Tap a
 *  row to open that file in the system viewer; tap the surrounding card to
 *  jump to the full preview screen. */
@Composable
private fun FilesReceivedBody(entry: ActivityEntry.FilesReceived) {
    val visibleUris = entry.fileUris.take(INLINE_FILE_LIMIT)
    val ctx = LocalContext.current
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        visibleUris.forEach { u ->
            FileMetaRow(
                uri = u,
                onTap = { meta -> openInExternalViewer(ctx, meta) },
            )
        }
        val remaining = entry.fileUris.size - visibleUris.size
        if (remaining > 0) {
            Text(
                "+ $remaining more — tap to view all",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        } else if (entry.fileUris.isEmpty()) {
            // Pre-Coil entries (or sessions with no captured URIs) — fall back
            // to showing the destination so the user still has a pointer.
            Text(
                entry.location,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

@Composable
private fun FileMetaRow(uri: String, onTap: (FileMeta) -> Unit) {
    val meta = rememberFileMeta(uri)
    if (meta == null) {
        Text(
            uri,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
        return
    }
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable { onTap(meta) },
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(
            modifier = Modifier
                .size(44.dp)
                .clip(RoundedCornerShape(8.dp))
                .background(MaterialTheme.colorScheme.surfaceVariant),
            contentAlignment = Alignment.Center,
        ) {
            if (meta.isImage) {
                AsyncImage(
                    model = ImageRequest.Builder(LocalContext.current)
                        .data(meta.uri)
                        .crossfade(true)
                        .build(),
                    contentDescription = meta.name,
                    contentScale = ContentScale.Crop,
                    modifier = Modifier.fillMaxSize(),
                )
            } else {
                Text(genericFileEmoji(meta.mime), style = MaterialTheme.typography.titleMedium)
            }
        }
        Spacer(Modifier.size(12.dp))
        Column(Modifier.weight(1f)) {
            Text(
                meta.name,
                style = MaterialTheme.typography.bodyMedium,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                listOfNotNull(formatBytesShort(meta.size), meta.mime).joinToString(" · "),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

private fun genericFileEmoji(mime: String?): String = when {
    mime == null -> "📄"
    mime.startsWith("video/") -> "🎬"
    mime.startsWith("audio/") -> "🎵"
    mime.startsWith("text/") -> "📝"
    mime == "application/pdf" -> "📕"
    mime.startsWith("application/zip") || mime.contains("compressed") -> "🗜"
    else -> "📄"
}

private fun openInExternalViewer(ctx: android.content.Context, meta: FileMeta) {
    val intent = Intent(Intent.ACTION_VIEW).apply {
        setDataAndType(meta.uri, meta.mime ?: "*/*")
        addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
    }
    runCatching { ctx.startActivity(Intent.createChooser(intent, "Open with")) }
}

private const val INLINE_FILE_LIMIT = 5

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
    paired: Boolean = false,
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
            if (!paired) {
                Spacer(Modifier.height(8.dp))
                CleartextWarning(v2Capable = peer.v2Capable)
            }
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

/**
 * Inline warning displayed inside the send sheet when the destination
 * peer isn't paired. The two flavours read differently:
 *   - v2Capable: a paired-eligible peer that the user just hasn't
 *     paired with yet — actionable, point at the peer-card menu.
 *   - !v2Capable: a v1 peer that can't do encryption at all — frame
 *     it as an unavoidable cleartext send.
 */
@Composable
private fun CleartextWarning(v2Capable: Boolean) {
    val container = androidx.compose.ui.graphics.Color(0xFFFFE9B3)
    val onContainer = androidx.compose.ui.graphics.Color(0xFF5C3B00)
    Surface(
        color = container,
        shape = RoundedCornerShape(8.dp),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp)) {
            Text(
                "⚠️  Cleartext",
                style = MaterialTheme.typography.titleSmall,
                color = onContainer,
            )
            Spacer(Modifier.height(2.dp))
            Text(
                if (v2Capable) {
                    "This peer supports encryption but isn't paired yet. " +
                        "Pair via the ⋮ menu on the peer card to encrypt future sends."
                } else {
                    "This peer doesn't run a v2 Dukto — bytes go on the wire " +
                        "in clear. Anyone on the same LAN can read them."
                },
                style = MaterialTheme.typography.bodySmall,
                color = onContainer,
            )
        }
    }
}

private fun formatBytes(b: Long): String {
    if (b < 0L) return "?"
    val units = listOf("B", "KB", "MB", "GB")
    var v = b.toDouble()
    var unit = 0
    while (v >= 1024.0 && unit < units.lastIndex) { v /= 1024.0; unit++ }
    return if (unit == 0) "$b B" else "%.1f %s".format(v, units[unit])
}
