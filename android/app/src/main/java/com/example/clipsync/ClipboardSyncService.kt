package com.example.clipsync

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.wifi.WifiManager
import android.os.Build
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.os.SystemClock
import android.util.Log
import org.json.JSONObject
import java.net.DatagramPacket
import java.net.DatagramSocket
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.SocketException
import java.net.SocketTimeoutException
import java.nio.charset.StandardCharsets
import java.util.UUID

class ClipboardSyncService : Service() {
    private val mainHandler = Handler(Looper.getMainLooper())
    private lateinit var clipboard: ClipboardManager
    private var socket: DatagramSocket? = null
    private var remoteAddress: InetAddress? = null
    private var origin = ""
    private var lastText: String? = null
    private var pendingRemoteText: String? = null
    private var nextWriteAttemptAt = 0L
    private var isWritingRemote = false
    private var multicastLock: WifiManager.MulticastLock? = null
    @Volatile private var active = false

    private val clipboardListener = ClipboardManager.OnPrimaryClipChangedListener {
        pollClipboard()
    }

    private val pollRunnable = object : Runnable {
        override fun run() {
            if (!active) return
            pollClipboard()
            mainHandler.postDelayed(this, POLL_INTERVAL_MS)
        }
    }

    override fun onCreate() {
        super.onCreate()
        clipboard = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
        createNotificationChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val server = intent?.getStringExtra(EXTRA_SERVER)
            ?: getSharedPreferences("clipboard-sync", MODE_PRIVATE).getString("server", null)
        if (server.isNullOrBlank()) {
            stopSelf(startId)
            return START_NOT_STICKY
        }
        startForegroundCompat(notification("正在连接 $server"))
        startSync(server)
        return START_STICKY
    }

    private fun startSync(server: String) {
        stopSync()
        try {
            remoteAddress = InetAddress.getByName(server)
            origin = UUID.randomUUID().toString()
            val wifi = getSystemService(Context.WIFI_SERVICE) as WifiManager
            multicastLock = wifi.createMulticastLock("clipboard-sync").apply {
                setReferenceCounted(false)
                acquire()
            }
            socket = DatagramSocket(null).apply {
                reuseAddress = true
                broadcast = true
                bind(InetSocketAddress(UDP_PORT))
                soTimeout = RECEIVE_TIMEOUT_MS
            }
            lastText = readClipboardText()
            pendingRemoteText = null
            nextWriteAttemptAt = 0L
            active = true
            clipboard.addPrimaryClipChangedListener(clipboardListener)
            mainHandler.post(pollRunnable)
            updateNotification("已连接 $server:$UDP_PORT")
            Thread({ receiveLoop() }, "clipboard-udp-receiver").start()
        } catch (error: Exception) {
            Log.e(TAG, "Unable to start clipboard sync", error)
            updateNotification("启动失败：${error.message ?: error.javaClass.simpleName}")
            stopSync()
        }
    }

    private fun stopSync() {
        active = false
        mainHandler.removeCallbacks(pollRunnable)
        clipboard.removePrimaryClipChangedListener(clipboardListener)
        socket?.close()
        socket = null
        remoteAddress = null
        multicastLock?.let {
            if (it.isHeld) it.release()
        }
        multicastLock = null
    }

    private fun pollClipboard() {
        if (!active) return
        try {
            val pendingText = pendingRemoteText
            if (pendingText != null) {
                writeRemoteClipboard(pendingText)
                return
            }
            val text = readClipboardText() ?: return
            if (text == lastText) return
            sendClipboard(text)
        } catch (error: SecurityException) {
            Log.w(TAG, "Android denied clipboard access while the app is in the background", error)
        } catch (error: Exception) {
            Log.e(TAG, "Unable to read or send clipboard text", error)
        }
    }

    private fun readClipboardText(): String? {
        val clip = clipboard.primaryClip ?: return null
        if (clip.itemCount == 0) return null
        return clip.getItemAt(0).text?.toString()
    }

    private fun sendClipboard(text: String) {
        if (text.contains('\u0000') || text.toByteArray(StandardCharsets.UTF_8).size > MAX_TEXT_BYTES) {
            Log.w(TAG, "Clipboard text contains NUL or exceeds $MAX_TEXT_BYTES UTF-8 bytes")
            return
        }
        val data = JSONObject()
            .put("version", PROTOCOL_VERSION)
            .put("type", MESSAGE_TYPE)
            .put("origin", origin)
            .put("text", text)
            .toString()
            .toByteArray(StandardCharsets.UTF_8)
        if (data.size > MAX_PACKET_SIZE) {
            Log.w(TAG, "Encoded clipboard update exceeds $MAX_PACKET_SIZE bytes")
            return
        }
        val target = remoteAddress ?: return
        val activeSocket = socket ?: return
        activeSocket.send(DatagramPacket(data, data.size, target, UDP_PORT))
        lastText = text
    }

    private fun receiveLoop() {
        val buffer = ByteArray(MAX_PACKET_SIZE + 1)
        while (active) {
            val packet = DatagramPacket(buffer, buffer.size)
            try {
                val activeSocket = socket ?: return
                activeSocket.receive(packet)
                val message = parseMessage(packet.data, packet.length) ?: continue
                if (message.first == origin) continue
                mainHandler.post {
                    if (!active) return@post
                    if (message.second == lastText && pendingRemoteText == null) {
                        updateNotification("收到广播，剪贴板内容相同")
                        return@post
                    }
                    pendingRemoteText = message.second
                    writeRemoteClipboard(message.second)
                }
            } catch (_: SocketTimeoutException) {
                continue
            } catch (error: SocketException) {
                if (active) Log.e(TAG, "UDP socket failed", error)
                return
            } catch (error: Exception) {
                if (active) Log.e(TAG, "Unable to receive clipboard packet", error)
            }
        }
    }

    private fun writeRemoteClipboard(text: String) {
        if (isWritingRemote || SystemClock.elapsedRealtime() < nextWriteAttemptAt) return
        isWritingRemote = true
        try {
            clipboard.setPrimaryClip(ClipData.newPlainText("Synced clipboard", text))
            lastText = text
            pendingRemoteText = null
            nextWriteAttemptAt = 0L
            Log.i(TAG, "Clipboard update received and written")
            updateNotification("已同步剪贴板")
        } catch (error: Exception) {
            nextWriteAttemptAt = SystemClock.elapsedRealtime() + WRITE_RETRY_INTERVAL_MS
            Log.e(TAG, "Unable to write clipboard; will retry", error)
            updateNotification("收到更新，但写入失败；稍后重试")
        } finally {
            isWritingRemote = false
        }
    }

    private fun parseMessage(data: ByteArray, length: Int): Pair<String, String>? {
        if (length <= 0 || length > MAX_PACKET_SIZE) return null
        return try {
            val json = JSONObject(String(data, 0, length, StandardCharsets.UTF_8))
            val version = json.optInt("version", -1)
            val type = json.optString("type")
            val messageOrigin = json.optString("origin")
            val text = json.optString("text")
            if (version != PROTOCOL_VERSION || type != MESSAGE_TYPE ||
                messageOrigin.isBlank() || messageOrigin.length > 64 ||
                text.contains('\u0000') ||
                text.toByteArray(StandardCharsets.UTF_8).size > MAX_TEXT_BYTES
            ) {
                Log.w(TAG, "Discarded invalid clipboard packet")
                null
            } else {
                messageOrigin to text
            }
        } catch (error: Exception) {
            Log.w(TAG, "Discarded malformed clipboard packet", error)
            null
        }
    }

    private fun startForegroundCompat(notification: Notification) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            startForeground(NOTIFICATION_ID, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC)
        } else {
            startForeground(NOTIFICATION_ID, notification)
        }
    }

    private fun notification(status: String): Notification {
        val openApp = Intent(this, MainActivity::class.java)
        val immutableFlag = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
            PendingIntent.FLAG_IMMUTABLE
        } else {
            0
        }
        val flags = PendingIntent.FLAG_UPDATE_CURRENT or immutableFlag
        val pendingIntent = PendingIntent.getActivity(this, 0, openApp, flags)
        val builder = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            Notification.Builder(this, CHANNEL_ID)
        } else {
            @Suppress("DEPRECATION")
            Notification.Builder(this)
        }
        return builder
            .setSmallIcon(android.R.drawable.stat_notify_sync)
            .setContentTitle("Clipboard Sync")
            .setContentText(status)
            .setContentIntent(pendingIntent)
            .setOngoing(true)
            .build()
    }

    private fun updateNotification(status: String) {
        val manager = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        manager.notify(NOTIFICATION_ID, notification(status))
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(CHANNEL_ID, "Clipboard sync", NotificationManager.IMPORTANCE_LOW)
            (getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager)
                .createNotificationChannel(channel)
        }
    }

    override fun onDestroy() {
        stopSync()
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
            stopForeground(STOP_FOREGROUND_REMOVE)
        } else {
            @Suppress("DEPRECATION")
            stopForeground(true)
        }
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    companion object {
        const val ACTION_START = "com.example.clipsync.START"
        const val EXTRA_SERVER = "server"

        private const val TAG = "ClipboardSync"
        private const val CHANNEL_ID = "clipboard-sync"
        private const val NOTIFICATION_ID = 65432
        private const val UDP_PORT = 65432
        private const val RECEIVE_TIMEOUT_MS = 1000
        private const val POLL_INTERVAL_MS = 500L
        private const val WRITE_RETRY_INTERVAL_MS = 2000L
        private const val PROTOCOL_VERSION = 1
        private const val MESSAGE_TYPE = "clipboard"
        private const val MAX_TEXT_BYTES = 48 * 1024
        private const val MAX_PACKET_SIZE = 60 * 1024
    }
}
