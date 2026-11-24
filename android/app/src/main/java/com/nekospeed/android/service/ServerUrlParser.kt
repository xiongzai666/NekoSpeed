package com.nekospeed.android.service

import java.io.File
import java.io.IOException
import java.util.Locale

data class ReadyEndpoint(
    val baseUrl: String,
    val port: Int
)

object ServerUrlParser {

    private val PRIMARY_READY_REGEX = Regex("""NEKOSPEED_WEB_READY=http://127\.0\.0\.1:(\d+)""")
    private val FALLBACK_URL_REGEX = Regex("""http://127\.0\.0\.1:(\d+)""")

    fun parseReadyUrl(line: String): ReadyEndpoint? {
        val match = PRIMARY_READY_REGEX.find(line) ?: FALLBACK_URL_REGEX.find(line) ?: return null
        val port = match.groupValues.getOrNull(1)?.toIntOrNull() ?: return null
        if (port !in 1..65535) return null
        return ReadyEndpoint(
            baseUrl = "http://127.0.0.1:$port",
            port = port
        )
    }

    fun isSafeSegment(name: String): Boolean {
        if (name.isEmpty()) return false
        if (name.startsWith(".")) return false
        if (name.contains('/') || name.contains('\\') || name.contains('\u0000')) return false
        if (name.any { it.code < 0x20 }) return false
        if (name.contains(':')) return false
        return true
    }

    fun resolveSafeWebFile(baseDir: File, runId: String, filename: String): File? {
        if (!isSafeSegment(runId) || !isSafeSegment(filename)) return null
        return try {
            val webRoot = File(File(baseDir, "out"), "web").canonicalFile
            val runDir = File(webRoot, runId).canonicalFile
            if (runDir.parentFile != webRoot) return null
            val target = File(runDir, filename).canonicalFile
            if (target.parentFile != runDir) return null
            target
        } catch (_: IOException) {
            null
        } catch (_: SecurityException) {
            null
        }
    }

    fun resolveSafeExportFile(baseDir: File, rawFilename: String): File? {
        val leaf = rawFilename.substringAfterLast('/').substringAfterLast('\\').trim()
        val sanitized = buildString {
            for (ch in leaf) {
                val ok = (ch in 'a'..'z') ||
                    (ch in 'A'..'Z') ||
                    (ch in '0'..'9') ||
                    ch == '.' ||
                    ch == '-' ||
                    ch == '_' ||
                    ch.code > 127
                append(if (ok) ch else '_')
            }
        }.trimStart('.')
        val withExt = when {
            sanitized.isEmpty() -> "nekospeed_export.csv"
            sanitized.lowercase(Locale.ROOT).endsWith(".csv") -> sanitized
            else -> "$sanitized.csv"
        }
        if (!isSafeSegment(withExt)) return null
        return try {
            val exportRoot = File(File(baseDir, "out"), "exports").canonicalFile
            val target = File(exportRoot, withExt).canonicalFile
            if (target.parentFile != exportRoot) return null
            target
        } catch (_: IOException) {
            null
        } catch (_: SecurityException) {
            null
        }
    }
}
