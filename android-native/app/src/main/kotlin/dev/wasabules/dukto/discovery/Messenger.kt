package dev.wasabules.dukto.discovery

import android.content.Context
import android.net.wifi.WifiManager
import android.util.Log
import dev.wasabules.dukto.protocol.BuddyMessage
import dev.wasabules.dukto.protocol.DEFAULT_PORT
import dev.wasabules.dukto.protocol.MessageType
import dev.wasabules.dukto.protocol.helloBroadcastType
import dev.wasabules.dukto.protocol.helloUnicastType
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.net.DatagramPacket
import java.net.DatagramSocket
import java.net.Inet4Address
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.NetworkInterface

/** A discovered Dukto peer on the LAN. */
data class Peer(
    val address: InetAddress,
    val port: Int,
    val signature: String,
) {
    /** Stable identity key — IP + port pair (signature can change as the user renames themselves). */
    val key: String get() = "${address.hostAddress}:$port"
}

/** Event emitted by [Messenger] on the [events] flow. */
sealed interface PeerEvent {
    data class Found(val peer: Peer) : PeerEvent
    data class Gone(val peer: Peer) : PeerEvent
}

/**
 * UDP peer discovery: HELLO broadcast every [helloIntervalMs], unicast replies
 * on incoming HELLOs, GOODBYE on stop. Mirrors wails/internal/discovery on the
 * Go side and the Qt Messenger class.
 *
 * On Android, a [WifiManager.MulticastLock] is required to receive UDP broadcast
 * datagrams on most devices — acquire/release happens around start/stop.
 *
 * Thread-safety: methods are only safe to call from the Activity / ViewModel
 * (single owner). Internal coroutines do their own dispatching.
 */
class Messenger(
    private val context: Context,
    private val signatureProvider: () -> String,
    private val port: Int = DEFAULT_PORT,
    private val helloIntervalMs: Long = 10_000L,
) {
    private val scope = CoroutineScope(Dispatchers.IO)

    private val _peers = MutableStateFlow<Map<String, Peer>>(emptyMap())
    val peers: StateFlow<Map<String, Peer>> = _peers.asStateFlow()

    private val _events = MutableSharedFlow<PeerEvent>(extraBufferCapacity = 16)
    val events: SharedFlow<PeerEvent> = _events.asSharedFlow()

    private var socket: DatagramSocket? = null
    private var multicastLock: WifiManager.MulticastLock? = null
    private var rxJob: Job? = null
    private var helloJob: Job? = null

    /** Local IPv4 addresses, refreshed on start and on each broadcast pass. */
    @Volatile private var localAddrs: Set<InetAddress> = emptySet()

    fun start() {
        if (socket != null) return
        acquireMulticastLock()
        val s = DatagramSocket(null).apply {
            reuseAddress = true
            broadcast = true
            bind(InetSocketAddress(port))
        }
        socket = s
        refreshLocalAddrs()

        rxJob = scope.launch { receiveLoop(s) }
        helloJob = scope.launch { helloLoop(s) }
    }

    fun stop() {
        val s = socket ?: return
        runCatching { sendGoodbye(s) }
        rxJob?.cancel()
        helloJob?.cancel()
        s.close()
        socket = null
        releaseMulticastLock()
        scope.cancel()
    }

    /** Send a HELLO immediately (used right after start to populate the peer list). */
    fun sayHello() {
        scope.launch { socket?.let { broadcastHello(it) } }
    }

    // ── network plumbing ─────────────────────────────────────────────────────

    private suspend fun receiveLoop(s: DatagramSocket) = withContext(Dispatchers.IO) {
        val buf = ByteArray(64 * 1024)
        while (true) {
            val pkt = DatagramPacket(buf, buf.size)
            try {
                s.receive(pkt)
            } catch (e: Exception) {
                if (s.isClosed) return@withContext
                Log.w(TAG, "receive error: ${e.message}")
                continue
            }
            handleDatagram(s, pkt)
        }
    }

    private suspend fun helloLoop(s: DatagramSocket) {
        // First pulse on start, then periodic.
        broadcastHello(s)
        while (true) {
            delay(helloIntervalMs)
            refreshLocalAddrs()
            broadcastHello(s)
        }
    }

    private fun handleDatagram(s: DatagramSocket, pkt: DatagramPacket) {
        val src = pkt.address ?: return
        if (src in localAddrs) return // self-echo
        val data = pkt.data.copyOfRange(0, pkt.length)
        val msg = try {
            BuddyMessage.parse(data)
        } catch (e: Exception) {
            return // ignore malformed
        }
        when (msg.type) {
            MessageType.HelloBroadcast,
            MessageType.HelloUnicast,
            MessageType.HelloPortBroadcast,
            MessageType.HelloPortUnicast -> {
                val peerPort = if (msg.type.hasPort) msg.port else DEFAULT_PORT
                val peer = Peer(src, peerPort, msg.signature)
                upsertPeer(peer)
                // Reply with unicast HELLO if this was a broadcast.
                if (msg.type == MessageType.HelloBroadcast || msg.type == MessageType.HelloPortBroadcast) {
                    runCatching { sendHelloUnicast(s, src, peerPort) }
                }
            }
            MessageType.Goodbye -> {
                removePeer(src)
            }
        }
    }

    private fun broadcastHello(s: DatagramSocket) {
        val sig = signatureProvider()
        val msg = BuddyMessage(helloBroadcastType(port), port = port, signature = sig)
        val bytes = msg.serialize()
        for (bcast in broadcastAddresses()) {
            try {
                s.send(DatagramPacket(bytes, bytes.size, bcast, DEFAULT_PORT))
            } catch (e: Exception) {
                Log.v(TAG, "broadcast to $bcast failed: ${e.message}")
            }
        }
    }

    private fun sendHelloUnicast(s: DatagramSocket, dst: InetAddress, dstPort: Int) {
        val sig = signatureProvider()
        val msg = BuddyMessage(helloUnicastType(port), port = port, signature = sig)
        val bytes = msg.serialize()
        s.send(DatagramPacket(bytes, bytes.size, dst, dstPort))
    }

    private fun sendGoodbye(s: DatagramSocket) {
        val bytes = BuddyMessage.goodbye().serialize()
        for (bcast in broadcastAddresses()) {
            runCatching { s.send(DatagramPacket(bytes, bytes.size, bcast, DEFAULT_PORT)) }
        }
    }

    // ── peer table ───────────────────────────────────────────────────────────

    private fun upsertPeer(peer: Peer) {
        var changed = false
        _peers.update { current ->
            val existing = current[peer.key]
            if (existing == peer) {
                current
            } else {
                changed = true
                current + (peer.key to peer)
            }
        }
        if (changed) _events.tryEmit(PeerEvent.Found(peer))
    }

    private fun removePeer(addr: InetAddress) {
        val toRemove = _peers.value.values.filter { it.address == addr }
        if (toRemove.isEmpty()) return
        _peers.update { current -> current - toRemove.map { it.key } }
        toRemove.forEach { _events.tryEmit(PeerEvent.Gone(it)) }
    }

    // ── interfaces ───────────────────────────────────────────────────────────

    private fun refreshLocalAddrs() {
        val addrs = mutableSetOf<InetAddress>()
        for (iface in NetworkInterface.getNetworkInterfaces()) {
            if (!iface.isUp || iface.isLoopback) continue
            for (addr in iface.inetAddresses) {
                if (addr is Inet4Address) addrs += addr
            }
        }
        localAddrs = addrs
    }

    private fun broadcastAddresses(): List<InetAddress> {
        val out = mutableListOf<InetAddress>()
        for (iface in NetworkInterface.getNetworkInterfaces()) {
            if (!iface.isUp || iface.isLoopback) continue
            for (addrInfo in iface.interfaceAddresses) {
                val bcast = addrInfo.broadcast ?: continue
                if (bcast is Inet4Address) out += bcast
            }
        }
        // Fallback to global 255.255.255.255 if no per-iface broadcast was found.
        if (out.isEmpty()) runCatching { out += InetAddress.getByName("255.255.255.255") }
        return out
    }

    // ── multicast lock ───────────────────────────────────────────────────────

    private fun acquireMulticastLock() {
        val wifi = context.applicationContext.getSystemService(Context.WIFI_SERVICE) as WifiManager
        val lock = wifi.createMulticastLock("dukto-discovery").apply {
            setReferenceCounted(false)
            acquire()
        }
        multicastLock = lock
    }

    private fun releaseMulticastLock() {
        runCatching { multicastLock?.release() }
        multicastLock = null
    }

    private companion object { const val TAG = "DuktoMessenger" }
}
