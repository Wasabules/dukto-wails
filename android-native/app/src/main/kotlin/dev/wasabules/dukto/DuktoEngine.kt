package dev.wasabules.dukto

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.net.Uri
import dev.wasabules.dukto.audit.AuditLog
import dev.wasabules.dukto.avatar.AvatarServer
import dev.wasabules.dukto.avatar.defaultAvatarPng
import dev.wasabules.dukto.identity.Identity
import dev.wasabules.dukto.identity.loadOrGenerate as loadOrGenerateIdentity
import java.io.ByteArrayOutputStream
import java.io.File
import dev.wasabules.dukto.discovery.Messenger
import dev.wasabules.dukto.discovery.Peer as DiscoveryPeer
import dev.wasabules.dukto.platform.currentSignature
import dev.wasabules.dukto.policy.PeerChoice
import dev.wasabules.dukto.policy.PendingRequest
import dev.wasabules.dukto.policy.SessionPolicy
import dev.wasabules.dukto.protocol.AVATAR_PORT_OFFSET
import dev.wasabules.dukto.protocol.DEFAULT_PORT
import dev.wasabules.dukto.settings.Settings
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
import kotlinx.coroutines.flow.merge
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import org.json.JSONArray
import org.json.JSONObject
import java.net.InetAddress

class DuktoEngine(private val app: Context) {

    private val scope = CoroutineScope(Dispatchers.Default + SupervisorJob())

    val settings = SettingsStore(app)
    val audit = AuditLog(app)
    val policy = SessionPolicy(settings, audit)

    val profile: StateFlow<Profile> = run {
        val flow = MutableStateFlow(Profile(buddyName = settings.state.value.buddyName))
        scope.launch { settings.state.collect { s -> flow.value = Profile(s.buddyName) } }
        flow.asStateFlow()
    }
    val settingsFlow: StateFlow<Settings> = settings.state

    private val _activity = MutableStateFlow(loadActivity())
    val activity: StateFlow<List<ActivityEntry>> = _activity.asStateFlow()

    private val _inflight = MutableStateFlow<InflightTransfer?>(null)
    val inflight: StateFlow<InflightTransfer?> = _inflight.asStateFlow()

    val destLabel: StateFlow<String> = run {
        val out = MutableStateFlow(settings.state.value.destTreeUri ?: "App private storage (default)")
        scope.launch {
            settings.state.collect { s ->
                out.value = s.destTreeUri ?: "App private storage (default)"
            }
        }
        out.asStateFlow()
    }

    val pendingPeerRequests: StateFlow<List<PendingRequest>> = policy.pending

    // ── identity (M1) ────────────────────────────────────────────────────

    /** Long-term Ed25519 keypair for the v2 encrypted overlay. May be null
     *  if loading or generating failed at startup; callers should treat the
     *  fingerprint as empty in that case rather than crash. */
    val identity: Identity? = runCatching {
        loadOrGenerateIdentity(app, File(app.filesDir, "identity.key"))
    }.getOrNull()

    /** User-visible 16-char fingerprint, or empty if [identity] is null. */
    val identityFingerprint: String get() = identity?.fingerprint.orEmpty()

    // ── avatar (local + custom) ──────────────────────────────────────────────

    private val avatarFile: File = File(app.filesDir, "avatar.png")

    /** Latest avatar PNG bytes (custom override if set, otherwise generated
     *  from the signature initials). Updated whenever the buddy name changes
     *  or the user picks/clears a custom image. */
    private val _avatarBytes = MutableStateFlow(loadCurrentAvatarBytes())
    val avatarBytes: StateFlow<ByteArray> = _avatarBytes.asStateFlow()

    /** True iff [avatarFile] exists — drives "Reset to initials" visibility. */
    private val _hasCustomAvatar = MutableStateFlow(avatarFile.exists())
    val hasCustomAvatar: StateFlow<Boolean> = _hasCustomAvatar.asStateFlow()

    private fun loadCurrentAvatarBytes(): ByteArray {
        if (avatarFile.exists()) {
            runCatching { return avatarFile.readBytes() }
        }
        return defaultAvatarPng(currentSignature(app, settings.state.value.buddyName))
    }

    private fun refreshAvatarBytes() {
        _avatarBytes.value = loadCurrentAvatarBytes()
        _hasCustomAvatar.value = avatarFile.exists()
    }

    val messenger: Messenger = Messenger(
        context = app,
        signatureProvider = { currentSignature(app, settings.state.value.buddyName) },
        identity = identity,
    )
    val peers = messenger.peers

    private val server = Server(
        context = app,
        destTreeUriProvider = { settings.state.value.destTreeUri },
        policy = policy,
        signatureLookup = { addr ->
            // Server hands us the InetAddress that just connected; cross-reference
            // with the discovery peer table (keyed by IP) to recover the buddy
            // signature the policy needs for block/approve decisions.
            messenger.peers.value.values
                .firstOrNull { it.address == addr }?.signature
        },
        v2Identity = identity,
        onSessionMode = { encrypted ->
            // Latch the per-session encrypted flag. The transfer.Server
            // calls this exactly once per session, so the Activity / audit
            // event handler can stamp the entry correctly.
            lastSessionEncrypted = encrypted
        },
    )
    private val sender = Sender(
        context = app,
        v2Identity = identity,
        pinnedX25519For = { addr ->
            // Look up the pinned X25519 pubkey for `addr`. We pin by Ed25519
            // fingerprint, so first translate IP→Ed25519 pubkey via the
            // discovery messenger's verified-peer table, then check the
            // pinned map keyed by fingerprint, then derive the X25519.
            val edPub = messenger.peers.value.values
                .firstOrNull { it.address == addr }
                ?.pubKey ?: return@Sender null
            val fp = dev.wasabules.dukto.identity.fingerprintOf(edPub)
            val pinned = settings.state.value.pinnedPeers[fp] ?: return@Sender null
            // We could either re-derive X25519 from the stored hex'd Ed25519
            // pubkey or stash the X25519 too. Re-derive for simplicity —
            // the Edwards-to-Montgomery transform is < 1 ms.
            dev.wasabules.dukto.identity.ed25519PubToX25519Pub(
                hexDecode(pinned.ed25519PubHex)
            )
        },
    )

    /** Latched in [Server.onSessionMode]; read by the receive-event handler
     *  so the Activity entry can record kind=ENCRYPTED vs CLEARTEXT. */
    @Volatile private var lastSessionEncrypted: Boolean = false

    private val avatarServer = AvatarServer(
        port = DEFAULT_PORT + AVATAR_PORT_OFFSET,
        signatureProvider = { currentSignature(app, settings.state.value.buddyName) },
        // Serve the live bytes — peers always see the current avatar
        // (custom upload or generated initials) without restart.
        bitmapProvider = { _avatarBytes.value },
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
        // The DefaultRenderer is keyed by signature — re-derive only if no
        // custom image is set, so a name change isn't silently overwritten by
        // the user's previous upload.
        if (!_hasCustomAvatar.value) refreshAvatarBytes()
        messenger.sayHello()
    }

    /**
     * Persist [uri]'s contents as the custom avatar. Decodes through
     * [BitmapFactory] to validate it's a real image, then re-encodes as a
     * 64×64 PNG so peers always get the canonical size regardless of input
     * resolution.
     */
    fun setCustomAvatar(uri: Uri) {
        val raw = app.contentResolver.openInputStream(uri)?.use { it.readBytes() }
            ?: throw IllegalStateException("cannot open $uri")
        val source = BitmapFactory.decodeByteArray(raw, 0, raw.size)
            ?: throw IllegalStateException("not a recognised image")
        val sized = if (source.width == AVATAR_PX && source.height == AVATAR_PX) source
        else Bitmap.createScaledBitmap(source, AVATAR_PX, AVATAR_PX, /* filter = */ true)
        val baos = ByteArrayOutputStream(8 * 1024)
        if (!sized.compress(Bitmap.CompressFormat.PNG, 100, baos)) {
            throw IllegalStateException("PNG encode failed")
        }
        val bytes = baos.toByteArray()
        avatarFile.writeBytes(bytes)
        _avatarBytes.value = bytes
        _hasCustomAvatar.value = true
    }

    fun clearCustomAvatar() {
        runCatching { avatarFile.delete() }
        refreshAvatarBytes()
    }

    fun setDestTreeUri(uri: String?) = settings.update { it.copy(destTreeUri = uri) }

    fun setReceivingEnabled(value: Boolean) = settings.update { it.copy(receivingEnabled = value) }
    fun setConfirmUnknownPeers(value: Boolean) = settings.update { it.copy(confirmUnknownPeers = value) }
    fun setBlockedExtensions(set: Set<String>) =
        settings.update { it.copy(blockedExtensions = set.map { e -> e.lowercase().trim() }.filter { e -> e.isNotEmpty() }.toSet()) }
    fun setMaxSessionSizeMB(mb: Int) = settings.update { it.copy(maxSessionSizeMB = mb.coerceAtLeast(0)) }
    fun setThemeMode(mode: dev.wasabules.dukto.settings.ThemeMode) =
        settings.update { it.copy(themeMode = mode) }

    fun setBiometricLockEnabled(enabled: Boolean) =
        settings.update { it.copy(biometricLockEnabled = enabled) }

    /**
     * Pin the v2 peer at [addr] using its currently advertised Ed25519
     * pubkey. Refuses to pin a peer that hasn't sent a verified 0x06/0x07
     * HELLO yet. After pinning, outbound transfers to that peer run over
     * Noise XX automatically.
     */
    fun pinPeer(addr: java.net.InetAddress): String? {
        val peer = messenger.peers.value.values.firstOrNull { it.address == addr } ?: return null
        val pub = peer.pubKey ?: return null
        val fp = dev.wasabules.dukto.identity.fingerprintOf(pub)
        settings.update {
            it.copy(
                pinnedPeers = it.pinnedPeers + (fp to dev.wasabules.dukto.settings.PinnedPeer(
                    fingerprint = fp,
                    ed25519PubHex = hexEncode(pub),
                    label = peer.signature,
                    pinnedAt = System.currentTimeMillis(),
                )),
            )
        }
        return fp
    }

    /** Remove the pinning for a fingerprint. Cleartext fallback resumes. */
    fun unpinPeer(fingerprint: String) {
        settings.update { it.copy(pinnedPeers = it.pinnedPeers - fingerprint) }
    }

    /** True when the peer at [addr] has been pinned by fingerprint. */
    fun isPeerPinned(addr: java.net.InetAddress): Boolean {
        val pub = messenger.peers.value.values.firstOrNull { it.address == addr }?.pubKey ?: return false
        val fp = dev.wasabules.dukto.identity.fingerprintOf(pub)
        return settings.state.value.pinnedPeers.containsKey(fp)
    }

    fun setMaxActivityEntries(n: Int) {
        val capped = n.coerceAtLeast(0)
        settings.update { it.copy(maxActivityEntries = capped) }
        // Trim immediately so a user lowering the limit sees the effect.
        if (capped > 0 && _activity.value.size > capped) {
            _activity.update { it.take(capped) }
            persistActivity()
        }
    }

    fun blockPeer(signature: String) = settings.update {
        it.copy(blockedPeers = it.blockedPeers + signature, approvedPeers = it.approvedPeers - signature)
    }
    fun unblockPeer(signature: String) = settings.update { it.copy(blockedPeers = it.blockedPeers - signature) }
    fun forgetApprovals() = settings.update { it.copy(approvedPeers = emptySet()) }

    fun resolvePeerRequest(id: String, choice: PeerChoice) = policy.resolve(id, choice)

    fun clearActivity() {
        _activity.value = emptyList()
        persistActivity()
    }

    fun clearAuditLog() = audit.clear()

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
                _inflight.update { it?.copy(bytesDone = ev.bytesDone, totalBytes = ev.totalBytes) }
                pushNotifProgress(ev)
            }
            is TransferEvent.TextReceived -> {
                clearInflight()
                appendActivity(
                    ActivityEntry.TextReceived(
                        from = ev.from.hostAddress.orEmpty(),
                        text = ev.text,
                        at = System.currentTimeMillis(),
                        encrypted = lastSessionEncrypted,
                    ),
                )
                lastSessionEncrypted = false
            }
            is TransferEvent.FilesReceived -> {
                clearInflight()
                appendActivity(
                    ActivityEntry.FilesReceived(
                        from = ev.from.hostAddress.orEmpty(),
                        location = ev.rootDescription,
                        fileCount = ev.fileCount,
                        fileUris = ev.fileUris,
                        at = System.currentTimeMillis(),
                        encrypted = lastSessionEncrypted,
                    ),
                )
                lastSessionEncrypted = false
            }
            is TransferEvent.Failed -> {
                clearInflight()
                appendActivity(
                    ActivityEntry.Error(
                        peer = ev.from?.hostAddress.orEmpty(),
                        message = ev.reason,
                        at = System.currentTimeMillis(),
                        encrypted = lastSessionEncrypted,
                    ),
                )
                lastSessionEncrypted = false
            }
            is TransferEvent.Sent -> {
                clearInflight()
                appendActivity(
                    ActivityEntry.Sent(
                        to = ev.to.address.hostAddress.orEmpty(),
                        bytes = ev.totalSize,
                        at = System.currentTimeMillis(),
                        // Send-side: derived from whether we successfully
                        // ran v2 to this peer. Approximate by checking
                        // pinning at completion time; since the Sender
                        // verified the remote_static, pinned == encrypted.
                        encrypted = isPeerPinned(ev.to.address),
                    ),
                )
            }
        }
    }

    private fun appendActivity(entry: ActivityEntry) {
        val cap = settings.state.value.maxActivityEntries
        _activity.update {
            val merged = listOf(entry) + it
            if (cap > 0) merged.take(cap) else merged
        }
        persistActivity()
    }

    private fun persistActivity() {
        val arr = JSONArray()
        for (e in _activity.value) arr.put(e.toJson())
        settings.saveActivityJson(arr.toString())
    }

    private fun loadActivity(): List<ActivityEntry> {
        val raw = settings.loadActivityJson() ?: return emptyList()
        val arr = runCatching { JSONArray(raw) }.getOrNull() ?: return emptyList()
        val out = mutableListOf<ActivityEntry>()
        for (i in 0 until arr.length()) {
            val obj = arr.optJSONObject(i) ?: continue
            ActivityEntry.fromJson(obj)?.let { out += it }
        }
        return out
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
        const val AVATAR_PX = 64
    }
}

private fun hexEncode(b: ByteArray): String =
    b.joinToString("") { "%02x".format(it) }

private fun hexDecode(s: String): ByteArray {
    require(s.length % 2 == 0) { "hex: odd length ${s.length}" }
    val out = ByteArray(s.length / 2)
    for (i in out.indices) {
        out[i] = s.substring(i * 2, i * 2 + 2).toInt(16).toByte()
    }
    return out
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
    /** True when the session ran over Noise XX (encrypted-on-the-wire). */
    val encrypted: Boolean

    data class TextReceived(
        val from: String,
        val text: String,
        override val at: Long,
        override val encrypted: Boolean = false,
    ) : ActivityEntry
    data class FilesReceived(
        val from: String,
        val location: String,
        val fileCount: Int,
        val fileUris: List<String>,
        override val at: Long,
        override val encrypted: Boolean = false,
    ) : ActivityEntry
    data class Sent(
        val to: String,
        val bytes: Long,
        override val at: Long,
        override val encrypted: Boolean = false,
    ) : ActivityEntry
    data class Error(
        val peer: String,
        val message: String,
        override val at: Long,
        override val encrypted: Boolean = false,
    ) : ActivityEntry

    fun toJson(): JSONObject = when (this) {
        is TextReceived -> JSONObject().put("kind", "text").put("from", from).put("text", text)
            .put("at", at).put("encrypted", encrypted)
        is FilesReceived -> JSONObject().put("kind", "files").put("from", from).put("location", location)
            .put("fileCount", fileCount).put("at", at).put("encrypted", encrypted)
            .put("fileUris", JSONArray().apply { fileUris.forEach { put(it) } })
        is Sent -> JSONObject().put("kind", "sent").put("to", to).put("bytes", bytes)
            .put("at", at).put("encrypted", encrypted)
        is Error -> JSONObject().put("kind", "error").put("peer", peer).put("message", message)
            .put("at", at).put("encrypted", encrypted)
    }

    companion object {
        fun fromJson(o: JSONObject): ActivityEntry? = when (o.optString("kind")) {
            "text" -> TextReceived(o.optString("from"), o.optString("text"), o.optLong("at"), o.optBoolean("encrypted"))
            "files" -> {
                val urisJson = o.optJSONArray("fileUris") ?: JSONArray()
                val uris = (0 until urisJson.length()).map { urisJson.getString(it) }
                FilesReceived(
                    from = o.optString("from"),
                    location = o.optString("location"),
                    fileCount = o.optInt("fileCount"),
                    fileUris = uris,
                    at = o.optLong("at"),
                    encrypted = o.optBoolean("encrypted"),
                )
            }
            "sent" -> Sent(o.optString("to"), o.optLong("bytes"), o.optLong("at"), o.optBoolean("encrypted"))
            "error" -> Error(o.optString("peer"), o.optString("message"), o.optLong("at"), o.optBoolean("encrypted"))
            else -> null
        }
    }
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
