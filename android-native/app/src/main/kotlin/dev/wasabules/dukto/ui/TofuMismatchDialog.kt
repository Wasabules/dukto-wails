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
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import dev.wasabules.dukto.TofuMismatch

/**
 * "Identity changed" alert. Triggered when an inbound v2 handshake
 * produces a remote_static that doesn't match the X25519 derived from
 * the peer's already-pinned Ed25519 fingerprint.
 *
 * Two ways to react:
 *   - **Re-pair**: open the PSK pairing flow so the user can verify
 *     the new key out-of-band (5-word code) before re-trusting it.
 *   - **Unpin & reject**: drop the existing pinning and treat the new
 *     key as hostile. Subsequent sessions fall back to cleartext (or
 *     get refused entirely if RefuseCleartext is on).
 */
@Composable
fun TofuMismatchDialog(
    mismatch: TofuMismatch,
    onClose: () -> Unit,
    onRepair: () -> Unit,
    onUnpin: () -> Unit,
) {
    Dialog(onDismissRequest = onClose) {
        Column(
            modifier = Modifier
                .clip(RoundedCornerShape(16.dp))
                .background(MaterialTheme.colorScheme.surface)
                .padding(20.dp)
                .fillMaxWidth(),
        ) {
            Text(
                "⚠️ Identity changed",
                style = MaterialTheme.typography.titleMedium,
                color = MaterialTheme.colorScheme.error,
            )
            Spacer(Modifier.height(6.dp))
            Text(
                "Peer ${if (mismatch.label.isNotBlank()) mismatch.label else mismatch.address}" +
                    " (${mismatch.address}) just presented a different long-term key" +
                    " than the one you previously paired with.",
                style = MaterialTheme.typography.bodySmall,
            )
            Spacer(Modifier.height(8.dp))
            Text("Pinned", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Text(mismatch.oldFingerprint, style = MaterialTheme.typography.bodySmall.copy(fontFamily = FontFamily.Monospace))
            Spacer(Modifier.height(4.dp))
            Text("New key", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Text(mismatch.newFingerprint, style = MaterialTheme.typography.bodySmall.copy(fontFamily = FontFamily.Monospace))
            Spacer(Modifier.height(8.dp))
            Text(
                "Usually means the peer reinstalled Dukto or reset the key file. " +
                    "It can also mean someone is impersonating them on the LAN. " +
                    "Verify the new fingerprint with the peer out-of-band before trusting it.",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.height(12.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = onRepair) { Text("Re-pair") }
                OutlinedButton(onClick = onUnpin) { Text("Unpin & reject") }
            }
            Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                TextButton(onClick = onClose) { Text("Close") }
            }
        }
    }
}
