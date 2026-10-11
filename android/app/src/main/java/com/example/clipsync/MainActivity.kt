package com.example.clipsync

import android.Manifest
import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.view.ViewGroup
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.TextView

class MainActivity : Activity() {
    private lateinit var addressInput: EditText
    private var pendingAddress: String? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val preferences = getSharedPreferences("clipboard-sync", MODE_PRIVATE)

        val layout = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(24, 32, 24, 24)
        }
        layout.addView(TextView(this).apply {
            text = "Debian 中转服务器 IP / 主机名"
            textSize = 18f
        })
        addressInput = EditText(this).apply {
            setSingleLine(true)
            hint = "例如 192.168.1.10"
            setText(preferences.getString("server", ""))
        }
        layout.addView(addressInput, matchWidth())
        layout.addView(TextView(this).apply {
            text = "UDP 端口固定为 65432。仅在可信任的同一局域网使用；广播内容没有加密或认证。Android 10 及以上可能限制后台读取剪贴板。"
            textSize = 14f
            setPadding(0, 12, 0, 16)
        })
        layout.addView(Button(this).apply {
            text = "启动同步"
            setOnClickListener { startSync() }
        }, matchWidth())
        layout.addView(Button(this).apply {
            text = "停止同步"
            setOnClickListener {
                stopService(Intent(this@MainActivity, ClipboardSyncService::class.java))
            }
        }, matchWidth())
        setContentView(layout)
    }

    private fun matchWidth(): LinearLayout.LayoutParams =
        LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT)

    private fun startSync() {
        val address = addressInput.text.toString().trim()
        if (address.isEmpty()) {
            addressInput.error = "请输入服务器地址"
            return
        }
        getSharedPreferences("clipboard-sync", MODE_PRIVATE)
            .edit().putString("server", address).apply()

        if (Build.VERSION.SDK_INT >= 33 &&
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            pendingAddress = address
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), REQUEST_NOTIFICATIONS)
            return
        }
        launchService(address)
    }

    private fun launchService(address: String) {
        val intent = Intent(this, ClipboardSyncService::class.java)
            .setAction(ClipboardSyncService.ACTION_START)
            .putExtra(ClipboardSyncService.EXTRA_SERVER, address)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            startForegroundService(intent)
        } else {
            startService(intent)
        }
    }

    @Deprecated("Permission callback required for minSdk 21")
    override fun onRequestPermissionsResult(
        requestCode: Int,
        permissions: Array<out String>,
        grantResults: IntArray
    ) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == REQUEST_NOTIFICATIONS) {
            pendingAddress?.let(::launchService)
            pendingAddress = null
        }
    }

    private companion object {
        const val REQUEST_NOTIFICATIONS = 10
    }
}
