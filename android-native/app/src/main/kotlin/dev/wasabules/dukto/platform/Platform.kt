package dev.wasabules.dukto.platform

/**
 * OS identity for the buddy signature ("<user> at <host> (<platform>)").
 *
 * Reference: wails/internal/platform/, original Qt code in platform.cpp
 * and androidutils.cpp.
 *
 * The platform token MUST be the literal "Android" so legacy peers
 * render the right icon (see docs/PROTOCOL.md §2.2).
 */
const val PLATFORM_TOKEN: String = "Android"

/**
 * Hostname for the signature.
 * Qt build uses Settings.Global "device_name", falling back to
 * Build.MODEL with spaces replaced by '-'. Mirror that here.
 *
 * TODO: implement using Settings.Global.getString(resolver, "device_name")
 * with a Build.MODEL fallback.
 */
fun deviceName(): String = TODO("port from androidutils.cpp::deviceName()")
