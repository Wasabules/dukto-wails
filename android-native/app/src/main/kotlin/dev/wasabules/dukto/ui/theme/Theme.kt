package dev.wasabules.dukto.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

/**
 * Fixed Dukto colour scheme — keyed off the original Qt build's brand green
 * (#248b00, see theme.cpp at the repo root). Material You / dynamic colour is
 * intentionally NOT used here: the goal is a consistent, branded look across
 * peers, so two devices on the same LAN don't render Dukto with two
 * wildly-different accent colours just because their wallpapers differ.
 *
 * Light/dark variants mirror the Qt theme.cpp light/dark blocks (#fcfcfc
 * surface in light, #333333 in dark, etc.) — keeps the visual continuity for
 * users coming from the Qt6 desktop.
 */

private val DuktoGreen = Color(0xFF248B00)
private val DuktoGreenLight = Color(0xFF42A41A)
private val DuktoGreenDark = Color(0xFF1C6F00)
private val OnPrimary = Color(0xFFFCFCFC)

private val LightColors = lightColorScheme(
    primary = DuktoGreen,
    onPrimary = OnPrimary,
    primaryContainer = Color(0xFFD7F0C9),
    onPrimaryContainer = Color(0xFF0F2A00),
    secondary = DuktoGreenDark,
    onSecondary = OnPrimary,
    background = Color(0xFFFCFCFC),
    onBackground = Color(0xFF333333),
    surface = Color(0xFFFCFCFC),
    onSurface = Color(0xFF333333),
    surfaceVariant = Color(0xFFEEEEEE),
    onSurfaceVariant = Color(0xFF555555),
    outline = Color(0xFFD0D0D0),
    error = Color(0xFFB91C1C),
    onError = OnPrimary,
)

private val DarkColors = darkColorScheme(
    primary = DuktoGreenLight,
    onPrimary = Color(0xFF0F2A00),
    primaryContainer = DuktoGreenDark,
    onPrimaryContainer = Color(0xFFD7F0C9),
    secondary = DuktoGreen,
    onSecondary = OnPrimary,
    background = Color(0xFF222222),
    onBackground = Color(0xFFCCCCCC),
    surface = Color(0xFF333333),
    onSurface = Color(0xFFCCCCCC),
    surfaceVariant = Color(0xFF444444),
    onSurfaceVariant = Color(0xFF999999),
    outline = Color(0xFF555555),
    error = Color(0xFFEF6B6B),
    onError = Color(0xFF330000),
)

@Composable
fun DuktoTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit,
) {
    MaterialTheme(
        colorScheme = if (darkTheme) DarkColors else LightColors,
        content = content,
    )
}
