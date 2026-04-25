package dev.wasabules.dukto

import android.app.Application
import android.app.NotificationChannel
import android.app.NotificationManager
import android.os.Build

class DuktoApp : Application() {

    lateinit var engine: DuktoEngine
        private set

    override fun onCreate() {
        super.onCreate()
        instance = this
        engine = DuktoEngine(this)
        engine.start()
        createNotificationChannel()
    }

    override fun onTerminate() {
        engine.stop()
        super.onTerminate()
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val channel = NotificationChannel(
            CHANNEL_TRANSFERS,
            "Transfers",
            NotificationManager.IMPORTANCE_LOW,
        ).apply { description = "Background notifications for active Dukto transfers" }
        getSystemService(NotificationManager::class.java)?.createNotificationChannel(channel)
    }

    companion object {
        const val CHANNEL_TRANSFERS = "dukto.transfers"
        @Volatile lateinit var instance: DuktoApp private set
    }
}
