# NekoSpeed Android 客户端开发文档

## 1. 架构定位：WebView 轻量壳端

NekoSpeed Android 客户端采用 **WebView 轻量壳端（Thin WebView Shell）** 架构，将 Go 核心测速引擎编译为原生共享库形态的可执行文件嵌入 APK，由前台服务托管内嵌 Web UI HTTP Server，再通过系统 `WebView` 加载本地回环地址。

相比早期的「`assets/*.gz` 解压 + Jetpack Compose 独立界面」方案，轻量壳端带来三项核心收益：

- **单一内核与单一界面**：Android 端直接复用 `internal/webui/static/` 前端（智能粘贴识别、测前挑节点、实时 SSE 流式看板、21 款主题免重测重绘、全宽报告画廊、滚轮/双指缩放灯箱与 GeoIP 一键更新），功能迭代零滞后。
- **绕过 Android 10+ W^X 限制**：不再把可执行文件解压到可写 `filesDir`，而是以 `libnekospeed.so` 打入 `jniLibs`，由 Android 包管理器在安装期释放至只读可执行目录 `applicationInfo.nativeLibraryDir`。
- **代码精简**：无需在 Kotlin 侧重复维护 `ResultDto` 序列化模型、Compose 表格、解锁网格或拓扑页面。

---

## 2. 整体运行架构与进程通信契约

```text
MainActivity (ComponentActivity + enableEdgeToEdge + WebView)
  │
  ├── 加载 http://127.0.0.1:<port>/?token=<random32HexToken>
  │     └─ 服务端写入 HttpOnly / SameSite=Lax Cookie (nekospeed_token)
  │
  ├── JavascriptInterface: window.NekoAndroid (NekoWebBridge)
  │     └─ VPN 冲突检测 / 原生剪贴板 / 相册保存 / 系统分享 / CSV 导出 / 主题与运行态同步
  │
  └── bindService / startForegroundService ──▶ NekoSpeedServerService (dataSync)
        │
        ├── 释放 assets/fonts/NotoSansCJKsc-Regular.otf ──▶ files/fonts/
        ├── 准备私有目录 files/out 与 files/geoip
        └── ProcessBuilder 启动 nativeLibraryDir/libnekospeed.so
              参数: -web 127.0.0.1:0 -web-token <token>
              输出: NEKOSPEED_WEB_READY=http://127.0.0.1:<port>
```

### 2.1 Go 子进程启动参数与环境变量

`NekoSpeedServerService` 在 `context.filesDir` 工作目录下启动 Go 后端：

- **可执行路径**：`File(applicationInfo.nativeLibraryDir, "libnekospeed.so")`
- **命令行参数**：`<soPath> -web 127.0.0.1:0 -web-token <random32HexToken>`（`127.0.0.1:0` 动态分配空闲端口，避免端口冲突）
- **环境变量注入**：

| 环境变量 | 注入值 | 作用说明 |
|---|---|---|
| `NEKOSPEED_OUT_DIR` | `<filesDir>/out` | 统一结果 JSON、PNG 报告与历史记录落盘目录 |
| `NEKOSPEED_GEOIP_DIR` | `<filesDir>/geoip` | 离线 GeoIP MMDB 存储与自动探测目录 |
| `NEKOSPEED_FONT_PATH` | `<filesDir>/fonts/NotoSansCJKsc-Regular.otf` | 兜底中文字体路径，避免厂商 ROM 缺字 |
| `NEKOSPEED_NO_BROWSER` | `"1"` | 禁止 Go 服务端尝试拉起桌面浏览器 |
| `NEKOSPEED_ANDROID_SHELL` | `"1"` | 触发服务端在标准输出打印就绪信号行 |

- **就绪握手**：`NekoSpeedServerService` 读取子进程 `stdout`，通过 `ServerUrlParser.parseReadyUrl` 解析 `NEKOSPEED_WEB_READY=http://127.0.0.1:<port>`（并保留 `http://127\.0\.0\.1:(\d+)` 正则兜底），随后更新状态为 `ServerState.Ready(baseUrl, port, token)`，由 `MainActivity` 加载 `http://127.0.0.1:<port>/?token=<token>`。

---

## 3. `jniLibs` W^X 机制与 16 KB ELF 页对齐

### 3.1 为什么改用 `jniLibs/arm64-v8a/libnekospeed.so`

从 Android 10（API 29）起，系统对目标版本 `targetSdk >= 29` 的应用严格执行 **W^X（Write XOR Execute）** 安全策略：禁止在应用可写数据目录（如 `filesDir`）内对下载或解压出的二进制赋予可执行权限并直接 `execve()`。

本项目将 Go CLI/WebUI 可执行程序通过 NDK 动态链接编译并命名为 `libnekospeed.so`，放置于：

- `android/app/src/main/jniLibs/arm64-v8a/libnekospeed.so`（CI 默认构建产物）
- `android/app/src/main/jniLibs/x86_64/libnekospeed.so`（本地设置 `ANDROID_ABIS="arm64 amd64"` 时可选生成，供模拟器调试）

同时在 `android/app/build.gradle.kts` 与 `AndroidManifest.xml` 中启用：

- `packaging { jniLibs { useLegacyPackaging = true } }`
- `android:extractNativeLibs="true"`
- `ndk { abiFilters }` 默认 `"arm64-v8a"`（支持 `-PabiFilters=...` 覆盖）

安装 APK 时，Android 系统会自动把 `libnekospeed.so` 解压至只读且具备执行权限的 `applicationInfo.nativeLibraryDir`，从而无需 `/system/bin/linker64` 绕行即可稳定拉起进程。

### 3.2 Android 15+ 16 KB 内存页对齐编译参数

为适配 Android 15（API 35）及后续 16 KB Page Size 设备，Go 交叉编译必须启用外部链接器（NDK `clang` / `lld`）并显式指定 `16384` 字节对齐：

```bash
CGO_ENABLED=1 \
GOOS=android \
GOARCH=arm64 \
CC="${NDK_BIN}/aarch64-linux-android26-clang" \
CGO_LDFLAGS="-Wl,-z,max-page-size=16384 -Wl,-z,common-page-size=16384" \
  go build -trimpath \
    -ldflags "-s -w -linkmode=external -extldflags '-Wl,-z,max-page-size=16384 -Wl,-z,common-page-size=16384' -X nekospeed/internal/runner.Version=${VERSION}" \
    -o android/app/src/main/jniLibs/arm64-v8a/libnekospeed.so ./cmd/nekospeed
```

- **NDK 编译器选择**：优先使用 `aarch64-linux-android26-clang`（匹配 `minSdk = 26`），缺失时回退至 `aarch64-linux-android21-clang`。
- **ELF 段对齐校验**：CI 流水线使用 `readelf -lW` 检查产物的所有 `LOAD` 段，要求对齐值均满足 `>= 0x4000`（16 KB）。

---

## 4. `NekoAndroid` JS 桥接契约与安全边界

前端与原生层通过 `@JavascriptInterface`（挂载于 `window.NekoAndroid`）以及全局返回键钩子 `window.__nekoHandleBack` 进行交互：

### 4.1 `window.NekoAndroid` 接口定义

| 方法签名 | 返回值 | 说明与行为 |
|---|---|---|
| `isVpnActive()` | `boolean` | 通过 `VpnDetector` 检测系统是否开启 VPN 传输、全局 HTTP 代理或系统代理，驱动首页 `#androidVpnBanner` 提醒横幅 |
| `copyText(text)` | `boolean` | 调用 Android `ClipboardManager` 复制订阅文本或节点信息，规避 WebView 内 `navigator.clipboard` 权限受限问题 |
| `saveImageToGallery(runId, filename)` | `string` | 返回 `"saved"`、`"shared"` 或 `"error:<msg>"`。API 29+ 通过 `MediaStore` 写入系统相册 `Pictures/NekoSpeed`；API 29 以下无需申请 `WRITE_EXTERNAL_STORAGE`，自动回退系统分享（返回 `"shared"`） |
| `shareFile(runId, filename)` | `boolean` | 通过 `FileProvider` 调起系统分享面板，支持 `.png`、`*_result.json` 与 `.csv` |
| `exportCsv(filename, content)` | `boolean` | 校验并限制文本大小上限为 `16 MiB`，写入 `files/out/exports/<safeName>` 后调用 `ShareHelper.shareFile` 分享导出 |
| `onThemeChanged(isDark)` | `void` | 前端切换浅色 / 深色主题时通知 `MainActivity` 同步系统状态栏与导航栏图标反色 |
| `onRunningStateChanged(running)` | `void` | 测速任务开始 / 结束时更新 `NekoSpeedServerService` 前台通知文案，并协同控制返回键二次确认与任务保活 |

### 4.2 前端返回键钩子与路径安全

- **`window.__nekoHandleBack(): boolean`**：用户按下系统返回键时，`MainActivity` 的 `OnBackPressedCallback` 先执行该方法，按「全屏灯箱 → 历史记录浮层 → 环境与 GeoIP 浮层 → 选节点抽屉」顺序关闭最顶层浮层；若当前无打开浮层且 WebView 可后退则回退历史；若测试正在运行则提示二次确认或退回后台保活。
- **文件访问沙箱**：`NekoWebBridge` 统一调用 `ServerUrlParser.isSafeSegment` 与 `resolveSafeWebFile`，强制校验路径规范化后位于 `<filesDir>/out/web` 之内，杜绝目录穿越。
- **WebView 安全加固**：
  - `res/xml/network_security_config.xml` 全局禁用明文流量（`cleartextTrafficPermitted="false"`），仅对 `<domain includeSubdomains="false">127.0.0.1</domain>` 放行本地回环 HTTP。
  - 关闭 `allowFileAccess`，设置 `MIXED_CONTENT_NEVER_ALLOW` 与 `CookieManager.setAcceptCookie(true)`。
  - `shouldOverrideUrlLoading` 仅允许 `http://127.0.0.1:<readyPort>` 在 WebView 内部打开，其余 `http/https` 外链一律交由系统浏览器（`Intent.ACTION_VIEW`）。
  - `onShowFileChooser` 绑定 `ActivityResultContracts` + `ACTION_OPEN_DOCUMENT`，支持在本地文件模式中直接挑选 `.yaml` / `.txt` 订阅文件上传。

---

## 5. Android 15 适配（`onTimeout`、Edge-to-Edge 与移动端调优）

1. **Android 15 `dataSync` 前台服务 `onTimeout`**：
   - Android 15（API 35）为 `dataSync` 类型前台服务设置了单日运行时长配额。`NekoSpeedServerService` 重写 `onTimeout(startId: Int, fgsType: Int)`，在收到系统超时通知时主动终止 Go 子进程、刷新 `ServerState` 并调用 `stopSelf()`，避免抛出 `ForegroundServiceDidNotStopInTimeException` 崩溃。
   - `onTaskRemoved` 根据当前是否处于运行态分别处理：仅在空闲时停止服务释放内存；测试进行中划掉最近任务卡片仍保持前台服务运行。
2. **Edge-to-Edge 全面屏与输入法避让**：
   - `MainActivity` 启用 `enableEdgeToEdge()`（`compileSdk = 35`、`targetSdk = 35`、`minSdk = 26`），在根布局动态监听并应用 `WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.ime()` 内边距。
   - 前端页面配合 `viewport-fit=cover` 与移动端 `max-width: 680px` 响应式样式（浮层固定贴合视口、触控目标 `>= 36px`、灯箱支持双指捏合缩放）。
3. **默认并发与权限收口**：
   - Go 服务端（`runtime.GOOS == "android"`）与前端初始化逻辑协同将 Android 默认节点并发探测数设为 `4`（桌面端为 `6`），在移动设备网络栈与发热控制之间取得平衡。
   - API 33+ 动态申请 `POST_NOTIFICATIONS` 用于前台服务通知展示；保存图片使用分区存储 `MediaStore`，无需读写外部存储权限。

---

## 6. 项目结构

```text
NekoSpeed/
├── android/
│   ├── build.gradle.kts                         # AGP 8.13.2 + Kotlin 2.1.21
│   ├── gradle/wrapper/gradle-wrapper.properties # Gradle 8.13
│   └── app/
│       ├── build.gradle.kts                     # compileSdk/targetSdk 35, minSdk 26, jniLibs & 签名回退
│       ├── proguard-rules.pro                   # 保留 @JavascriptInterface 与 NekoWebBridge
│       └── src/
│           ├── main/
│           │   ├── AndroidManifest.xml          # networkSecurityConfig / extractNativeLibs / dataSync 服务
│           │   ├── assets/fonts/                # NotoSansCJKsc-Regular.otf（构建时下载，不入库）
│           │   ├── jniLibs/arm64-v8a/           # libnekospeed.so（构建时生成，不入库）
│           │   ├── res/xml/
│           │   │   ├── file_paths.xml           # FileProvider 分享路径（files/out/ 等）
│           │   │   └── network_security_config.xml
│           │   └── java/com/nekospeed/android/
│           │       ├── MainActivity.kt          # Edge-to-Edge、WebView 安全配置、文件选择与返回键拦截
│           │       ├── NekoSpeedApp.kt          # 通知渠道初始化
│           │       ├── bridge/
│           │       │   └── NekoWebBridge.kt     # window.NekoAndroid JS 桥接实现
│           │       ├── service/
│           │       │   ├── NekoSpeedServerService.kt # 前台服务、字体释放、Go 进程托管与 onTimeout
│           │       │   └── ServerUrlParser.kt   # NEKOSPEED_WEB_READY 解析与路径穿越防护
│           │       └── util/
│           │           ├── ShareHelper.kt       # MediaStore 相册保存（Q+）与 FileProvider 分享
│           │           └── VpnDetector.kt       # VPN 传输 / 全局 HTTP 代理检测
│           └── test/java/com/nekospeed/android/
│               └── AndroidShellLogicTest.kt     # ServerUrlParser 与安全路径 JVM 单元测试
└── scripts/
    └── build.sh                                 # 桌面 6 平台 + Android jniLibs 交叉编译与字体下载
```

---

## 7. 本地构建与测试

### 7.1 准备依赖

- Go 1.25+
- Android SDK 与 NDK（支持通过 `ANDROID_NDK_LATEST_HOME`、`ANDROID_NDK_HOME`、`ANDROID_NDK` 或 `$ANDROID_HOME/ndk/*` 自动发现）
- JDK 17

### 7.2 编译原生库与 APK

1. 在仓库根目录运行构建脚本，下载 `NotoSansCJKsc-Regular.otf` 并交叉编译 `libnekospeed.so`：

```bash
# 默认仅构建 arm64-v8a（真机）
./scripts/build.sh

# 如需同时包含 x86_64（模拟器）
ANDROID_ABIS="arm64 amd64" ./scripts/build.sh
```

2. 进入 `android/` 运行 JVM 单元测试并打包 APK：

```bash
cd android
chmod +x gradlew
./gradlew testReleaseUnitTest assembleRelease -PversionName="0.3.0" -PversionCode="1" --no-daemon
```

> 若本地未配置 `android/keystore.properties` 且未导出 `ANDROID_KEYSTORE_*` 环境变量，`buildTypes.release` 会自动回退到 Gradle `debug` 签名，生成的 `android/app/build/outputs/apk/release/app-release.apk` 可直接安装测试；如需同时打入 `x86_64`，可追加 `-PabiFilters=arm64-v8a,x86_64`。

---

## 8. CI 自动构建 APK 与可选正式签名配置

### 8.1 流水线产物与校验

以下两个 GitHub Actions 工作流均已集成 Android `arm64-v8a` APK 自动构建（基于 Node 24 原生 Action：`actions/checkout@v7`、`actions/setup-go@v7`、`actions/setup-java@v6`、`gradle/actions/setup-gradle@v6`、`actions/upload-artifact@v7`、`actions/download-artifact@v8`、`softprops/action-gh-release@v3`）：

- **[CI & Tag Release](../.github/workflows/ci-release.yml)**：
  - `build-android` 作业在每次 `main` / `master` / `feat/**` / `fork/**` 分支推送、PR 以及 `v*` 标签推送时均会并行运行。
  - 编译 `libnekospeed.so` 后使用 `readelf -lW` 校验所有 `LOAD` 段对齐 `>= 0x4000`（16 KB），下载并校验 `NotoSansCJKsc-Regular.otf`（`> 5 MB`），执行 `./gradlew testReleaseUnitTest assembleRelease` 与 `apksigner verify --print-certs`。
  - 在 `v*` 标签推送时，`release` 作业自动下载 `desktop-release` 与 `android-release` 产物，生成统一 `SHA256SUMS.txt` 并随 Release 一并发布 **`nekospeed_<tag>_android-arm64.apk`**。
- **[Sync Upstream & Mihomo Auto Release](../.github/workflows/sync-mihomo.yml)**：
  - 上游同步或 `mihomo` 版本升级（或手动 `force_release`）触发发布时，在完成桌面 6 平台构建后同样执行 `Build Android APK (arm64-v8a)`，将 `nekospeed_<tag>_android-arm64.apk` 并入 `dist/release/` 统一计算 `SHA256SUMS.txt` 后发布。

### 8.2 可选正式签名 Secrets 配置步骤

默认情况下（未配置 Secrets），CI 会输出 `::notice::` 提示并回退 Gradle `debug` 签名，不影响测试与直接安装。若希望对外发布的 Release APK 使用固定正式签名证书（便于覆盖升级安装），请按以下步骤配置：

1. **生成签名密钥库（切勿提交至 Git）**：

```bash
keytool -genkeypair -v -keystore release.keystore -alias nekospeed \
  -keyalg RSA -keysize 2048 -validity 10000
```

2. **将密钥库编码为单行 Base64**：

```bash
# Linux / macOS
base64 -w 0 release.keystore   # macOS 使用 base64 -i release.keystore | tr -d '\n'

# Windows PowerShell
[Convert]::ToBase64String([IO.File]::ReadAllBytes("release.keystore"))
```

3. **在 GitHub 仓库配置 4 个 Actions Secrets**（`Settings -> Secrets and variables -> Actions -> New repository secret`）：

| Secret 名称 | 内容说明 | CI 注入环境变量 |
|---|---|---|
| `ANDROID_KEYSTORE_BASE64` | `release.keystore` 的 Base64 编码字符串 | 解码写入 `$RUNNER_TEMP/release.keystore` 并设为 `ANDROID_KEYSTORE_PATH` |
| `ANDROID_KEYSTORE_PASSWORD` | Keystore 仓库口令 | `ANDROID_KEYSTORE_PASSWORD` |
| `ANDROID_KEY_ALIAS` | 密钥别名（如 `nekospeed`） | `ANDROID_KEY_ALIAS` |
| `ANDROID_KEY_PASSWORD` | 密钥私钥口令 | `ANDROID_KEY_PASSWORD` |

> **注意**：仅当上述 4 个 Secrets 全部非空时，CI 才会启用正式签名；`*.jks`、`*.keystore` 与 `android/keystore.properties` 均已加入 `.gitignore`，请妥善保管离线备份。

---

## 9. 开源协议与第三方合规

- **主协议（GPL-3.0）**：因 `libnekospeed.so` 静态链接 `github.com/metacubex/mihomo`（GPL-3.0），NekoSpeed Android 客户端整体遵循 **GPL-3.0** 协议开源（见 [LICENSE](../LICENSE)），分发 APK 时须保持对应源码公开。
- **内置字体（SIL OFL 1.1）**：`NotoSansCJKsc-Regular.otf` 遵循 SIL Open Font License 1.1。
- **Android 端依赖（Apache-2.0）**：`androidx.core:core-ktx`、`androidx.activity:activity-ktx`、`kotlinx-coroutines-android` 等遵循 Apache-2.0。
- **离线 GeoIP 数据归属（CC BY-SA 4.0）**：用户通过 Web UI「更新 IP 库」下载的离线数据库包含 `GeoCN.mmdb`（[ljxi/GeoCN](https://github.com/ljxi/GeoCN)）、`ipinfo_lite.mmdb`（IP address data powered by [IPinfo](https://ipinfo.io), **CC BY-SA 4.0**）以及 `GeoLite2-City.mmdb` / `GeoLite2-ASN.mmdb`（MaxMind GeoLite2, **CC BY-SA 4.0**，镜像来自 [P3TERX/GeoLite.mmdb](https://github.com/P3TERX/GeoLite.mmdb)）。

---

> 仅供网络质量测试与学习研究使用，请勿用于违反当地法律法规的场景。
