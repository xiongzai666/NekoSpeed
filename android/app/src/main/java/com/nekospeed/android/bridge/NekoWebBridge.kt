package com.nekospeed.android.bridge

import android.app.Activity
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.os.Handler
import android.os.Looper
import android.webkit.JavascriptInterface
import com.nekospeed.android.service.ServerUrlParser
import com.nekospeed.android.util.ShareHelper
import com.nekospeed.android.util.VpnDetector
import java.io.File
import java.lang.ref.WeakReference
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

class NekoWebBridge(
    activity: Activity,
    private val onThemeChangedCallback: (Boolean) -> Unit,
    private val onRunningStateChangedCallback: (Boolean) -> Unit
) {

    private val appContext: Context = activity.applicationContext
    private val activityRef = WeakReference(activity)
    private val mainHandler = Handler(Looper.getMainLooper())

    @JavascriptInterface
    fun isVpnActive(): Boolean = runCatching {
        VpnDetector.isVpnActive(appContext)
    }.getOrDefault(false)

    @JavascriptInterface
    fun copyText(text: String): Boolean {
        return runOnMainThread(false) {
            val cm = appContext.getSystemService(Context.CLIPBOARD_SERVICE) as? ClipboardManager
                ?: return@runOnMainThread false
            cm.setPrimaryClip(ClipData.newPlainText("NekoSpeed", text))
            true
        }
    }

    @JavascriptInterface
    fun saveImageToGallery(runId: String, filename: String): String {
        val file = ServerUrlParser.resolveSafeWebFile(appContext.filesDir, runId, filename)
            ?: return "error:非法文件路径"
        if (!file.exists() || !file.isFile) {
            return "error:文件不存在"
        }
        val savedUri = runCatching {
            ShareHelper.saveImageToGallery(appContext, file)
        }.getOrElse { err ->
            return "error:${err.message ?: "保存失败"}"
        }
        if (savedUri != null) {
            return "saved"
        }
        val shared = runOnMainThread(false) {
            ShareHelper.shareFile(shareContext(), file, "保存报告图片")
        }
        return if (shared) "shared" else "error:保存或分享失败"
    }

    @JavascriptInterface
    fun shareFile(runId: String, filename: String): Boolean {
        val file = ServerUrlParser.resolveSafeWebFile(appContext.filesDir, runId, filename)
            ?: return false
        if (!file.exists() || !file.isFile) {
            return false
        }
        return runOnMainThread(false) {
            ShareHelper.shareFile(shareContext(), file, "分享文件")
        }
    }

    @JavascriptInterface
    fun exportCsv(filename: String, content: String): Boolean {
        if (content.length > MAX_EXPORT_BYTES) return false
        val bytes = content.toByteArray(Charsets.UTF_8)
        if (bytes.size > MAX_EXPORT_BYTES) return false

        val target = ServerUrlParser.resolveSafeExportFile(appContext.filesDir, filename)
            ?: return false
        val exportDir = target.parentFile ?: return false
        if (!exportDir.exists() && !exportDir.mkdirs()) return false

        val tmp = File(exportDir, "${target.name}.tmp")
        val wrote = runCatching {
            tmp.writeBytes(bytes)
            if (!tmp.renameTo(target)) {
                if (target.exists()) target.delete()
                if (!tmp.renameTo(target)) {
                    tmp.delete()
                    return false
                }
            }
            pruneOldExports(exportDir, target)
            true
        }.getOrElse {
            tmp.delete()
            false
        }
        if (!wrote) return false

        return runOnMainThread(false) {
            ShareHelper.shareFile(shareContext(), target, "导出 CSV")
        }
    }

    @JavascriptInterface
    fun onThemeChanged(isDark: Boolean) {
        mainHandler.post {
            onThemeChangedCallback(isDark)
        }
    }

    @JavascriptInterface
    fun onRunningStateChanged(running: Boolean) {
        mainHandler.post {
            onRunningStateChangedCallback(running)
        }
    }

    private fun shareContext(): Context {
        val activity = activityRef.get()
        return if (activity != null && !activity.isFinishing && !activity.isDestroyed) {
            activity
        } else {
            appContext
        }
    }

    private fun pruneOldExports(exportDir: File, keepFile: File) {
        val files = exportDir.listFiles { f -> f.isFile && !f.name.endsWith(".tmp") } ?: return
        if (files.size <= MAX_EXPORT_FILES) return
        files
            .filter { it.absolutePath != keepFile.absolutePath }
            .sortedByDescending { it.lastModified() }
            .drop(MAX_EXPORT_FILES - 1)
            .forEach { it.delete() }
    }

    private fun <T> runOnMainThread(defaultValue: T, block: () -> T): T {
        if (Looper.myLooper() == Looper.getMainLooper()) {
            return runCatching(block).getOrDefault(defaultValue)
        }
        val latch = CountDownLatch(1)
        var result = defaultValue
        val posted = mainHandler.post {
            try {
                result = runCatching(block).getOrDefault(defaultValue)
            } finally {
                latch.countDown()
            }
        }
        if (!posted) return defaultValue
        runCatching { latch.await(2, TimeUnit.SECONDS) }
        return result
    }

    companion object {
        private const val MAX_EXPORT_BYTES = 16 * 1024 * 1024
        private const val MAX_EXPORT_FILES = 20
    }
}
