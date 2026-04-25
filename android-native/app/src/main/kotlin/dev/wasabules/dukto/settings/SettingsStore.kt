package dev.wasabules.dukto.settings

import android.content.Context
import android.content.SharedPreferences
import androidx.core.content.edit
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * Persistent settings.
 *
 * Mirrors the Wails desktop's settings surface for the bits that make sense on
 * a mobile peer. The whitelist / per-interface allow-list / interface-bound
 * cooldowns are intentionally absent — Android peers usually have a single
 * Wi-Fi interface so the value of those gates is much lower.
 */
class SettingsStore(context: Context) {

    private val prefs: SharedPreferences =
        context.applicationContext.getSharedPreferences("dukto", Context.MODE_PRIVATE)

    private val _state = MutableStateFlow(load())
    val state: StateFlow<Settings> = _state.asStateFlow()

    fun update(transform: (Settings) -> Settings) {
        val next = transform(_state.value)
        if (next == _state.value) return
        prefs.edit {
            putString(KEY_BUDDY_NAME, next.buddyName)
            if (next.destTreeUri == null) remove(KEY_DEST_TREE) else putString(KEY_DEST_TREE, next.destTreeUri)

            putBoolean(KEY_RECEIVING_ENABLED, next.receivingEnabled)
            putBoolean(KEY_CONFIRM_UNKNOWN, next.confirmUnknownPeers)
            putString(KEY_BLOCKED_PEERS, next.blockedPeers.joinToString("\n"))
            putString(KEY_APPROVED_PEERS, next.approvedPeers.joinToString("\n"))
            putString(KEY_BLOCKED_EXT, next.blockedExtensions.joinToString(","))
            putInt(KEY_MAX_SIZE_MB, next.maxSessionSizeMB)
            putInt(KEY_MAX_ACTIVITY, next.maxActivityEntries)
            putString(KEY_THEME_MODE, next.themeMode.name)
        }
        _state.value = next
    }

    /** Convenience: bulk-replace the recent activity log (persisted as JSON). */
    fun saveActivityJson(json: String) {
        prefs.edit { putString(KEY_ACTIVITY_JSON, json) }
    }

    fun loadActivityJson(): String? = prefs.getString(KEY_ACTIVITY_JSON, null)

    private fun load(): Settings = Settings(
        buddyName = prefs.getString(KEY_BUDDY_NAME, "").orEmpty(),
        destTreeUri = prefs.getString(KEY_DEST_TREE, null),
        receivingEnabled = prefs.getBoolean(KEY_RECEIVING_ENABLED, true),
        confirmUnknownPeers = prefs.getBoolean(KEY_CONFIRM_UNKNOWN, false),
        blockedPeers = prefs.getString(KEY_BLOCKED_PEERS, "")
            ?.lines()?.filter { it.isNotBlank() }.orEmpty().toSet(),
        approvedPeers = prefs.getString(KEY_APPROVED_PEERS, "")
            ?.lines()?.filter { it.isNotBlank() }.orEmpty().toSet(),
        blockedExtensions = prefs.getString(KEY_BLOCKED_EXT, DEFAULT_BLOCKED_EXT)
            ?.split(',')?.map { it.trim().lowercase() }?.filter { it.isNotEmpty() }.orEmpty().toSet(),
        maxSessionSizeMB = prefs.getInt(KEY_MAX_SIZE_MB, 0),
        maxActivityEntries = prefs.getInt(KEY_MAX_ACTIVITY, DEFAULT_MAX_ACTIVITY),
        themeMode = runCatching {
            ThemeMode.valueOf(prefs.getString(KEY_THEME_MODE, ThemeMode.System.name).orEmpty())
        }.getOrDefault(ThemeMode.System),
    )

    private companion object {
        const val KEY_BUDDY_NAME = "buddy_name"
        const val KEY_DEST_TREE = "dest_tree_uri"
        const val KEY_RECEIVING_ENABLED = "receiving_enabled"
        const val KEY_CONFIRM_UNKNOWN = "confirm_unknown_peers"
        const val KEY_BLOCKED_PEERS = "blocked_peers"
        const val KEY_APPROVED_PEERS = "approved_peers"
        const val KEY_BLOCKED_EXT = "blocked_extensions"
        const val KEY_MAX_SIZE_MB = "max_session_size_mb"
        const val KEY_MAX_ACTIVITY = "max_activity_entries"
        const val KEY_THEME_MODE = "theme_mode"
        const val KEY_ACTIVITY_JSON = "activity_json"

        // Mirrors the Wails default; conservative against Windows-only nasties.
        const val DEFAULT_BLOCKED_EXT = "exe,bat,cmd,com,scr,msi,ps1,vbs,jse,lnk"
        const val DEFAULT_MAX_ACTIVITY = 64
    }
}

/**
 * @property destTreeUri serialized [android.net.Uri] of an
 * `ACTION_OPEN_DOCUMENT_TREE` result the user has granted persistent access to.
 *   Null = use the app's private external Downloads dir as fallback.
 * @property receivingEnabled master switch — when false, Server hangs up on
 *   incoming sessions before reading any data.
 * @property confirmUnknownPeers when true, a session from a signature not in
 *   [approvedPeers] pops a 60-second modal asking the user whether to allow.
 * @property blockedPeers signatures that are hard-rejected (no modal).
 * @property approvedPeers signatures the user has approved (skips the modal).
 * @property blockedExtensions case-insensitive file extensions (without the dot)
 *   that abort the session if any element matches.
 * @property maxSessionSizeMB rejects sessions larger than this; 0 = no cap.
 */
data class Settings(
    val buddyName: String = "",
    val destTreeUri: String? = null,
    val receivingEnabled: Boolean = true,
    val confirmUnknownPeers: Boolean = false,
    val blockedPeers: Set<String> = emptySet(),
    val approvedPeers: Set<String> = emptySet(),
    val blockedExtensions: Set<String> = emptySet(),
    val maxSessionSizeMB: Int = 0,
    /**
     * Hard cap on entries kept in the recent activity list (and persisted in
     * SharedPreferences). 0 = unlimited; defaults to 64.
     */
    val maxActivityEntries: Int = 64,
    /** UI theme override; SYSTEM = follow system, LIGHT/DARK = force. */
    val themeMode: ThemeMode = ThemeMode.System,
)

enum class ThemeMode { System, Light, Dark }
