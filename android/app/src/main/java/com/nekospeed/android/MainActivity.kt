package com.nekospeed.android

import android.Manifest
import android.annotation.SuppressLint
import android.content.ActivityNotFoundException
import android.content.Intent
import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import android.content.res.Configuration
import android.graphics.Bitmap
import android.graphics.Color
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.util.TypedValue
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.webkit.CookieManager
import android.webkit.RenderProcessGoneDetail
import android.webkit.ValueCallback
import android.webkit.WebChromeClient
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.Button
import android.widget.FrameLayout
import android.widget.LinearLayout
import android.widget.ProgressBar
import android.widget.ScrollView
import android.widget.TextView
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.OnBackPressedCallback
import androidx.activity.SystemBarStyle
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.content.ContextCompat
import androidx.core.view.ViewCompat
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import com.nekospeed.android.bridge.NekoWebBridge
import com.nekospeed.android.service.NekoSpeedServerService
import com.nekospeed.android.service.NekoSpeedServerService.ServerState
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import java.util.Locale

class MainActivity : ComponentActivity() {

    private val activityScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)

    private lateinit var rootLayout: FrameLayout
    private lateinit var webView: WebView
    private lateinit var overlayLayout: LinearLayout
    private lateinit var progressBar: ProgressBar
    private lateinit var statusTitleView: TextView
    private lateinit var statusDetailScroll: ScrollView
    private lateinit var statusDetailView: TextView
    private lateinit var retryButton: Button

    private var filePathCallback: ValueCallback<Array<Uri>>? = null
    private var readyPort: Int = 0
    private var loadedPort: Int = 0
    private var loadedToken: String = ""
    private var mainFrameLoadFailed: Boolean = false
    private var renderProcessDead: Boolean = false
    private var lastBackPressedMs: Long = 0L

    private val notificationPermissionLauncher = registerForActivityResult(
        ActivityResultContracts.RequestPermission()
    ) { _ ->
        // 通知权限仅影响通知栏显示，不阻塞本地服务启动
    }

    private val fileChooserLauncher = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { result ->
        val callback = filePathCallback
        filePathCallback = null
        if (callback == null) return@registerForActivityResult

        val uris: Array<Uri>? = if (result.resultCode == RESULT_OK) {
            val data = result.data
            when {
                data?.clipData != null -> {
                    val clip = data.clipData!!
                    val list = ArrayList<Uri>(clip.itemCount)
                    for (index in 0 until clip.itemCount) {
                        clip.getItemAt(index)?.uri?.let { list.add(it) }
                    }
                    list.takeIf { it.isNotEmpty() }?.toTypedArray()
                }
                data?.data != null -> arrayOf(data.data!!)
                else -> null
            }
        } else {
            null
        }
        callback.onReceiveValue(uris)
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge(
            statusBarStyle = SystemBarStyle.auto(Color.TRANSPARENT, Color.TRANSPARENT),
            navigationBarStyle = SystemBarStyle.auto(Color.TRANSPARENT, Color.TRANSPARENT)
        )
        super.onCreate(savedInstanceState)

        buildUi()
        setContentView(rootLayout)
        applySystemUiTheme(isSystemDarkMode())

        ViewCompat.setOnApplyWindowInsetsListener(rootLayout) { view, windowInsets ->
            val insets = windowInsets.getInsets(
                WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.ime()
            )
            view.setPadding(insets.left, insets.top, insets.right, insets.bottom)
            WindowInsetsCompat.CONSUMED
        }

        requestNotificationPermissionIfNeeded()
        registerBackHandler()
        observeServerState()

        val initialState = NekoSpeedServerService.serverState.value
        if (initialState is ServerState.Idle || initialState is ServerState.Error) {
            NekoSpeedServerService.start(this)
        }
    }

    override fun onDestroy() {
        activityScope.cancel()
        filePathCallback?.onReceiveValue(null)
        filePathCallback = null
        rootLayout.removeView(webView)
        webView.stopLoading()
        webView.removeJavascriptInterface("NekoAndroid")
        webView.destroy()
        super.onDestroy()
    }

    private fun buildUi() {
        rootLayout = FrameLayout(this).apply {
            layoutParams = ViewGroup.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.MATCH_PARENT
            )
        }

        webView = WebView(this).apply {
            layoutParams = FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.MATCH_PARENT
            )
            visibility = View.INVISIBLE
        }
        configureWebView(webView)

        val bridge = NekoWebBridge(
            activity = this,
            onThemeChangedCallback = { isDark ->
                applySystemUiTheme(isDark)
            },
            onRunningStateChangedCallback = { running ->
                NekoSpeedServerService.updateRunningState(running)
            }
        )
        webView.addJavascriptInterface(bridge, "NekoAndroid")

        overlayLayout = LinearLayout(this).apply {
            layoutParams = FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT,
                FrameLayout.LayoutParams.MATCH_PARENT
            )
            orientation = LinearLayout.VERTICAL
            gravity = Gravity.CENTER
            val pad = dp(24)
            setPadding(pad, pad, pad, pad)
        }

        progressBar = ProgressBar(this).apply {
            layoutParams = LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.WRAP_CONTENT,
                LinearLayout.LayoutParams.WRAP_CONTENT
            ).apply {
                bottomMargin = dp(16)
            }
        }

        statusTitleView = TextView(this).apply {
            layoutParams = LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                LinearLayout.LayoutParams.WRAP_CONTENT
            ).apply {
                bottomMargin = dp(10)
            }
            gravity = Gravity.CENTER
            setTextSize(TypedValue.COMPLEX_UNIT_SP, 17f)
            text = "正在启动 NekoSpeed 本地服务..."
        }

        statusDetailView = TextView(this).apply {
            layoutParams = ViewGroup.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT
            )
            setTextSize(TypedValue.COMPLEX_UNIT_SP, 12f)
            setTextIsSelectable(true)
        }

        statusDetailScroll = ScrollView(this).apply {
            layoutParams = LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                dp(220)
            ).apply {
                bottomMargin = dp(16)
            }
            visibility = View.GONE
            addView(statusDetailView)
        }

        retryButton = Button(this).apply {
            layoutParams = LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.WRAP_CONTENT,
                LinearLayout.LayoutParams.WRAP_CONTENT
            )
            text = "重试启动"
            visibility = View.GONE
            setOnClickListener {
                if (renderProcessDead) {
                    recreate()
                    return@setOnClickListener
                }
                showLoadingOverlay("正在重新启动 NekoSpeed 本地服务...")
                NekoSpeedServerService.restart(this@MainActivity)
            }
        }

        overlayLayout.addView(progressBar)
        overlayLayout.addView(statusTitleView)
        overlayLayout.addView(statusDetailScroll)
        overlayLayout.addView(retryButton)

        rootLayout.addView(webView)
        rootLayout.addView(overlayLayout)
    }

    @SuppressLint("SetJavaScriptEnabled")
    private fun configureWebView(view: WebView) {
        if ((applicationInfo.flags and ApplicationInfo.FLAG_DEBUGGABLE) != 0) {
            WebView.setWebContentsDebuggingEnabled(true)
        }

        view.settings.apply {
            javaScriptEnabled = true
            domStorageEnabled = true
            allowFileAccess = false
            allowContentAccess = false
            mixedContentMode = WebSettings.MIXED_CONTENT_NEVER_ALLOW
            setSupportZoom(false)
            builtInZoomControls = false
            displayZoomControls = false
            useWideViewPort = true
            loadWithOverviewMode = true
        }

        CookieManager.getInstance().apply {
            setAcceptCookie(true)
            setAcceptThirdPartyCookies(view, false)
        }

        view.webViewClient = object : WebViewClient() {
            override fun shouldOverrideUrlLoading(
                view: WebView?,
                request: WebResourceRequest?
            ): Boolean {
                val uri = request?.url ?: return true
                val scheme = uri.scheme?.lowercase(Locale.ROOT)
                val host = uri.host
                val port = uri.port
                val expectedPort = readyPort
                if (expectedPort > 0 && scheme == "http" && host == "127.0.0.1" && port == expectedPort) {
                    return false
                }
                if (scheme == "http" || scheme == "https") {
                    runCatching {
                        val intent = Intent(Intent.ACTION_VIEW, uri).apply {
                            addCategory(Intent.CATEGORY_BROWSABLE)
                        }
                        startActivity(intent)
                    }
                }
                return true
            }

            override fun onPageStarted(view: WebView?, url: String?, favicon: Bitmap?) {
                super.onPageStarted(view, url, favicon)
                mainFrameLoadFailed = false
            }

            override fun onPageFinished(view: WebView?, url: String?) {
                super.onPageFinished(view, url)
                val port = readyPort
                if (!mainFrameLoadFailed && port > 0 && url?.startsWith("http://127.0.0.1:$port") == true) {
                    showWebContent()
                }
            }

            override fun onReceivedError(
                view: WebView?,
                request: WebResourceRequest?,
                error: WebResourceError?
            ) {
                super.onReceivedError(view, request, error)
                if (request?.isForMainFrame == true) {
                    mainFrameLoadFailed = true
                    val desc = error?.description?.toString()?.takeIf { it.isNotBlank() } ?: "页面加载失败"
                    showErrorOverlay("本地页面加载失败：$desc")
                }
            }

            override fun onRenderProcessGone(
                view: WebView?,
                detail: RenderProcessGoneDetail?
            ): Boolean {
                renderProcessDead = true
                mainFrameLoadFailed = true
                loadedPort = 0
                loadedToken = ""
                showErrorOverlay("WebView 渲染进程已退出，请点击重试启动")
                return true
            }
        }

        view.webChromeClient = object : WebChromeClient() {
            override fun onShowFileChooser(
                webView: WebView?,
                filePathCallback: ValueCallback<Array<Uri>>?,
                fileChooserParams: FileChooserParams?
            ): Boolean {
                if (filePathCallback == null) return false
                this@MainActivity.filePathCallback?.onReceiveValue(null)
                this@MainActivity.filePathCallback = filePathCallback

                val intent = Intent(Intent.ACTION_OPEN_DOCUMENT).apply {
                    addCategory(Intent.CATEGORY_OPENABLE)
                    type = "*/*"
                    if (fileChooserParams?.mode == FileChooserParams.MODE_OPEN_MULTIPLE) {
                        putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true)
                    }
                }
                return try {
                    fileChooserLauncher.launch(intent)
                    true
                } catch (_: ActivityNotFoundException) {
                    this@MainActivity.filePathCallback = null
                    filePathCallback.onReceiveValue(null)
                    false
                }
            }
        }
    }

    private fun observeServerState() {
        activityScope.launch {
            NekoSpeedServerService.serverState.collect { state ->
                when (state) {
                    is ServerState.Idle -> {
                        readyPort = 0
                        loadedPort = 0
                        loadedToken = ""
                        showLoadingOverlay("正在初始化 NekoSpeed 服务...")
                    }
                    is ServerState.Starting -> {
                        readyPort = 0
                        loadedPort = 0
                        loadedToken = ""
                        showLoadingOverlay("正在启动 NekoSpeed 本地服务...")
                    }
                    is ServerState.Ready -> {
                        readyPort = state.port
                        if (loadedPort != state.port || loadedToken != state.token) {
                            loadedPort = state.port
                            loadedToken = state.token
                            mainFrameLoadFailed = false
                            webView.loadUrl("${state.baseUrl}/?token=${state.token}")
                        } else if (!mainFrameLoadFailed) {
                            showWebContent()
                        }
                    }
                    is ServerState.Error -> {
                        readyPort = 0
                        loadedPort = 0
                        loadedToken = ""
                        showErrorOverlay(state.message)
                    }
                }
            }
        }
    }

    private fun registerBackHandler() {
        onBackPressedDispatcher.addCallback(this, object : OnBackPressedCallback(true) {
            override fun handleOnBackPressed() {
                if (webView.visibility != View.VISIBLE || readyPort <= 0) {
                    handleUnconsumedBack()
                    return
                }
                webView.evaluateJavascript(
                    "(function(){try{return !!(window.__nekoHandleBack&&window.__nekoHandleBack());}catch(e){return false;}})()"
                ) { rawResult ->
                    if (rawResult?.trim() != "true") {
                        handleUnconsumedBack()
                    }
                }
            }
        })
    }

    private fun handleUnconsumedBack() {
        if (webView.visibility == View.VISIBLE && webView.canGoBack()) {
            webView.goBack()
            return
        }
        if (NekoSpeedServerService.isRunningTest.value) {
            Toast.makeText(
                this,
                "测速正在后台运行，可通过通知栏返回或停止服务",
                Toast.LENGTH_SHORT
            ).show()
            moveTaskToBack(true)
            return
        }
        val now = System.currentTimeMillis()
        if (now - lastBackPressedMs <= BACK_EXIT_INTERVAL_MS) {
            finish()
        } else {
            lastBackPressedMs = now
            Toast.makeText(this, "再按一次返回键退出", Toast.LENGTH_SHORT).show()
        }
    }

    private fun requestNotificationPermissionIfNeeded() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            val granted = ContextCompat.checkSelfPermission(
                this,
                Manifest.permission.POST_NOTIFICATIONS
            ) == PackageManager.PERMISSION_GRANTED
            if (!granted) {
                notificationPermissionLauncher.launch(Manifest.permission.POST_NOTIFICATIONS)
            }
        }
    }

    private fun showLoadingOverlay(title: String) {
        webView.visibility = View.INVISIBLE
        overlayLayout.visibility = View.VISIBLE
        progressBar.visibility = View.VISIBLE
        statusTitleView.text = title
        statusDetailScroll.visibility = View.GONE
        statusDetailView.text = ""
        retryButton.visibility = View.GONE
    }

    private fun showErrorOverlay(message: String) {
        webView.visibility = View.INVISIBLE
        overlayLayout.visibility = View.VISIBLE
        progressBar.visibility = View.GONE
        statusTitleView.text = "本地服务启动失败"
        statusDetailView.text = message
        statusDetailScroll.visibility = View.VISIBLE
        retryButton.visibility = View.VISIBLE
    }

    private fun showWebContent() {
        overlayLayout.visibility = View.GONE
        webView.visibility = View.VISIBLE
    }

    private fun applySystemUiTheme(isDark: Boolean) {
        val bgColor = if (isDark) COLOR_BG_DARK else COLOR_BG_LIGHT
        val titleColor = if (isDark) COLOR_TEXT_DARK else COLOR_TEXT_LIGHT
        val detailColor = if (isDark) COLOR_MUTED_DARK else COLOR_MUTED_LIGHT
        rootLayout.setBackgroundColor(bgColor)
        overlayLayout.setBackgroundColor(bgColor)
        webView.setBackgroundColor(bgColor)
        statusTitleView.setTextColor(titleColor)
        statusDetailView.setTextColor(detailColor)
        WindowCompat.getInsetsController(window, window.decorView).apply {
            isAppearanceLightStatusBars = !isDark
            isAppearanceLightNavigationBars = !isDark
        }
    }

    private fun isSystemDarkMode(): Boolean {
        val nightMask = resources.configuration.uiMode and Configuration.UI_MODE_NIGHT_MASK
        return nightMask == Configuration.UI_MODE_NIGHT_YES
    }

    private fun dp(value: Int): Int {
        return TypedValue.applyDimension(
            TypedValue.COMPLEX_UNIT_DIP,
            value.toFloat(),
            resources.displayMetrics
        ).toInt()
    }

    companion object {
        private const val BACK_EXIT_INTERVAL_MS = 2000L
        private const val COLOR_BG_LIGHT = 0xFFF6F7F9.toInt()
        private const val COLOR_BG_DARK = 0xFF0C1017.toInt()
        private const val COLOR_TEXT_LIGHT = 0xFF0F172A.toInt()
        private const val COLOR_TEXT_DARK = 0xFFF1F5F9.toInt()
        private const val COLOR_MUTED_LIGHT = 0xFF526071.toInt()
        private const val COLOR_MUTED_DARK = 0xFF94A3B8.toInt()
    }
}
