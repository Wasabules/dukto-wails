package dev.wasabules.dukto.ui

import com.journeyapps.barcodescanner.CaptureActivity

/**
 * zxing-android-embedded's stock [CaptureActivity] is locked to landscape
 * via its manifest entry. The rest of Dukto Native is portrait-only, so
 * the abrupt rotation when the user taps "Scan QR" feels broken.
 *
 * Subclassing the activity here lets us register a separate manifest
 * entry with `android:screenOrientation="portrait"` and point
 * [ScanOptions.setCaptureActivity] at it — no behaviour change beyond
 * the orientation lock.
 */
class PortraitCaptureActivity : CaptureActivity()
