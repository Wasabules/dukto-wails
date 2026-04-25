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
import dev.wasabules.dukto.discovery.Peer
import dev.wasabules.dukto.ui.DuktoScreen
import dev.wasabules.dukto.ui.theme.DuktoTheme

class MainActivity : ComponentActivity() {

    private val app: DuktoApp get() = application as DuktoApp
    private val engine: DuktoEngine get() = app.engine

    /** Peer queued by the most recent action that needs an external picker result. */
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
            // Persist read+write so we can write to it after process death.
            val flags = Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_WRITE_URI_PERMISSION
            runCatching { contentResolver.takePersistableUriPermission(uri, flags) }
            engine.setDestTreeUri(uri.toString())
        }
    }

    private val askNotificationPermission = registerForActivityResult(
        ActivityResultContracts.RequestPermission(),
    ) { /* the OS retains the answer; nothing to do here */ }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        maybeAskNotificationPermission()
        handleShareIntent(intent)

        setContent {
            DuktoTheme {
                val peers by engine.peers.collectAsState()
                val activity by engine.activity.collectAsState()
                val inflight by engine.inflight.collectAsState()
                val profile by engine.profile.collectAsState()
                val destLabel by engine.destLabel.collectAsState()

                var pendingShare by remember { mutableStateOf<List<Uri>>(emptyList()) }
                LaunchedEffect(Unit) { pendingShare = consumeSharedUris() }

                DuktoScreen(
                    profile = profile,
                    destLabel = destLabel,
                    peers = peers.values.toList(),
                    activity = activity,
                    inflight = inflight,
                    pendingShare = pendingShare,
                    onBuddyNameChange = engine::setBuddyName,
                    onPickDestFolder = { pickDestTree.launch(null) },
                    onClearDestFolder = { engine.setDestTreeUri(null) },
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
                )
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
