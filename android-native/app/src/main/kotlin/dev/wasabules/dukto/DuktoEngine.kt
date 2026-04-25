package dev.wasabules.dukto

import android.content.Context
import android.net.Uri
import dev.wasabules.dukto.discovery.Messenger
import dev.wasabules.dukto.discovery.Peer as DiscoveryPeer
import dev.wasabules.dukto.platform.currentSignature
import dev.wasabules.dukto.transfer.Peer as TransferPeer
import dev.wasabules.dukto.transfer.Sender
import dev.wasabules.dukto.transfer.Server
import dev.wasabules.dukto.transfer.TransferEvent
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.merge
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.File

/**
 * Process-wide orchestration: discovery + TCP server + sender, plus the
 * persistent UI state (current signature, recent transfers, in-flight
 * transfer status).
 *
 * Held by [DuktoApp] (the Application class) so it survives configuration
 * changes and Activity recreation. Stop is called from
 * [DuktoApp.onTerminate]; in practice that's a hint Android may not honour,
 * so the foreground service in [TransferService] is what actually anchors
 * the lifecycle while a transfer is in flight.
 */
class DuktoEngine(private val app: Context) {

    private val scope = CoroutineScope(Dispatchers.Default + SupervisorJob())

    private val _profile = MutableStateFlow(Profile(buddyName = ""))
    val profile: StateFlow<Profile> = _profile.asStateFlow()

    private val _activity = MutableStateFlow<List<ActivityEntry>>(emptyList())
    val activity: StateFlow<List<ActivityEntry>> = _activity.asStateFlow()

    private val _inflight = MutableStateFlow<InflightTransfer?>(null)
    val inflight: StateFlow<InflightTransfer?> = _inflight.asStateFlow()

    val messenger: Messenger = Messenger(
        context = app,
        signatureProvider = { currentSignature(app, _profile.value.buddyName) },
    )
    val peers = messenger.peers

    private val server = Server(context = app)
    private val sender = Sender(context = app)

    fun start() {
        messenger.start()
        server.start()
        // Pump server + sender events into the activity log + inflight state.
        scope.launch {
            merge(server.events, sender.events).collect(::onTransferEvent)
        }
        // First HELLO right away.
        messenger.sayHello()
    }

    fun stop() {
        messenger.stop()
        server.stop()
        sender.close()
        scope.coroutineContext[kotlinx.coroutines.Job]?.cancel()
    }

    // ── public API ───────────────────────────────────────────────────────────

    fun setBuddyName(name: String) {
        _profile.update { it.copy(buddyName = name) }
        // Rebroadcast so peers learn the new name without waiting for the next tick.
        messenger.sayHello()
    }

    fun sendText(toAddress: String, port: Int, text: String) {
        val peer = TransferPeer(java.net.InetAddress.getByName(toAddress), port)
        sender.sendText(peer, text)
    }

    fun sendFiles(toAddress: String, port: Int, uris: List<Uri>) {
        val peer = TransferPeer(java.net.InetAddress.getByName(toAddress), port)
        sender.sendFiles(peer, uris)
    }

    // ── event handling ───────────────────────────────────────────────────────

    private fun onTransferEvent(ev: TransferEvent) {
        when (ev) {
            is TransferEvent.Started -> _inflight.value =
                InflightTransfer(ev.from.hostAddress.orEmpty(), ev.totalSize, 0L, isReceive = true)
            is TransferEvent.Progress -> _inflight.update {
                it?.copy(bytesDone = ev.bytesReceived, totalBytes = ev.totalSize)
            }
            is TransferEvent.TextReceived -> {
                _inflight.value = null
                appendActivity(
                    ActivityEntry.TextReceived(
                        from = ev.from.hostAddress.orEmpty(),
                        text = ev.text,
                        at = System.currentTimeMillis(),
                    ),
                )
            }
            is TransferEvent.FilesReceived -> {
                _inflight.value = null
                appendActivity(
                    ActivityEntry.FilesReceived(
                        from = ev.from.hostAddress.orEmpty(),
                        rootDir = ev.rootDir,
                        fileCount = ev.fileCount,
                        at = System.currentTimeMillis(),
                    ),
                )
            }
            is TransferEvent.Failed -> {
                _inflight.value = null
                appendActivity(
                    ActivityEntry.Error(
                        peer = ev.from?.hostAddress.orEmpty(),
                        message = ev.reason,
                        at = System.currentTimeMillis(),
                    ),
                )
            }
            is TransferEvent.Sent -> appendActivity(
                ActivityEntry.Sent(
                    to = ev.to.address.hostAddress.orEmpty(),
                    bytes = ev.totalSize,
                    at = System.currentTimeMillis(),
                ),
            )
            is TransferEvent.SendFailed -> appendActivity(
                ActivityEntry.Error(
                    peer = ev.to.address.hostAddress.orEmpty(),
                    message = "send: ${ev.reason}",
                    at = System.currentTimeMillis(),
                ),
            )
        }
    }

    private fun appendActivity(entry: ActivityEntry) {
        _activity.update { (listOf(entry) + it).take(64) }
    }
}

// ── UI state types ───────────────────────────────────────────────────────────

data class Profile(val buddyName: String)

data class InflightTransfer(
    val peer: String,
    val totalBytes: Long,
    val bytesDone: Long,
    val isReceive: Boolean,
)

sealed interface ActivityEntry {
    val at: Long

    data class TextReceived(val from: String, val text: String, override val at: Long) : ActivityEntry
    data class FilesReceived(val from: String, val rootDir: File, val fileCount: Int, override val at: Long) : ActivityEntry
    data class Sent(val to: String, val bytes: Long, override val at: Long) : ActivityEntry
    data class Error(val peer: String, val message: String, override val at: Long) : ActivityEntry
}

/** Convenience for UI code that uses the discovery [DiscoveryPeer] directly. */
fun DiscoveryPeer.toTransferPeer(): TransferPeer = TransferPeer(address, port)
