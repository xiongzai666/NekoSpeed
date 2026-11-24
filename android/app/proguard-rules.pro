# 保留注解（@JavascriptInterface 等运行时注解必需）
-keepattributes *Annotation*

# 保留 WebView JS Bridge 类及其带 @JavascriptInterface 的方法
-keep class com.nekospeed.android.bridge.NekoWebBridge { *; }
-keepclassmembers class com.nekospeed.android.bridge.NekoWebBridge {
    @android.webkit.JavascriptInterface <methods>;
}
-keepclassmembers class * {
    @android.webkit.JavascriptInterface <methods>;
}

# Kotlin 协程
-dontwarn kotlinx.coroutines.**
