package dev.wasabules.dukto

import android.content.Context
import android.net.Uri
import dev.wasabules.dukto.avatar.AvatarServer
import dev.wasabules.dukto.discovery.Messenger
import dev.wasabules.dukto.discovery.Peer as DiscoveryPeer
import dev.wasabules.dukto.platform.currentSignature
import dev.wasabules.dukto.protocol.AVATAR_PORT_OFFSET
import dev.wasabules.dukto.protocol.DEFAULT_PORT
import dev.wasabules.dukto.settings.SettingsStore
import dev.wasabules.dukto.transfer.Peer as TransferPeer
import dev.wasabules.dukto.transfer.Sender
import dev.wasabules.dukto.transfer.Server
import dev.wasabules.dukto.transfer.TransferEvent
import dev.wasabules.dukto.transfer.TransferNotifier
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.merge
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.net.InetAddress

/**
 * Process-wide orchestration: settings store + UDP discovery + TCP server +
 * sender + avatar HTTP, plus the persistent UI state (signature, recent
 * transfers, in-flight progress).
 *
 * Held by [DuktoApp] so it survives configuration changes and Activity
 * recreation. The foreground [dev.wasabules.dukto.transfer.TransferService]
 * is what actually anchors the lifecycle while a transfer is in flight.
 */
class DuktoEngine(private val app: Context) {

    private val scope = CoroutineScope(Dispatchers.Default + SupervisorJob())

    val settings = SettingsStore(app)

    /** Surfaced to the UI for the bottom sheet. */
    val profile: StateFlow<Profile> = run {
        val flow = MutableStateFlow(Profile(buddyName = settings.state.value.buddyName))
        scope.launch {
            settings.state.collect { s -> flow.value = Profile(s.buddyName) }
        }
        flow.asStateFlow()
    }

    private val _activity = MutableStateFlow<List<ActivityEntry>>(emptyList())
    val activity: StateFlow<List<ActivityEntry>> = _activity.asStateFlow()

    private val _inflight = MutableStateFlow<InflightTransfer?>(null)
    val inflight: StateFlow<InflightTransfer?> = _inflight.asStateFlow()

    /** UI-friendly mirror of the SAF tree URI. */
    val destLabel: StateFlow<String> = settings.state.map { s ->
        s.destTreeUri ?: "App private storage (default)"
    }.let { flow ->
        // map returns a Flow; promote to a StateFlow via a launching coroutine.
        val out = MutableStateFlow(settings.state.value.destTreeUri ?: "App private storage (default)")
        scope.launch { flow.collect { out.value = it } }
        out.asStateFlow()
    }

    val messenger: Messenger = Messenger(
        context = app,
        signatureProvider = { currentSignature(app, settings.state.value.buddyName) },
    )
    val peers = messenger.peers

    private val server = Server(
        context = app,
        destTreeUriProvider = { settings.state.value.destTreeUri },
    )
    private val sender = Sender(context = app)

    private val avatarServer = AvatarServer(
        port = DEFAULT_PORT + AVATAR_PORT_OFFSET,
        signatureProvider = { currentSignature(app, settings.state.value.buddyName) },
    )

    fun start() {
        messenger.start()
        server.start()
        avatarServer.start()
        scope.launch {
            merge(server.events, sender.events).collect(::onTransferEvent)
        }
        messenger.sayHello()
    }

    fun stop() {
        messenger.stop()
        server.stop()
        sender.close()
        avatarServer.stop()
        scope.coroutineContext[kotlinx.coroutines.Job]?.cancel()
    }

    // ── public API ───────────────────────────────────────────────────────────

    fun setBuddyName(name: String) {
        settings.update { it.copy(buddyName = name) }
        // Rebroadcast so peers learn the new name immediately.
        messenger.sayHello()
    }

    fun setDestTreeUri(uri: String?) {
        settings.update { it.copy(destTreeUri = uri) }
    }

    fun sendText(toAddress: String, port: Int, text: String) {
        sender.sendText(TransferPeer(InetAddress.getByName(toAddress), port), text)
    }

    fun sendFiles(toAddress: String, port: Int, uris: List<Uri>) {
        sender.sendFiles(TransferPeer(InetAddress.getByName(toAddress), port), uris)
    }

    fun sendFolder(toAddress: String, port: Int, treeUri: Uri) {
        sender.sendFolder(TransferPeer(InetAddress.getByName(toAddress), port), treeUri)
    }

    fun cancelInflight() {
        // Either side could be active — signal both, only the matching one
        // has work to abort.
        server.cancelActiveReceive()
        sender.cancelActiveSend()
    }

    // ── event handling ───────────────────────────────────────────────────────

    private fun onTransferEvent(ev: TransferEvent) {
        when (ev) {
            is TransferEvent.Started -> {
                _inflight.value = InflightTransfer(
                    peer = ev.from.hostAddress.orEmpty(),
                    totalBytes = ev.totalSize,
                    bytesDone = 0L,
                    isReceive = ev.isReceive,
                )
                pushNotif(ev)
            }
            is TransferEvent.Progress -> {
                _inflight.update {
                    it?.copy(bytesDone = ev.bytesDone, totalBytes = ev.totalBytes)
                }
                pushNotifProgress(ev)
            }
            is TransferEvent.TextReceived -> {
                clearInflight()
                appendActivity(
                    ActivityEntry.TextReceived(
                        from = ev.from.hostAddress.orEmpty(),
                        text = ev.text,
                        at = System.currentTimeMillis(),
                    ),
                )
            }
            is TransferEvent.FilesReceived -> {
                clearInflight()
                appendActivity(
                    ActivityEntry.FilesReceived(
                        from = ev.from.hostAddress.orEmpty(),
                        location = ev.rootDescription,
                        fileCount = ev.fileCount,
                        at = System.currentTimeMillis(),
                    ),
                )
            }
            is TransferEvent.Failed -> {
                clearInflight()
                appendActivity(
                    ActivityEntry.Error(
                        peer = ev.from?.hostAddress.orEmpty(),
                        message = ev.reason,
                        at = System.currentTimeMillis(),
                    ),
                )
            }
            is TransferEvent.Sent -> {
                clearInflight()
                appendActivity(
                    ActivityEntry.Sent(
                        to = ev.to.address.hostAddress.orEmpty(),
                        bytes = ev.totalSize,
                        at = System.currentTimeMillis(),
                    ),
                )
            }
        }
    }

    private fun appendActivity(entry: ActivityEntry) {
        _activity.update { (listOf(entry) + it).take(64) }
    }

    private fun clearInflight() {
        _inflight.value = null
        TransferNotifier.stop(app)
    }

    private fun pushNotif(ev: TransferEvent.Started) {
        val title = if (ev.isReceive) "Receiving from ${ev.from.hostAddress}" else "Sending to ${ev.from.hostAddress}"
        val body = "${formatBytesShort(0)} / ${formatBytesShort(ev.totalSize)}"
        TransferNotifier.update(app, title, body, progress = 0, max = NOTIF_MAX)
    }

    private fun pushNotifProgress(ev: TransferEvent.Progress) {
        if (ev.totalBytes <= 0L) return
        val pct = (ev.bytesDone * NOTIF_MAX / ev.totalBytes).toInt().coerceIn(0, NOTIF_MAX)
        val title = if (ev.isReceive) "Receiving from ${ev.peer.hostAddress}" else "Sending to ${ev.peer.hostAddress}"
        val body = "${formatBytesShort(ev.bytesDone)} / ${formatBytesShort(ev.totalBytes)}"
        TransferNotifier.update(app, title, body, progress = pct, max = NOTIF_MAX)
    }

    private companion object {
        const val NOTIF_MAX = 1000
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
    data class FilesReceived(val from: String, val location: String, val fileCount: Int, override val at: Long) : ActivityEntry
    data class Sent(val to: String, val bytes: Long, override val at: Long) : ActivityEntry
    data class Error(val peer: String, val message: String, override val at: Long) : ActivityEntry
}

fun DiscoveryPeer.toTransferPeer(): TransferPeer = TransferPeer(address, port)

private fun formatBytesShort(b: Long): String {
    if (b < 0L) return "?"
    val units = listOf("B", "KB", "MB", "GB")
    var v = b.toDouble()
    var unit = 0
    while (v >= 1024.0 && unit < units.lastIndex) { v /= 1024.0; unit++ }
    return if (unit == 0) "${b}B" else "%.1f%s".format(v, units[unit])
}
