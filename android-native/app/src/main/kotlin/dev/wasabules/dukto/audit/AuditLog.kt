package dev.wasabules.dukto.audit

import android.content.Context
import android.util.Log
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import org.json.JSONObject
import java.io.BufferedReader
import java.io.File
import java.io.FileReader
import java.io.IOException
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * Append-only JSON-per-line security audit log.
 *
 * Mirrors the Wails desktop's audit feature: every accept / reject / policy
 * decision lands here so users can review what happened. Rotates at 1 MiB
 * (smaller than the desktop because we don't want to hog phone storage), keeps
 * one prior generation as `audit.log.1`.
 *
 * Reads happen on the calling thread (the file is small) and back the Audit
 * tab in Settings. Writes are synchronous and best-effort — a failed write
 * surfaces as a Log.w but never bubbles to the caller.
 */
class AuditLog(context: Context) {

    private val file: File = File(context.filesDir, "audit.log")
    private val rotated: File = File(context.filesDir, "audit.log.1")

    private val _entries = MutableStateFlow<List<Entry>>(loadAll())
    val entries: StateFlow<List<Entry>> = _entries.asStateFlow()

    @Synchronized
    fun append(level: Level, kind: String, peer: String, detail: String = "") {
        val e = Entry(System.currentTimeMillis(), level, kind, peer, detail)
        try {
            if (file.exists() && file.length() > MAX_BYTES) {
                if (rotated.exists()) rotated.delete()
                file.renameTo(rotated)
            }
            file.appendText(e.toJsonLine() + "\n")
        } catch (ex: IOException) {
            Log.w(TAG, "audit write failed: ${ex.message}")
            return
        }
        _entries.value = (_entries.value + e).takeLast(MAX_IN_MEMORY)
    }

    fun clear() {
        runCatching { file.delete(); rotated.delete() }
        _entries.value = emptyList()
    }

    private fun loadAll(): List<Entry> {
        val out = mutableListOf<Entry>()
        for (f in listOf(rotated, file)) {
            if (!f.exists()) continue
            BufferedReader(FileReader(f)).use { r ->
                while (true) {
                    val line = r.readLine() ?: break
                    Entry.fromJsonLine(line)?.let { out += it }
                }
            }
        }
        return out.takeLast(MAX_IN_MEMORY)
    }

    enum class Level { Info, Warn, Reject }

    data class Entry(
        val at: Long,
        val level: Level,
        val kind: String,
        val peer: String,
        val detail: String,
    ) {
        fun formattedTime(): String =
            DATE_FMT.format(Date(at))

        internal fun toJsonLine(): String = JSONObject().apply {
            put("at", at)
            put("level", level.name)
            put("kind", kind)
            put("peer", peer)
            put("detail", detail)
        }.toString()

        companion object {
            internal fun fromJsonLine(line: String): Entry? = try {
                val o = JSONObject(line)
                Entry(
                    at = o.optLong("at"),
                    level = runCatching { Level.valueOf(o.optString("level", "Info")) }
                        .getOrDefault(Level.Info),
                    kind = o.optString("kind"),
                    peer = o.optString("peer"),
                    detail = o.optString("detail"),
                )
            } catch (e: Exception) { null }

            private val DATE_FMT = SimpleDateFormat("yyyy-MM-dd HH:mm:ss", Locale.US)
        }
    }

    private companion object {
        const val TAG = "DuktoAudit"
        const val MAX_BYTES = 1L * 1024L * 1024L
        const val MAX_IN_MEMORY = 500
    }
}
