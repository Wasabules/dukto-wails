package dev.wasabules.dukto

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.core.content.ContextCompat
import androidx.compose.foundation.isSystemInDarkTheme
import dev.wasabules.dukto.discovery.Peer
import dev.wasabules.dukto.settings.ThemeMode
import dev.wasabules.dukto.ui.DuktoScreen
import dev.wasabules.dukto.ui.PreviewScreen
import dev.wasabules.dukto.ui.theme.DuktoTheme

class MainActivity : ComponentActivity() {

    private val app: DuktoApp get() = application as DuktoApp
    private val engine: DuktoEngine get() = app.engine

    private var pendingPeer: Peer? = null

    private val pickFiles = registerForActivityResult(
        ActivityResultContracts.OpenMultipleDocuments(),
    ) { uris ->
        val peer = pendingPeer
        pendingPeer = null
        if (peer != null && !uris.isNullOrEmpty()) {
            engine.sendFiles(peer.address.hostAddress.orEmpty(), peer.port, uris)
        }
    }

    private val pickFolderToSend = registerForActivityResult(
        ActivityResultContracts.OpenDocumentTree(),
    ) { uri: Uri? ->
        val peer = pendingPeer
        pendingPeer = null
        if (peer != null && uri != null) {
            engine.sendFolder(peer.address.hostAddress.orEmpty(), peer.port, uri)
        }
    }

    private val pickDestTree = registerForActivityResult(
        ActivityResultContracts.OpenDocumentTree(),
    ) { uri: Uri? ->
        if (uri != null) {
            val flags = Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_WRITE_URI_PERMISSION
            runCatching { contentResolver.takePersistableUriPermission(uri, flags) }
            engine.setDestTreeUri(uri.toString())
        }
    }

    private val pickAvatarImage = registerForActivityResult(
        ActivityResultContracts.OpenDocument(),
    ) { uri: Uri? ->
        if (uri != null) {
            runCatching { engine.setCustomAvatar(uri) }
                .onFailure { android.widget.Toast.makeText(this, "Avatar: ${it.message}", android.widget.Toast.LENGTH_SHORT).show() }
        }
    }

    private val askNotificationPermission = registerForActivityResult(
        ActivityResultContracts.RequestPermission(),
    ) { /* OS retains the answer */ }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        maybeAskNotificationPermission()
        handleShareIntent(intent)

        setContent {
            val settingsState by engine.settingsFlow.collectAsState()
            // Resolve "follow system / force light / force dark" against the
            // current activity configuration. isSystemInDarkTheme() is itself
            // composable so it stays reactive to OS-level dark mode toggles.
            val systemDark = isSystemInDarkTheme()
            val effectiveDark = when (settingsState.themeMode) {
                ThemeMode.System -> systemDark
                ThemeMode.Light -> false
                ThemeMode.Dark -> true
            }
            DuktoTheme(darkTheme = effectiveDark) {
                val auditEntries by engine.audit.entries.collectAsState()
                val peers by engine.peers.collectAsState()
                val activity by engine.activity.collectAsState()
                val inflight by engine.inflight.collectAsState()
                val profile by engine.profile.collectAsState()
                val destLabel by engine.destLabel.collectAsState()
                val pendingRequests by engine.pendingPeerRequests.collectAsState()
                val avatarBytes by engine.avatarBytes.collectAsState()
                val hasCustomAvatar by engine.hasCustomAvatar.collectAsState()

                var pendingShare by remember { mutableStateOf<List<Uri>>(emptyList()) }
                LaunchedEffect(Unit) { pendingShare = consumeSharedUris() }

                // Activity entry currently being previewed (back press / topbar arrow returns null).
                var preview by remember { mutableStateOf<ActivityEntry?>(null) }

                if (preview != null) {
                    PreviewScreen(entry = preview!!, onClose = { preview = null })
                } else {
                    DuktoScreen(
                        profile = profile,
                        settings = settingsState,
                        destLabel = destLabel,
                        audit = auditEntries,
                        peers = peers.values.toList(),
                        activity = activity,
                        inflight = inflight,
                        pendingShare = pendingShare,
                        pendingPeerRequests = pendingRequests,
                        avatarBytes = avatarBytes,
                        hasCustomAvatar = hasCustomAvatar,
                        onPickAvatar = { pickAvatarImage.launch(arrayOf("image/*")) },
                        onClearAvatar = { engine.clearCustomAvatar() },
                        onBuddyNameChange = engine::setBuddyName,
                        onPickDestFolder = { pickDestTree.launch(null) },
                        onClearDestFolder = { engine.setDestTreeUri(null) },
                        onReceivingEnabledChange = engine::setReceivingEnabled,
                        onConfirmUnknownPeersChange = engine::setConfirmUnknownPeers,
                        onBlockedExtensionsChange = engine::setBlockedExtensions,
                        onMaxSessionSizeChange = engine::setMaxSessionSizeMB,
                        onUnblockPeer = engine::unblockPeer,
                        onForgetApprovals = engine::forgetApprovals,
                        onClearAudit = engine::clearAuditLog,
                        onMaxActivityChange = engine::setMaxActivityEntries,
                        onClearActivity = engine::clearActivity,
                        onThemeModeChange = engine::setThemeMode,
                        onResolvePeerRequest = engine::resolvePeerRequest,
                        onSendText = { peer, text ->
                            engine.sendText(peer.address.hostAddress.orEmpty(), peer.port, text)
                        },
                        onSendFiles = { peer ->
                            if (pendingShare.isNotEmpty()) {
                                engine.sendFiles(peer.address.hostAddress.orEmpty(), peer.port, pendingShare)
                                pendingShare = emptyList()
                            } else {
                                pendingPeer = peer
                                pickFiles.launch(arrayOf("*/*"))
                            }
                        },
                        onSendFolder = { peer ->
                            pendingPeer = peer
                            pickFolderToSend.launch(null)
                        },
                        onCancelInflight = { engine.cancelInflight() },
                        onOpenActivity = { entry -> preview = entry },
                    )
                }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        handleShareIntent(intent)
    }

    private fun maybeAskNotificationPermission() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return
        val granted = ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS)
        if (granted != PackageManager.PERMISSION_GRANTED) {
            askNotificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
        }
    }

    private val sharedUris = mutableListOf<Uri>()

    private fun handleShareIntent(intent: Intent?) {
        intent ?: return
        sharedUris.clear()
        when (intent.action) {
            Intent.ACTION_SEND -> intent.data?.let { sharedUris += it } ?: run {
                val u: Uri? = if (Build.VERSION.SDK_INT >= 33) {
                    intent.getParcelableExtra(Intent.EXTRA_STREAM, Uri::class.java)
                } else {
                    @Suppress("DEPRECATION")
                    intent.getParcelableExtra(Intent.EXTRA_STREAM)
                }
                u?.let { sharedUris += it }
            }
            Intent.ACTION_SEND_MULTIPLE -> {
                val list: ArrayList<Uri>? = if (Build.VERSION.SDK_INT >= 33) {
                    intent.getParcelableArrayListExtra(Intent.EXTRA_STREAM, Uri::class.java)
                } else {
                    @Suppress("DEPRECATION")
                    intent.getParcelableArrayListExtra(Intent.EXTRA_STREAM)
                }
                list?.let { sharedUris.addAll(it) }
            }
        }
    }

    private fun consumeSharedUris(): List<Uri> {
        val out = sharedUris.toList()
        sharedUris.clear()
        return out
    }
}
