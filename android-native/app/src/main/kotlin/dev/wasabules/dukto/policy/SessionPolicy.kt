package dev.wasabules.dukto.policy

import dev.wasabules.dukto.audit.AuditLog
import dev.wasabules.dukto.settings.Settings
import dev.wasabules.dukto.settings.SettingsStore
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.sync.Semaphore
import kotlinx.coroutines.withTimeoutOrNull
import java.net.InetAddress
import java.util.UUID

/**
 * Allow/reject decisions for incoming Dukto sessions.
 *
 * Mirrors the Wails desktop's defense-in-depth chain. Order on the receive
 * path:
 *   1. master switch (receivingEnabled)
 *   2. block list (signature)
 *   3. confirm-unknown peers (60 s modal) if enabled and signature unknown
 *   4. session header checks: max session size in MB
 *   5. element checks: blocked extensions
 *
 * Whitelist mode and free-disk / file-count / depth / cooldown gates are
 * deliberately not implemented yet — they're the next-tier hardening, not
 * needed for parity with the most common Wails usage on a phone.
 */
class SessionPolicy(
    private val settings: SettingsStore,
    private val audit: AuditLog,
) {
    /** Pending-session requests awaiting a UI decision. */
    private val _pending = MutableStateFlow<List<PendingRequest>>(emptyList())
    val pending: StateFlow<List<PendingRequest>> = _pending.asStateFlow()

    /** Allow only one pending modal at a time (UX) — the second concurrent request waits. */
    private val modalLock = Semaphore(1)

    // ── stage 1-3: pre-session gates ─────────────────────────────────────────

    /**
     * Decide whether to even read the session header from this peer. Called by
     * Server immediately after accept().
     *
     * The signature is unknown at this point (it comes via the discovery
     * channel, not the TCP transfer); we use the source IP as the closest
     * proxy. The Messenger maintains a peer table keyed by IP, so we ask it.
     */
    suspend fun preSession(from: InetAddress, signatureLookup: () -> String?): Decision {
        val s = settings.state.value
        if (!s.receivingEnabled) {
            audit.append(AuditLog.Level.Reject, "MASTER_SWITCH_OFF", from.hostAddress.orEmpty())
            return Decision.Reject("Receiving is disabled in settings")
        }
        val sig = signatureLookup().orEmpty()
        if (sig.isNotEmpty() && sig in s.blockedPeers) {
            audit.append(AuditLog.Level.Reject, "BLOCKED", from.hostAddress.orEmpty(), sig)
            return Decision.Reject("Peer is on the block list")
        }
        if (s.confirmUnknownPeers && sig.isNotEmpty() && sig !in s.approvedPeers) {
            return askUser(from, sig)
        }
        // Approved by config (or we have no signature yet — fall through, the
        // session-header / element checks still run).
        if (sig.isNotEmpty()) {
            audit.append(AuditLog.Level.Info, "ACCEPT", from.hostAddress.orEmpty(), sig)
        }
        return Decision.Accept
    }

    private suspend fun askUser(from: InetAddress, sig: String): Decision {
        modalLock.acquire()
        try {
            val req = PendingRequest(
                id = UUID.randomUUID().toString(),
                peerAddress = from.hostAddress.orEmpty(),
                peerSignature = sig,
                deadline = System.currentTimeMillis() + CONFIRM_TIMEOUT_MS,
            )
            audit.append(AuditLog.Level.Info, "CONFIRM_PROMPT", req.peerAddress, sig)
            _pending.update { it + req }
            val choice: PeerChoice? = withTimeoutOrNull(CONFIRM_TIMEOUT_MS) {
                req.decision.await()
            }
            _pending.update { it.filter { p -> p.id != req.id } }
            return when (choice) {
                PeerChoice.AllowOnce -> {
                    audit.append(AuditLog.Level.Info, "USER_ALLOW_ONCE", req.peerAddress, sig)
                    Decision.Accept
                }
                PeerChoice.AllowAlways -> {
                    audit.append(AuditLog.Level.Info, "USER_APPROVE", req.peerAddress, sig)
                    settings.update { it.copy(approvedPeers = it.approvedPeers + sig) }
                    Decision.Accept
                }
                PeerChoice.Block -> {
                    audit.append(AuditLog.Level.Reject, "USER_BLOCK", req.peerAddress, sig)
                    settings.update { it.copy(blockedPeers = it.blockedPeers + sig) }
                    Decision.Reject("User blocked this peer")
                }
                PeerChoice.Reject, null -> {
                    val reason = if (choice == null) "USER_TIMEOUT" else "USER_REJECT"
                    audit.append(AuditLog.Level.Reject, reason, req.peerAddress, sig)
                    Decision.Reject("Rejected by user")
                }
            }
        } finally {
            modalLock.release()
        }
    }

    /** Resolve a pending request from the UI. No-op if the request has expired. */
    fun resolve(id: String, choice: PeerChoice) {
        _pending.value.firstOrNull { it.id == id }?.decision?.complete(choice)
    }

    // ── stage 4: session header check ────────────────────────────────────────

    fun checkSessionSize(from: InetAddress, totalBytes: Long): Decision {
        val capMB = settings.state.value.maxSessionSizeMB
        if (capMB > 0) {
            val capBytes = capMB.toLong() * 1024L * 1024L
            if (totalBytes > capBytes) {
                audit.append(
                    AuditLog.Level.Reject, "SIZE_CAP", from.hostAddress.orEmpty(),
                    "session=${totalBytes}B cap=${capBytes}B",
                )
                return Decision.Reject("Session exceeds ${capMB} MB limit")
            }
        }
        return Decision.Accept
    }

    // ── stage 5: per-element check ───────────────────────────────────────────

    fun checkElement(from: InetAddress, name: String): Decision {
        val ext = name.substringAfterLast('.', "").lowercase()
        if (ext.isEmpty()) return Decision.Accept
        if (ext in settings.state.value.blockedExtensions) {
            audit.append(
                AuditLog.Level.Reject, "EXT_BLOCKED", from.hostAddress.orEmpty(),
                "name=$name ext=$ext",
            )
            return Decision.Reject("Blocked extension .$ext")
        }
        return Decision.Accept
    }

    private companion object {
        const val CONFIRM_TIMEOUT_MS = 60_000L
    }
}

sealed interface Decision {
    data object Accept : Decision
    data class Reject(val reason: String) : Decision
}

data class PendingRequest(
    val id: String,
    val peerAddress: String,
    val peerSignature: String,
    val deadline: Long,
    internal val decision: kotlinx.coroutines.CompletableDeferred<PeerChoice> =
        kotlinx.coroutines.CompletableDeferred(),
)

enum class PeerChoice { AllowOnce, AllowAlways, Reject, Block }
