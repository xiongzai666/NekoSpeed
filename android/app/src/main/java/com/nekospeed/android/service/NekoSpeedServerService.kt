package com.nekospeed.android.service

import android.annotation.SuppressLint
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import androidx.core.content.pm.PackageInfoCompat
import com.nekospeed.android.MainActivity
import com.nekospeed.android.R
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import java.io.BufferedReader
import java.io.File
import java.io.IOException
import java.io.InputStreamReader
import java.security.SecureRandom
import java.util.Locale
import java.util.concurrent.TimeUnit

class NekoSpeedServerService : Service() {

    sealed class ServerState {
        data object Idle : ServerState()
        data object Starting : ServerState()
        data class Ready(
            val baseUrl: String,
            val port: Int,
            val token: String
        ) : ServerState()
        data class Error(val message: String) : ServerState()
    }

    private val serviceScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private var launchJob: Job? = null

    @Volatile
    private var serverProcess: Process? = null

    @Volatile
    private var stoppingIntentionally = false

    private val recentLogLock = Any()
    private val recentLogs = ArrayDeque<String>(MAX_LOG_LINES)

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        activeInstance = this
        createNotificationChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_STOP -> {
                shutdownServerAndStop(
                    startId = startId,
                    errorMessage = "本地服务已停止"
                )
            }
            ACTION_RESTART -> {
                promoteToForeground(currentNotificationText())
                launchServer(forceRestart = true)
            }
            ACTION_START, null -> {
                promoteToForeground(currentNotificationText())
                launchServer(forceRestart = false)
            }
        }
        return START_NOT_STICKY
    }

    @SuppressLint("NewApi")
    @Suppress("UNUSED_PARAMETER")
    override fun onTimeout(startId: Int, fgsType: Int) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.VANILLA_ICE_CREAM) {
            super.onTimeout(startId, fgsType)
        }
        shutdownServerAndStop(
            startId = startId,
            errorMessage = "后台服务达到系统运行时长限制，已自动停止"
        )
    }

    override fun onTaskRemoved(rootIntent: Intent?) {
        super.onTaskRemoved(rootIntent)
        if (!_isRunningTest.value) {
            shutdownServerAndStop(startId = null, errorMessage = null)
        }
    }

    override fun onDestroy() {
        if (activeInstance === this) {
            activeInstance = null
        }
        stoppingIntentionally = true
        launchJob?.cancel()
        launchJob = null
        val proc = serverProcess
        serverProcess = null
        terminateProcessAsync(proc)
        _isRunningTest.value = false
        if (_serverState.value !is ServerState.Error) {
            _serverState.value = ServerState.Idle
        }
        serviceScope.cancel()
        super.onDestroy()
    }

    private fun launchServer(forceRestart: Boolean) {
        val currentState = _serverState.value
        if (!forceRestart) {
            val alive = serverProcess?.isAlive == true
            if (currentState is ServerState.Starting || (currentState is ServerState.Ready && alive)) {
                return
            }
        }

        stoppingIntentionally = true
        launchJob?.cancel()
        _isRunningTest.value = false
        _serverState.value = ServerState.Starting
        refreshNotification()

        launchJob = serviceScope.launch {
            try {
                stopCurrentProcess(wait = true)
                clearRecentLogs()
                stoppingIntentionally = false

                val soFile = File(applicationInfo.nativeLibraryDir, "libnekospeed.so")
                if (!soFile.exists() || !soFile.isFile) {
                    failWithError("未找到原生核心库：${soFile.absolutePath}")
                    return@launch
                }

                val fontFile = ensureFontExtracted()
                val outDir = File(filesDir, "out").apply { mkdirs() }
                val geoipDir = File(filesDir, "geoip").apply { mkdirs() }
                val token = generateHexToken()

                val pb = ProcessBuilder(
                    soFile.absolutePath,
                    "-web",
                    "127.0.0.1:0",
                    "-web-token",
                    token
                ).apply {
                    directory(filesDir)
                    environment().apply {
                        put("NEKOSPEED_OUT_DIR", outDir.absolutePath)
                        put("NEKOSPEED_GEOIP_DIR", geoipDir.absolutePath)
                        if (fontFile != null) {
                            put("NEKOSPEED_FONT_PATH", fontFile.absolutePath)
                        }
                        put("NEKOSPEED_NO_BROWSER", "1")
                        put("NEKOSPEED_ANDROID_SHELL", "1")
                    }
                }

                val process = pb.start()
                serverProcess = process

                val readyDeferred = CompletableDeferred<ReadyEndpoint>()

                val stdoutJob = launch(Dispatchers.IO) {
                    BufferedReader(InputStreamReader(process.inputStream, Charsets.UTF_8)).use { reader ->
                        while (true) {
                            val line = reader.readLine() ?: break
                            appendRecentLog(line)
                            if (!readyDeferred.isCompleted) {
                                ServerUrlParser.parseReadyUrl(line)?.let { endpoint ->
                                    readyDeferred.complete(endpoint)
                                }
                            }
                        }
                    }
                }

                val stderrJob = launch(Dispatchers.IO) {
                    BufferedReader(InputStreamReader(process.errorStream, Charsets.UTF_8)).use { reader ->
                        while (true) {
                            val line = reader.readLine() ?: break
                            appendRecentLog("[stderr] $line")
                        }
                    }
                }

                val exitMonitorJob = launch(Dispatchers.IO) {
                    val exitCode = process.waitFor()
                    stdoutJob.join()
                    stderrJob.join()
                    if (!readyDeferred.isCompleted) {
                        readyDeferred.completeExceptionally(
                            IOException("测速核心服务提前退出 (code=$exitCode)")
                        )
                    } else if (!stoppingIntentionally && serverProcess === process) {
                        serverProcess = null
                        val logs = snapshotRecentLogs()
                        val detail = if (logs.isNotBlank()) "\n$logs" else ""
                        failWithError("本地服务异常退出 (code=$exitCode)$detail")
                    }
                }

                val endpoint = withTimeoutOrNull(STARTUP_TIMEOUT_MS) {
                    readyDeferred.await()
                }

                if (endpoint == null) {
                    stoppingIntentionally = true
                    stopCurrentProcess(wait = true)
                    val logs = snapshotRecentLogs()
                    val detail = if (logs.isNotBlank()) "\n$logs" else ""
                    failWithError("等待本地服务就绪超时$detail")
                    return@launch
                }

                _serverState.value = ServerState.Ready(
                    baseUrl = endpoint.baseUrl,
                    port = endpoint.port,
                    token = token
                )
                refreshNotification()
                exitMonitorJob.join()
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (!stoppingIntentionally) {
                    stopCurrentProcess(wait = false)
                    val logs = snapshotRecentLogs()
                    val baseMsg = e.message ?: "启动本地服务失败"
                    val detail = if (logs.isNotBlank()) "$baseMsg\n$logs" else baseMsg
                    failWithError(detail)
                }
            }
        }
    }

    private fun failWithError(message: String) {
        _isRunningTest.value = false
        _serverState.value = ServerState.Error(message)
        ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    private fun shutdownServerAndStop(startId: Int?, errorMessage: String?) {
        stoppingIntentionally = true
        launchJob?.cancel()
        launchJob = null
        val proc = serverProcess
        serverProcess = null
        terminateProcessAsync(proc)
        _isRunningTest.value = false
        _serverState.value = if (errorMessage != null) {
            ServerState.Error(errorMessage)
        } else {
            ServerState.Idle
        }
        ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
        if (startId != null) {
            stopSelf(startId)
        } else {
            stopSelf()
        }
    }

    private fun stopCurrentProcess(wait: Boolean) {
        val proc = serverProcess
        serverProcess = null
        if (proc == null) return
        runCatching { proc.destroy() }
        if (wait) {
            val exited = runCatching {
                proc.waitFor(1200, TimeUnit.MILLISECONDS)
            }.getOrDefault(false)
            if (!exited) {
                runCatching { proc.destroyForcibly() }
                runCatching { proc.waitFor(500, TimeUnit.MILLISECONDS) }
            }
        } else {
            terminateProcessAsync(proc)
        }
    }

    private fun terminateProcessAsync(proc: Process?) {
        if (proc == null) return
        runCatching { proc.destroy() }
        Thread(
            {
                val exited = runCatching {
                    proc.waitFor(800, TimeUnit.MILLISECONDS)
                }.getOrDefault(false)
                if (!exited) {
                    runCatching { proc.destroyForcibly() }
                }
            },
            "nekospeed-proc-reaper"
        ).apply {
            isDaemon = true
            start()
        }
    }

    private fun ensureFontExtracted(): File? {
        val fontsDir = File(filesDir, "fonts").apply { mkdirs() }
        val target = File(fontsDir, "NotoSansCJKsc-Regular.otf")
        val stampFile = File(fontsDir, ".font_version")
        val expectedStamp = appVersionStamp()

        if (target.exists() && target.length() > 0L && isStampValid(stampFile, expectedStamp)) {
            return target
        }

        val assetStream = try {
            assets.open(FONT_ASSET)
        } catch (_: IOException) {
            return if (target.exists() && target.length() > 0L) target else null
        }

        val tmp = File(fontsDir, "NotoSansCJKsc-Regular.otf.tmp")
        tmp.delete()
        return try {
            assetStream.use { input ->
                tmp.outputStream().use { output ->
                    input.copyTo(output)
                }
            }
            if (!tmp.renameTo(target)) {
                if (target.exists()) target.delete()
                if (!tmp.renameTo(target)) {
                    tmp.delete()
                    return if (target.exists() && target.length() > 0L) target else null
                }
            }
            stampFile.writeText(expectedStamp)
            target
        } catch (_: IOException) {
            tmp.delete()
            if (target.exists() && target.length() > 0L) target else null
        }
    }

    private fun isStampValid(stampFile: File, expectedStamp: String): Boolean {
        if (!stampFile.exists()) return false
        return runCatching { stampFile.readText().trim() == expectedStamp }.getOrDefault(false)
    }

    private fun appVersionStamp(): String {
        return try {
            val info = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                packageManager.getPackageInfo(
                    packageName,
                    PackageManager.PackageInfoFlags.of(0L)
                )
            } else {
                @Suppress("DEPRECATION")
                packageManager.getPackageInfo(packageName, 0)
            }
            "${PackageInfoCompat.getLongVersionCode(info)}-${info.lastUpdateTime}"
        } catch (_: Exception) {
            "0"
        }
    }

    private fun generateHexToken(): String {
        val bytes = ByteArray(16)
        SecureRandom().nextBytes(bytes)
        return bytes.joinToString("") { "%02x".format(Locale.ROOT, it) }
    }

    private fun appendRecentLog(line: String) {
        synchronized(recentLogLock) {
            if (recentLogs.size >= MAX_LOG_LINES) {
                recentLogs.removeFirst()
            }
            recentLogs.addLast(line)
        }
    }

    private fun snapshotRecentLogs(): String {
        synchronized(recentLogLock) {
            return recentLogs.joinToString("\n")
        }
    }

    private fun clearRecentLogs() {
        synchronized(recentLogLock) {
            recentLogs.clear()
        }
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                "NekoSpeed 本地服务",
                NotificationManager.IMPORTANCE_LOW
            ).apply {
                description = "保持 NekoSpeed 测速服务在后台稳定运行"
                setShowBadge(false)
            }
            val manager = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
            manager.createNotificationChannel(channel)
        }
    }

    private fun promoteToForeground(contentText: String) {
        val notification = buildNotification(contentText)
        val fgsType = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC
        } else {
            0
        }
        ServiceCompat.startForeground(this, NOTIFICATION_ID, notification, fgsType)
    }

    private fun currentNotificationText(): String {
        return when {
            _isRunningTest.value -> "正在后台执行节点测速..."
            _serverState.value is ServerState.Ready -> "本地测速服务运行中"
            else -> "正在启动 NekoSpeed 本地服务..."
        }
    }

    private fun refreshNotification() {
        val manager = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        manager.notify(NOTIFICATION_ID, buildNotification(currentNotificationText()))
    }

    private fun buildNotification(contentText: String): Notification {
        val openIntent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
        }
        val openPendingIntent = PendingIntent.getActivity(
            this,
            0,
            openIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        val stopIntent = Intent(this, NekoSpeedServerService::class.java).apply {
            action = ACTION_STOP
        }
        val stopPendingIntent = PendingIntent.getService(
            this,
            1,
            stopIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_launcher_foreground)
            .setContentTitle("NekoSpeed")
            .setContentText(contentText)
            .setOngoing(true)
            .setContentIntent(openPendingIntent)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .setOnlyAlertOnce(true)
            .addAction(0, "停止服务", stopPendingIntent)
            .build()
    }

    companion object {
        const val ACTION_START = "com.nekospeed.android.action.START_SERVER"
        const val ACTION_RESTART = "com.nekospeed.android.action.RESTART_SERVER"
        const val ACTION_STOP = "com.nekospeed.android.action.STOP_SERVER"

        private const val CHANNEL_ID = "nekospeed_server_channel"
        private const val NOTIFICATION_ID = 1001
        private const val FONT_ASSET = "fonts/NotoSansCJKsc-Regular.otf"
        private const val MAX_LOG_LINES = 30
        private const val STARTUP_TIMEOUT_MS = 15_000L

        private val _serverState = MutableStateFlow<ServerState>(ServerState.Idle)
        val serverState: StateFlow<ServerState> = _serverState.asStateFlow()

        private val _isRunningTest = MutableStateFlow(false)
        val isRunningTest: StateFlow<Boolean> = _isRunningTest.asStateFlow()

        @Volatile
        private var activeInstance: NekoSpeedServerService? = null

        fun start(context: Context) {
            val intent = Intent(context, NekoSpeedServerService::class.java).apply {
                action = ACTION_START
            }
            ContextCompat.startForegroundService(context, intent)
        }

        fun restart(context: Context) {
            val intent = Intent(context, NekoSpeedServerService::class.java).apply {
                action = ACTION_RESTART
            }
            ContextCompat.startForegroundService(context, intent)
        }

        fun stop(context: Context) {
            runCatching {
                val intent = Intent(context, NekoSpeedServerService::class.java).apply {
                    action = ACTION_STOP
                }
                context.startService(intent)
            }
        }

        fun updateRunningState(running: Boolean) {
            _isRunningTest.value = running
            activeInstance?.refreshNotification()
        }
    }
}
