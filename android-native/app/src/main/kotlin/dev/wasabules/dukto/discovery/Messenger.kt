package dev.wasabules.dukto.discovery

import android.content.Context
import android.net.wifi.WifiManager
import android.util.Log
import dev.wasabules.dukto.identity.Identity
import dev.wasabules.dukto.identity.fingerprintOf
import dev.wasabules.dukto.identity.verifyEd25519
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
    /** True once we've received and verified at least one v2 HELLO from this peer. */
    val v2Capable: Boolean = false,
    /** Long-term Ed25519 pubkey (32 B) advertised by the peer; null until verified. */
    val pubKey: ByteArray? = null,
) {
    /** Stable identity key — IP + port pair (signature can change as the user renames themselves). */
    val key: String get() = "${address.hostAddress}:$port"

    /** 16-char base32 fingerprint, or empty for v1-only peers. */
    val fingerprint: String get() = pubKey?.let { fingerprintOf(it) }.orEmpty()

    // Manual equals/hashCode because ByteArray comparisons are by-identity by default.
    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (other !is Peer) return false
        if (address != other.address || port != other.port || signature != other.signature) return false
        if (v2Capable != other.v2Capable) return false
        val a = pubKey; val b = other.pubKey
        if ((a == null) != (b == null)) return false
        if (a != null && b != null && !a.contentEquals(b)) return false
        return true
    }

    override fun hashCode(): Int {
        var h = address.hashCode()
        h = 31 * h + port
        h = 31 * h + signature.hashCode()
        h = 31 * h + v2Capable.hashCode()
        h = 31 * h + (pubKey?.contentHashCode() ?: 0)
        return h
    }
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
    /** When non-null, every HELLO interval also broadcasts a v2 0x06 datagram and
     *  every reply sends a 0x07 unicast alongside the legacy ones. Inbound
     *  0x06/0x07 datagrams are accepted only after Ed25519 verification. */
    private val identity: Identity? = null,
    /** Returns true to suppress every outbound HELLO (broadcast + unicast
     *  reply + GOODBYE). Re-read on each emission so toggling at runtime
     *  takes effect immediately. */
    private val hideFromDiscovery: () -> Boolean = { false },
    /** Called when a source IP that previously produced a verified
     *  0x06/0x07 with [oldPub] now produces one with [newPub]. The
     *  discovery layer doesn't know what's pinned — the engine
     *  decides whether to surface a UI alert. */
    private val onIdentityRotation: (
        addr: InetAddress,
        oldPub: ByteArray,
        newPub: ByteArray,
    ) -> Unit = { _, _, _ -> },
    /** Returns true when [pub] (raw 32-byte Ed25519) is in our TOFU
     *  table. Used to bypass HideFromDiscovery on the auto-reply
     *  path: a paired stealth peer answers its friends' probes while
     *  staying silent to strangers. */
    private val isPubKeyPinned: (ByteArray) -> Boolean = { false },
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

    /** Pubkey advertised by each peer that has produced a verified 0x06/0x07.
     *  Keyed by IP so the legacy 0x04/0x05 branch can stamp v2 capability too. */
    private val v2Keys: MutableMap<InetAddress, ByteArray> = mutableMapOf()

    fun start() {
        if (socket != null) return
        acquireMulticastLock()
        // NB: use `also` rather than `apply`. Inside apply { ... } `this` rebinds
        // to the DatagramSocket, so a bare `port` would resolve to
        // DatagramSocket.port (== -1 while unbound), not Messenger.port.
        val s = DatagramSocket(null).also {
            it.reuseAddress = true
            it.broadcast = true
            it.bind(InetSocketAddress(port))
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

    /**
     * Send a unicast HELLO to a single peer. Used to poke manual peers
     * (cross-subnet / out-of-broadcast) so they learn we exist even when
     * UDP broadcast can't reach. **Bypasses HideFromDiscovery** —
     * the caller (engine, manual peer poke, paired peer poke) has
     * already decided that this destination is appropriate; in stealth
     * mode we still want to reach our pinned friends and our manual
     * peers, just not strangers.
     */
    fun unicastHello(dst: InetAddress, dstPort: Int = DEFAULT_PORT) {
        scope.launch {
            socket?.let { s -> runCatching { writeHelloUnicast(s, dst, dstPort) } }
        }
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
                val knownKey = synchronized(v2Keys) { v2Keys[src]?.copyOf() }
                val peer = Peer(
                    src, peerPort, msg.signature,
                    v2Capable = knownKey != null,
                    pubKey = knownKey,
                )
                upsertPeer(peer)
                if (msg.type == MessageType.HelloBroadcast || msg.type == MessageType.HelloPortBroadcast) {
                    // Legacy 0x01/0x04: no key in this datagram, but we
                    // may have a v2 key stamped from a recent 0x06 by
                    // the same IP — pass it for the paired-bypass check.
                    runCatching { sendHelloUnicastReply(s, src, peerPort, knownKey) }
                }
            }
            MessageType.Goodbye -> {
                synchronized(v2Keys) { v2Keys.remove(src) }
                removePeer(src)
            }
            MessageType.HelloPortKeyBroadcast,
            MessageType.HelloPortKeyUnicast -> {
                val pub = msg.pubKey ?: return
                val sig = msg.sig ?: return
                if (!verifyEd25519(pub, msg.signedPayload(), sig)) return
                // Identity-rotation detection: when the same IP swaps
                // its advertised pubkey, fire the rotation hook so the
                // engine can decide whether to alert (only matters when
                // the old key was in the TOFU table).
                val rotated = synchronized(v2Keys) {
                    val previous = v2Keys[src]
                    v2Keys[src] = pub.copyOf()
                    previous?.takeIf { !it.contentEquals(pub) }
                }
                if (rotated != null) onIdentityRotation(src, rotated, pub.copyOf())
                val peer = Peer(
                    src, msg.port, msg.signature,
                    v2Capable = true,
                    pubKey = pub.copyOf(),
                )
                upsertPeer(peer)
                if (msg.type == MessageType.HelloPortKeyBroadcast) {
                    // Verified pubkey in hand: paired-bypass uses it
                    // directly, so a paired stealth peer answers and
                    // an unpaired stealth peer stays silent.
                    runCatching { sendHelloUnicastReply(s, src, msg.port, pub) }
                }
            }
        }
    }

    private fun broadcastHello(s: DatagramSocket) {
        if (hideFromDiscovery()) return
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
        // v2 layer: best-effort 0x06 broadcast alongside the legacy datagram.
        // Failures are deliberately ignored here so the legacy path keeps
        // working when the v2 send fails (interface-level issues).
        identity?.let { id ->
            val v2Bytes = signedHelloBytes(MessageType.HelloPortKeyBroadcast, sig, id)
            for (bcast in broadcastAddresses()) {
                runCatching { s.send(DatagramPacket(v2Bytes, v2Bytes.size, bcast, DEFAULT_PORT)) }
            }
        }
    }

    /**
     * Auto-reply to an inbound HELLO. Suppressed by stealth UNLESS
     * [peerPub] identifies a paired peer — that bypass keeps
     * "answering my friends" working even when broadcast is off.
     * Pass [peerPub] = null when the trigger is a legacy 0x01..0x05
     * with no key material; the bypass then never kicks in.
     */
    private fun sendHelloUnicastReply(
        s: DatagramSocket,
        dst: InetAddress,
        dstPort: Int,
        peerPub: ByteArray?,
    ) {
        if (hideFromDiscovery()) {
            if (peerPub == null || !isPubKeyPinned(peerPub)) return
        }
        writeHelloUnicast(s, dst, dstPort)
    }

    /** Raw unicast HELLO write. No stealth gate — callers above this
     *  point own the gating policy. */
    private fun writeHelloUnicast(s: DatagramSocket, dst: InetAddress, dstPort: Int) {
        val sig = signatureProvider()
        val msg = BuddyMessage(helloUnicastType(port), port = port, signature = sig)
        val bytes = msg.serialize()
        s.send(DatagramPacket(bytes, bytes.size, dst, dstPort))
        identity?.let { id ->
            val v2Bytes = signedHelloBytes(MessageType.HelloPortKeyUnicast, sig, id)
            runCatching { s.send(DatagramPacket(v2Bytes, v2Bytes.size, dst, dstPort)) }
        }
    }

    private fun signedHelloBytes(type: MessageType, sig: String, id: Identity): ByteArray {
        val unsigned = BuddyMessage(type, port = port, signature = sig)
        val signature = id.sign(unsigned.signedPayload())
        return unsigned.copy(pubKey = id.publicKey.copyOf(), sig = signature).serialize()
    }

    private fun sendGoodbye(s: DatagramSocket) {
        if (hideFromDiscovery()) return
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
