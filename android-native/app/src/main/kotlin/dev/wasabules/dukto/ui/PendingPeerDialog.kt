package dev.wasabules.dukto.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import dev.wasabules.dukto.policy.PeerChoice
import dev.wasabules.dukto.policy.PendingRequest
import kotlinx.coroutines.delay

/**
 * "Confirm new peer" modal — only shown when settings.confirmUnknownPeers is on
 * and a session arrives from a signature not in approvedPeers.
 *
 * Four exits: Allow once / Allow always / Reject / Block. Auto-rejects after
 * the request's deadline; the engine's policy applies the same fallback if the
 * UI dies before the user picks.
 */
@Composable
fun PendingPeerDialog(
    request: PendingRequest,
    onChoice: (String, PeerChoice) -> Unit,
) {
    var nowMs by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(request.id) {
        while (nowMs < request.deadline) {
            delay(500)
            nowMs = System.currentTimeMillis()
        }
    }
    val secondsLeft = ((request.deadline - nowMs).coerceAtLeast(0L) / 1000L).toInt()

    AlertDialog(
        onDismissRequest = { /* must choose explicitly */ },
        title = { Text("Allow incoming transfer?") },
        text = {
            Column {
                Text(request.peerSignature, style = MaterialTheme.typography.titleSmall)
                Text(
                    request.peerAddress,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.height(12.dp))
                Text(
                    "This peer hasn't been approved before. Auto-rejects in ${secondsLeft}s.",
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
        },
        confirmButton = {
            Column(modifier = Modifier.fillMaxWidth()) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Button(
                        onClick = { onChoice(request.id, PeerChoice.AllowOnce) },
                        modifier = Modifier.weight(1f),
                    ) { Text("Allow once") }
                    Button(
                        onClick = { onChoice(request.id, PeerChoice.AllowAlways) },
                        modifier = Modifier.weight(1f),
                    ) { Text("Allow always") }
                }
                Spacer(Modifier.height(8.dp))
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    OutlinedButton(
                        onClick = { onChoice(request.id, PeerChoice.Reject) },
                        modifier = Modifier.weight(1f),
                    ) { Text("Reject") }
                    TextButton(
                        onClick = { onChoice(request.id, PeerChoice.Block) },
                        modifier = Modifier.weight(1f),
                    ) { Text("Block forever") }
                }
            }
        },
    )
}
