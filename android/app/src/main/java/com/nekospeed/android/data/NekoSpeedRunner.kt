package com.nekospeed.android.data

import android.content.Context
import android.os.Build
import androidx.core.content.pm.PackageInfoCompat
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.BufferedReader
import java.io.File
import java.io.IOException
import java.io.InputStreamReader
import java.util.zip.GZIPInputStream

class NekoSpeedRunner(private val context: Context) {

    private val binary: File
        get() = File(context.filesDir, "nekospeed")

    private val binaryStamp: File
        get() = File(context.filesDir, ".nekospeed_version")

    private val font: File
        get() = File(context.filesDir, "NotoSansCJKsc-Regular.otf")

    private val fontStamp: File
        get() = File(context.filesDir, ".font_version")

    private val fontAsset = "fonts/NotoSansCJKsc-Regular.otf"

    private val compressedAssets: List<String>
        get() = if (Build.SUPPORTED_ABIS.any { it.startsWith("arm64") }) {
            listOf("nekospeed-android-arm64.gz", "nekospeed-android-arm64")
        } else {
            listOf("nekospeed-android-amd64.gz", "nekospeed-android-amd64")
        }

    @Volatile
    private var currentProcess: Process? = null

    private fun appVersionStamp(): String {
        return try {
            @Suppress("DEPRECATION")
            val info = context.packageManager.getPackageInfo(context.packageName, 0)
            "${PackageInfoCompat.getLongVersionCode(info)}-${info.lastUpdateTime}"
        } catch (_: Exception) {
            "0"
        }
    }

    private fun isUpToDate(target: File, stampFile: File, expectedStamp: String): Boolean {
        if (!target.exists() || target.length() == 0L || !stampFile.exists()) return false
        return try {
            stampFile.readText().trim() == expectedStamp
        } catch (_: Exception) {
            false
        }
    }

    private fun extractAssetAtomically(
        assetName: String,
        target: File,
        stampFile: File,
        stamp: String,
        executable: Boolean
    ) {
        val tmp = File(target.parentFile, "${target.name}.tmp")
        tmp.delete()
        context.assets.open(assetName).use { input ->
            val stream = if (assetName.endsWith(".gz")) GZIPInputStream(input) else input
            tmp.outputStream().use { output ->
                stream.copyTo(output)
            }
        }
        if (executable) {
            tmp.setExecutable(true)
        }
        if (target.exists() && !target.delete()) {
            tmp.delete()
            throw IOException("无法覆盖旧文件: ${target.name}")
        }
        if (!tmp.renameTo(target)) {
            tmp.delete()
            throw IOException("无法落盘文件: ${target.name}")
        }
        if (executable) {
            target.setExecutable(true)
        }
        stampFile.writeText(stamp)
    }

    suspend fun ensureBinary() = withContext(Dispatchers.IO) {
        val stamp = appVersionStamp()
        if (isUpToDate(binary, binaryStamp, stamp)) {
            binary.setExecutable(true)
            return@withContext
        }

        val assetName = compressedAssets.firstOrNull { assetExists(context, it) }
            ?: throw IllegalStateException("未找到 assets 中的二进制文件")

        extractAssetAtomically(assetName, binary, binaryStamp, stamp, executable = true)
    }

    /** 解压内置中文字体（厂商 ROM 系统字体解析失败时经 -font 兜底） */
    suspend fun ensureFont() = withContext(Dispatchers.IO) {
        val stamp = appVersionStamp()
        if (isUpToDate(font, fontStamp, stamp)) return@withContext
        if (!assetExists(context, fontAsset)) return@withContext
        extractAssetAtomically(fontAsset, font, fontStamp, stamp, executable = false)
    }

    private fun assetExists(context: Context, name: String): Boolean {
        return try {
            context.assets.open(name).close()
            true
        } catch (_: Exception) {
            false
        }
    }

    suspend fun run(
        sub: String,
        tests: List<String>,
        threads: Int = 8,
        duration: Int = 10,
        concurrency: Int = 1,
        pingCount: Int = 4,
        filter: String? = null,
        exclude: String? = null,
        sortBy: String? = null,
        maxMB: Int = 0,
        preflight: Boolean = true,
        onLog: (String) -> Unit = {}
    ): Result = withContext(Dispatchers.IO) {
        val outDir = File(context.filesDir, "out")
        outDir.deleteRecursively()
        outDir.mkdirs()

        val args = mutableListOf(
            binary.absolutePath,
            "-sub", sub,
            "-tests", tests.joinToString(","),
            "-o", outDir.absolutePath,
            "-threads", threads.toString(),
            "-duration", duration.toString(),
            "-concurrency", concurrency.toString(),
            "-ping-count", pingCount.toString()
        )
        if (font.exists()) {
            args.addAll(listOf("-font", font.absolutePath))
        }
        if (!filter.isNullOrBlank()) {
            args.addAll(listOf("-filter", filter))
        }
        if (!exclude.isNullOrBlank()) {
            args.addAll(listOf("-exclude", exclude))
        }
        if (!sortBy.isNullOrBlank() && sortBy != "none") {
            args.addAll(listOf("-sort", sortBy))
        }
        if (maxMB > 0) {
            args.addAll(listOf("-max-mb", maxMB.toString()))
        }
        if (!preflight) {
            args.add("-preflight=false")
        }

        val process = try {
            startProcess(outDir, args)
        } catch (e: IOException) {
            startProcessViaLinker(outDir, args)
        }
        currentProcess = process

        try {
            val reader = BufferedReader(InputStreamReader(process.inputStream))
            var line: String?
            while (reader.readLine().also { line = it } != null) {
                onLog(line!!)
            }
            val exitCode = process.waitFor()
            val resultFile = outDir.listFiles { f ->
                f.name.endsWith("_result.json")
            }?.maxByOrNull { it.lastModified() }

            if (exitCode != 0 || resultFile == null) {
                Result.Error("进程退出码=$exitCode，结果文件不存在")
            } else {
                Result.Success(resultFile)
            }
        } catch (e: CancellationException) {
            process.destroyForcibly()
            throw e
        } catch (e: Exception) {
            process.destroyForcibly()
            Result.Error(e.message ?: "未知错误")
        } finally {
            currentProcess = null
        }
    }

    private fun startProcess(outDir: File, args: List<String>): Process =
        ProcessBuilder(args)
            .redirectErrorStream(true)
            .directory(outDir)
            .start()

    /** Android 14+ SELinux 拒绝直接 execve app_data_file，回退经动态链接器执行 */
    private fun startProcessViaLinker(outDir: File, args: List<String>): Process =
        ProcessBuilder(listOf("/system/bin/linker64") + args)
            .redirectErrorStream(true)
            .directory(outDir)
            .start()

    fun cancel() {
        currentProcess?.destroyForcibly()
    }

    fun getOutDir(): File = File(context.filesDir, "out")

    fun getPngFiles(): List<File> = getOutDir().listFiles { f ->
        f.name.endsWith(".png")
    }?.sorted() ?: emptyList()

    sealed class Result {
        data class Success(val file: File) : Result()
        data class Error(val message: String) : Result()
    }
}
