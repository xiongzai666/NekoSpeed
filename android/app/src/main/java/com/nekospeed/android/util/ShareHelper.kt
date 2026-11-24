package com.nekospeed.android.util

import android.app.Activity
import android.content.ContentValues
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Environment
import android.provider.MediaStore
import androidx.core.content.FileProvider
import java.io.File
import java.io.IOException
import java.util.Locale

object ShareHelper {

    fun shareFile(context: Context, file: File, title: String = "分享"): Boolean {
        if (!file.exists() || !file.isFile) return false
        return runCatching {
            val uri = getUriForFile(context, file)
            val intent = Intent(Intent.ACTION_SEND).apply {
                type = getMimeType(file)
                putExtra(Intent.EXTRA_STREAM, uri)
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            }
            val chooser = Intent.createChooser(intent, title).apply {
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                if (context !is Activity) {
                    addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                }
            }
            context.startActivity(chooser)
            true
        }.getOrDefault(false)
    }

    /** 保存图片到系统相册 Pictures/NekoSpeed/（仅 API 29+），返回 MediaStore Uri；低版本返回 null */
    fun saveImageToGallery(context: Context, file: File): Uri? {
        if (!file.exists() || !file.isFile) return null
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) {
            return null
        }
        val mimeType = getMimeType(file).let { if (it.startsWith("image/")) it else "image/png" }
        val values = ContentValues().apply {
            put(MediaStore.Images.Media.DISPLAY_NAME, file.name)
            put(MediaStore.Images.Media.MIME_TYPE, mimeType)
            put(
                MediaStore.Images.Media.RELATIVE_PATH,
                Environment.DIRECTORY_PICTURES + "/NekoSpeed"
            )
            put(MediaStore.Images.Media.IS_PENDING, 1)
        }
        val resolver = context.contentResolver
        val uri = resolver.insert(MediaStore.Images.Media.EXTERNAL_CONTENT_URI, values) ?: return null
        val copied = runCatching {
            resolver.openOutputStream(uri)?.use { out ->
                file.inputStream().use { input ->
                    input.copyTo(out)
                }
            } ?: throw IOException("无法打开相册写入流")
        }.isSuccess

        if (!copied) {
            runCatching { resolver.delete(uri, null, null) }
            return null
        }

        val clearPending = ContentValues().apply {
            put(MediaStore.Images.Media.IS_PENDING, 0)
        }
        resolver.update(uri, clearPending, null, null)
        return uri
    }

    private fun getUriForFile(context: Context, file: File): Uri {
        return FileProvider.getUriForFile(
            context,
            "${context.packageName}.fileprovider",
            file
        )
    }

    internal fun getMimeType(file: File): String {
        return when (file.extension.lowercase(Locale.ROOT)) {
            "json" -> "application/json"
            "png" -> "image/png"
            "jpg", "jpeg" -> "image/jpeg"
            "webp" -> "image/webp"
            "csv" -> "text/csv"
            else -> "*/*"
        }
    }
}
