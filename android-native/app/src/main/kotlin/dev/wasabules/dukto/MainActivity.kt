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

    private val pickFiles = registerForActivityResult(
        ActivityResultContracts.OpenMultipleDocuments(),
    ) { uris ->
        uris?.let { selectedUris ->
            pendingPeer?.let { peer ->
                engine.sendFiles(peer.address.hostAddress.orEmpty(), peer.port, selectedUris)
            }
        }
        pendingPeer = null
    }

    private val askNotificationPermission = registerForActivityResult(
        ActivityResultContracts.RequestPermission(),
    ) { /* answer ignored — Material 3 informs the user via OS dialog */ }

    /** Set just before launching the file picker; used to know which peer to send to on result. */
    private var pendingPeer: Peer? = null

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

                var pendingShare by remember { mutableStateOf<List<Uri>>(emptyList()) }
                LaunchedEffect(Unit) {
                    pendingShare = consumeSharedUris()
                }

                DuktoScreen(
                    profile = profile,
                    peers = peers.values.toList(),
                    activity = activity,
                    inflight = inflight,
                    pendingShare = pendingShare,
                    onBuddyNameChange = engine::setBuddyName,
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

    /** Capture URIs from a share intent, if any; consumed on the next composition. */
    private val sharedUris = mutableListOf<Uri>()

    private fun handleShareIntent(intent: Intent?) {
        intent ?: return
        sharedUris.clear()
        when (intent.action) {
            Intent.ACTION_SEND -> intent.data?.let { sharedUris += it } ?: run {
                @Suppress("DEPRECATION")
                val u: Uri? = if (Build.VERSION.SDK_INT >= 33) {
                    intent.getParcelableExtra(Intent.EXTRA_STREAM, Uri::class.java)
                } else intent.getParcelableExtra(Intent.EXTRA_STREAM)
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
