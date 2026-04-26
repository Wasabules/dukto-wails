package dev.wasabules.dukto.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog

/**
 * Confirmation gate for the "Trust fingerprint as-is" peer-card action.
 *
 * Pinning a freshly-advertised fingerprint without verifying it
 * out-of-band exposes a first-contact MitM window — anyone
 * impersonating the peer on the LAN at this moment becomes the
 * forever-trusted identity. We make the user read the trade-off
 * before committing to it; the recommended path is the 5-word PSK
 * pairing flow (Pair via 5-word code…).
 */
@Composable
fun TrustFingerprintDialog(
    peerLabel: String,
    fingerprint: String,
    onConfirm: () -> Unit,
    onCancel: () -> Unit,
) {
    Dialog(onDismissRequest = onCancel) {
        Column(
            modifier = Modifier
                .clip(RoundedCornerShape(16.dp))
                .background(MaterialTheme.colorScheme.surface)
                .padding(20.dp)
                .fillMaxWidth()
                .heightIfSmall()
                .verticalScroll(rememberScrollState()),
        ) {
            Text(
                "⚠️ Trust this fingerprint without verification?",
                style = MaterialTheme.typography.titleMedium,
                color = MaterialTheme.colorScheme.tertiary,
            )
            Spacer(Modifier.height(8.dp))
            Text(
                "You're about to pin $peerLabel's current Ed25519 fingerprint",
                style = MaterialTheme.typography.bodySmall,
            )
            Spacer(Modifier.height(4.dp))
            Text(
                fingerprint.ifEmpty { "—" },
                style = MaterialTheme.typography.bodyMedium.copy(fontFamily = FontFamily.Monospace),
            )
            Spacer(Modifier.height(4.dp))
            Text(
                "without any out-of-band check. This is called TOFU (Trust-On-First-Use).",
                style = MaterialTheme.typography.bodySmall,
            )

            Spacer(Modifier.height(12.dp))
            Text("Why this is weaker than the 5-word code", style = MaterialTheme.typography.titleSmall)
            Spacer(Modifier.height(4.dp))
            BulletText(
                "No protection against a first-contact MitM. Anyone impersonating this peer on the LAN right now would become the identity you trust forever.",
            )
            BulletText(
                "One-sided trust. Pinning here only changes how your outbound transfers are encrypted. For the peer to talk encrypted back, they need to pin your fingerprint too.",
            )
            BulletText(
                "\"Refuse cleartext\" mode requires both peers paired. TOFU on one side alone won't unblock the channel.",
            )

            Spacer(Modifier.height(12.dp))
            Text("Recommended", style = MaterialTheme.typography.titleSmall)
            Spacer(Modifier.height(4.dp))
            Text(
                "Use the Pair via 5-word code action instead. Both peers derive the same one-shot PSK from the passphrase; the Noise XXpsk2 handshake authenticates the keys mutually before either side commits a pin, so a MitM can't stand in.",
                style = MaterialTheme.typography.bodySmall,
            )

            Spacer(Modifier.height(16.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = onConfirm) { Text("Trust anyway") }
                OutlinedButton(onClick = onCancel) { Text("Cancel") }
            }
        }
    }
}

@Composable
private fun BulletText(text: String, modifier: Modifier = Modifier) {
    Row(modifier = modifier) {
        Text("• ", style = MaterialTheme.typography.bodySmall)
        Text(text, style = MaterialTheme.typography.bodySmall)
    }
}

/** Lets the dialog grow naturally; the inner verticalScroll handles
 *  overflow on tall content. */
@Composable
private fun Modifier.heightIfSmall(): Modifier = this
