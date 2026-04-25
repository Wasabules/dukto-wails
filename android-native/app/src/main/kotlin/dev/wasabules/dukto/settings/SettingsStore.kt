package dev.wasabules.dukto.settings

import android.content.Context
import android.content.SharedPreferences
import androidx.core.content.edit
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * Tiny persistent settings layer.
 *
 * SharedPreferences is overkill-resistant enough for the handful of values we
 * need (display name, persisted SAF dest tree URI). DataStore Preferences would
 * be the current best practice but adds a transitive Coroutines/IO dep with
 * no real win at this scope.
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
        }
        _state.value = next
    }

    private fun load(): Settings = Settings(
        buddyName = prefs.getString(KEY_BUDDY_NAME, "").orEmpty(),
        destTreeUri = prefs.getString(KEY_DEST_TREE, null),
    )

    private companion object {
        const val KEY_BUDDY_NAME = "buddy_name"
        const val KEY_DEST_TREE = "dest_tree_uri"
    }
}

/**
 * @property destTreeUri serialized [android.net.Uri] of an
 * `ACTION_OPEN_DOCUMENT_TREE` result that the user has granted persistent
 * access to. Null means "use the app's private external Downloads dir as a
 * fallback".
 */
data class Settings(
    val buddyName: String = "",
    val destTreeUri: String? = null,
)
