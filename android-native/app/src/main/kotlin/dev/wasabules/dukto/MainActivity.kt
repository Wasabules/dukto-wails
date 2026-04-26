package dev.wasabules.dukto

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricPrompt
import androidx.compose.runtime.LaunchedEffect
import androidx.core.content.ContextCompat
import androidx.fragment.app.FragmentActivity
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.foundation.isSystemInDarkTheme
import dev.wasabules.dukto.discovery.Peer
import dev.wasabules.dukto.settings.ThemeMode
import dev.wasabules.dukto.ui.BiometricLockScreen
import dev.wasabules.dukto.ui.DuktoScreen
import dev.wasabules.dukto.ui.PreviewScreen
import dev.wasabules.dukto.ui.theme.DuktoTheme

class MainActivity : FragmentActivity() {

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

    /** Compose-readable flag that drives the lock screen vs the main UI. */
    private var locked by mutableStateOf(false)

    /** Last error shown on the lock screen — null while no prior failure. */
    private var lockError by mutableStateOf<String?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        maybeAskNotificationPermission()
        handleShareIntent(intent)
        // First-render decision: lock immediately if the user has the
        // setting on AND the device can authenticate. Devices that lost
        // their enrolled biometric (e.g. user removed all fingerprints)
        // fall through and we leave the app open — the toggle in Settings
        // will warn them.
        locked = engine.settingsFlow.value.biometricLockEnabled && biometricAvailable()

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
                if (locked) {
                    BiometricLockScreen(
                        errorMessage = lockError,
                        onUnlockTap = { showBiometricPrompt() },
                    )
                    LaunchedEffect(Unit) { showBiometricPrompt() }
                    return@DuktoTheme
                }

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
                        biometricAvailable = biometricAvailable(),
                        onBiometricLockChange = engine::setBiometricLockEnabled,
                        fingerprint = engine.identityFingerprint,
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
                        pinnedFingerprints = settingsState.pinnedPeers.keys,
                        onPinPeer = { peer ->
                            val fp = engine.pinPeer(peer.address)
                            if (fp == null) {
                                android.widget.Toast.makeText(this, "Pair: peer hasn't broadcast its key yet", android.widget.Toast.LENGTH_SHORT).show()
                            }
                        },
                        onUnpinPeer = { fp -> engine.unpinPeer(fp) },
                        onStartPairing = { engine.startPairing() },
                        onCancelPairing = { engine.cancelPairing() },
                        onPairWithPassphrase = { peer, code ->
                            runCatching { engine.pairWithPassphrase(peer.address, peer.port, code) }
                        },
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

    // ── biometric lock ───────────────────────────────────────────────────

    /** Called when the user taps "Unlock" on the lock screen and on first
     *  composition while [locked] is true. Shows the system biometric prompt;
     *  on success flips [locked] to false. On error, leaves the lock screen
     *  visible with the error message so the user can retry. */
    private fun showBiometricPrompt() {
        if (!locked) return
        val executor = ContextCompat.getMainExecutor(this)
        val callback = object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) {
                locked = false
                lockError = null
            }
            override fun onAuthenticationError(errorCode: Int, errString: CharSequence) {
                // ERROR_USER_CANCELED / ERROR_NEGATIVE_BUTTON: user dismissed
                // the prompt — keep the lock screen visible without yelling.
                lockError = if (errorCode == BiometricPrompt.ERROR_USER_CANCELED ||
                    errorCode == BiometricPrompt.ERROR_NEGATIVE_BUTTON) {
                    null
                } else {
                    errString.toString()
                }
            }
        }
        val prompt = BiometricPrompt(this, executor, callback)
        val info = BiometricPrompt.PromptInfo.Builder()
            .setTitle("Unlock Dukto")
            .setSubtitle("Verify your identity to access transfers")
            .setNegativeButtonText("Cancel")
            // Allow weak biometrics (face unlock on most phones) since this
            // is an app-level guard, not a key release.
            .setAllowedAuthenticators(BiometricManager.Authenticators.BIOMETRIC_WEAK)
            .build()
        prompt.authenticate(info)
    }

    /** True if the device has at least one enrolled biometric we can verify
     *  against. Used to decide whether to honour the setting at startup and
     *  to enable/disable the toggle in Settings. */
    private fun biometricAvailable(): Boolean {
        val mgr = BiometricManager.from(this)
        return mgr.canAuthenticate(BiometricManager.Authenticators.BIOMETRIC_WEAK) ==
            BiometricManager.BIOMETRIC_SUCCESS
    }

    override fun onStop() {
        super.onStop()
        // Re-arm the lock whenever the app leaves the foreground. The next
        // time the user comes back to it, BiometricLockScreen pops and the
        // prompt fires automatically.
        if (engine.settingsFlow.value.biometricLockEnabled && biometricAvailable()) {
            locked = true
            lockError = null
        }
    }
}
