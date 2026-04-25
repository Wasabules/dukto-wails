package dev.wasabules.dukto.platform

import android.content.Context
import android.os.Build
import android.provider.Settings
import dev.wasabules.dukto.protocol.PLATFORM_ANDROID
import dev.wasabules.dukto.protocol.buildSignature

/**
 * Mirror of `wails/internal/platform` and the original Qt `androidutils.cpp`
 * device-name logic.
 *
 * The platform token must remain the literal "Android" so legacy peers render
 * the right icon (see docs/PROTOCOL.md §2.2).
 */

const val PLATFORM_TOKEN: String = PLATFORM_ANDROID

/**
 * Best-effort device name. Qt build uses `Settings.Global.device_name`,
 * falling back to `Build.MODEL` with spaces → '-'. Same here.
 */
fun deviceName(context: Context): String {
    val secure = runCatching {
        Settings.Global.getString(context.contentResolver, "device_name")
    }.getOrNull()
    val raw = secure?.takeIf { it.isNotBlank() } ?: Build.MODEL ?: "Android"
    return raw.trim().replace(' ', '-')
}

/**
 * Compose the HELLO signature. [buddyName] is what the user typed in their
 * profile (may be empty — caller should fall back to the OS user; on Android
 * we don't have a "login name" so the brand/model is the standard fallback).
 */
fun currentSignature(context: Context, buddyName: String): String {
    val name = buddyName.trim().ifEmpty { Build.MANUFACTURER ?: "Android" }
    return buildSignature(name.replaceFirstChar { it.uppercase() }, deviceName(context), PLATFORM_TOKEN)
}
