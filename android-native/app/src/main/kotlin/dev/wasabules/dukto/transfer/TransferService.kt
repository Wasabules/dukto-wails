package dev.wasabules.dukto.transfer

import android.app.Notification
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import dev.wasabules.dukto.DuktoApp
import dev.wasabules.dukto.MainActivity

/**
 * Foreground service that anchors active transfers so the OS doesn't kill
 * the process under Doze / app standby while bytes are flying.
 *
 * The service is started by [DuktoEngine] when a transfer's Started event
 * fires and stopped on the matching terminal event (TextReceived /
 * FilesReceived / Sent / Failed). The notification shows live progress.
 *
 * Why a service instead of a `setForeground(...)` from a process-scope
 * coroutine: only `Service` instances can use the `dataSync` foreground type
 * required by Android 14+; an Activity-bound foreground notification is
 * tied to the visible UI and disappears as soon as the user backgrounds the
 * app, exposing the transfer to the killer.
 */
class TransferService : Service() {

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_UPDATE -> {
                val title = intent.getStringExtra(EXTRA_TITLE).orEmpty()
                val body = intent.getStringExtra(EXTRA_BODY).orEmpty()
                val progress = intent.getIntExtra(EXTRA_PROGRESS, -1)
                val maxProgress = intent.getIntExtra(EXTRA_MAX, 0)
                promoteForeground(buildNotification(title, body, progress, maxProgress))
            }
            ACTION_STOP -> {
                // The caller delivered this intent via startForegroundService,
                // so the system armed a 5-second startForeground deadline. We
                // MUST call startForeground before stopping or the kernel
                // kills the process with ForegroundServiceDidNotStartInTime —
                // even when the matching ACTION_UPDATE was never sent (e.g.
                // a session rejected by RefuseCleartext before any progress).
                promoteForeground(buildNotification("Dukto", "Stopping…", 0, 0))
                stopForeground(STOP_FOREGROUND_REMOVE)
                stopSelf()
            }
        }
        return START_NOT_STICKY
    }

    private fun promoteForeground(n: Notification) {
        ServiceCompat.startForeground(
            this, NOTIF_ID, n,
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE)
                ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC else 0,
        )
    }

    private fun buildNotification(title: String, body: String, progress: Int, max: Int): Notification {
        val openApp = PendingIntent.getActivity(
            this, 0,
            Intent(this, MainActivity::class.java).apply {
                addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP)
            },
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        return NotificationCompat.Builder(this, DuktoApp.CHANNEL_TRANSFERS)
            .setSmallIcon(android.R.drawable.stat_sys_upload)
            .setContentTitle(title.ifEmpty { "Dukto" })
            .setContentText(body)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setContentIntent(openApp)
            .apply {
                if (max > 0) setProgress(max, progress.coerceAtLeast(0), false)
                else setProgress(0, 0, true)
            }
            .build()
    }

    companion object {
        const val ACTION_UPDATE = "dukto.transfer.UPDATE"
        const val ACTION_STOP = "dukto.transfer.STOP"
        const val EXTRA_TITLE = "title"
        const val EXTRA_BODY = "body"
        const val EXTRA_PROGRESS = "progress"
        const val EXTRA_MAX = "max"
        private const val NOTIF_ID = 0xD17C0
    }
}

/** Helper bag the engine uses to push notification updates without dragging
 * Service constants into call sites. */
object TransferNotifier {
    fun update(ctx: Context, title: String, body: String, progress: Int, max: Int) {
        val intent = Intent(ctx, TransferService::class.java).apply {
            action = TransferService.ACTION_UPDATE
            putExtra(TransferService.EXTRA_TITLE, title)
            putExtra(TransferService.EXTRA_BODY, body)
            putExtra(TransferService.EXTRA_PROGRESS, progress)
            putExtra(TransferService.EXTRA_MAX, max)
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            ctx.startForegroundService(intent)
        } else {
            ctx.startService(intent)
        }
    }

    fun stop(ctx: Context) {
        val intent = Intent(ctx, TransferService::class.java).apply {
            action = TransferService.ACTION_STOP
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            ctx.startForegroundService(intent)
        } else {
            ctx.startService(intent)
        }
    }
}
