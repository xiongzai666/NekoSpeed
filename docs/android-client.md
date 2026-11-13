# NekoSpeed Android 客户端开发文档

## 1. 目标

为 NekoSpeed 提供原生 Android GUI，支持以下功能：

- 通过订阅链接或直接粘贴订阅内容导入节点
- 选择测试项（`speed` / `media` / `ai` / `topo` / `nat`）
- 在前台服务中执行测速并实时查看日志与进度，支持随时取消
- 展示节点列表、速度统计、流媒体与 AI 解锁矩阵、入口/出口拓扑以及渲染 PNG 报告图
- 自动归档历史测速记录，支持相册保存与系统分享 JSON / PNG
- 主题切换（System / Light / Dark / NekoSpeed）

## 2. 架构

```text
Android App (Kotlin + Jetpack Compose)
        │
        ├── assets/nekospeed-android-{arm64,amd64}.gz + fonts/NotoSansCJKsc-Regular.otf
        │     └─ 运行时解压释放到 files/
        │
        ├── ProcessBuilder / linker64 前台服务调用 NekoSpeed 二进制
        │     -sub <url|sub.txt> -tests <...> -o files/out -font files/NotoSansCJKsc-Regular.otf
        │
        └── 发现 *_result.json 与 *.png → 归档到 files/history/<ts>/ → 渲染 Compose UI
```

## 3. 技术栈

- **UI**：Jetpack Compose + Material3
- **架构**：MVVM + ViewModel + StateFlow
- **并发**：Kotlin Coroutines
- **进程调用**：Runtime.exec / ProcessBuilder
- **JSON 解析**：Kotlinx Serialization
- **图片显示**：Coil（展示 NekoSpeed 生成的 PNG 结果）

## 4. 为什么选嵌入二进制方案

NekoSpeed 支持通过 NDK cgo 交叉编译为 Android 可执行文件（统一使用 bionic 动态链接适配 Android 14+ SELinux）：

```bash
CGO_ENABLED=1 GOOS=android GOARCH=arm64 CC="$NDK_ARM64_CLANG" \
  go build -trimpath -ldflags "-s -w" -o nekospeed-android-arm64 ./cmd/nekospeed
gzip -f nekospeed-android-arm64
```

- gzip 压缩后约 10 MB（支持 `arm64` 与 `amd64` 模拟器）
- 无需重写测速、解锁与拓扑探测逻辑
- 与 Go 主仓共享核心内核，更新后通过 `scripts/build.sh` 重新编译替换即可

## 5. 项目结构

```text
NekoSpeed/
├── android/                                     # Android 客户端工程
│   ├── app/
│   │   ├── src/main/
│   │   │   ├── assets/
│   │   │   │   ├── nekospeed-android-arm64.gz   # 运行时按 ABI 解压
│   │   │   │   ├── nekospeed-android-amd64.gz
│   │   │   │   └── fonts/NotoSansCJKsc-Regular.otf
│   │   │   ├── java/com/nekospeed/android/
│   │   │   │   ├── MainActivity.kt
│   │   │   │   ├── NekoSpeedApp.kt
│   │   │   │   ├── data/
│   │   │   │   │   ├── NekoSpeedRunner.kt       # 二进制释放、执行、取消与结果发现
│   │   │   │   │   ├── ResultRepository.kt      # 结果读取与历史归档
│   │   │   │   │   └── model/
│   │   │   │       ├── ResultDto.kt
│   │   │   │       └── HistoryRecord.kt
│   │   │   │   ├── service/
│   │   │   │   │   ├── NekoSpeedService.kt      # 前台服务保活与取消
│   │   │   │   │   └── NekoSpeedServiceState.kt
│   │   │   │   ├── util/
│   │   │   │   │   ├── ShareHelper.kt           # FileProvider 分享与相册保存
│   │   │   │   │   └── VpnDetector.kt           # VPN / 代理冲突检测
│   │   │   │   ├── ui/
│   │   │   │   │   ├── screens/
│   │   │   │   │   │   ├── InputScreen.kt
│   │   │   │   │   │   ├── RunningScreen.kt
│   │   │   │   │   │   ├── ResultScreen.kt
│   │   │   │   │   │   └── HistoryScreen.kt
│   │   │   │   │   └── components/
│   │   │   │   │       ├── NodeList.kt
│   │   │   │   │       ├── UnlockGrid.kt
│   │   │   │       └── TopologyTable.kt
│   │   │   │   └── viewmodel/
│   │   │   │       ├── RunViewModel.kt
│   │   │   │       └── HistoryViewModel.kt
│   │   └── build.gradle.kts
│   └── build.gradle.kts
├── scripts/
│   └── build.sh                                 # 自动交叉编译并 gzip 压缩入 assets
└── docs/
    ├── android-progress.md                      # 详细开发与发布日志
    └── android-implementation-plan.md           # Phase 0-1 实施计划
```

## 6. 关键模块

### 6.1 二进制释放与执行

```kotlin
class NekoSpeedRunner(private val context: Context) {
    private val binary: File
        get() = File(context.filesDir, "nekospeed")

    suspend fun ensureBinary() = withContext(Dispatchers.IO) {
        if (!binary.exists()) {
            // 按 Build.SUPPORTED_ABIS 解压 assets/nekospeed-android-{arm64,amd64}.gz
            // 并释放内置中文字体 NotoSansCJKsc-Regular.otf
        }
    }

    suspend fun run(
        sub: String,
        tests: List<String>,
        concurrency: Int = 6
    ): File? = withContext(Dispatchers.IO) {
        val outDir = File(context.filesDir, "out").apply {
            deleteRecursively()
            mkdirs()
        }
        val process = ProcessBuilder(
            binary.absolutePath,
            "-sub", sub, // 支持本地 sub.txt，避免敏感 URL 出现在进程参数中
            "-tests", tests.joinToString(","),
            "-o", outDir.absolutePath,
            "-concurrency", concurrency.toString(),
            "-font", fontFile.absolutePath
        ).redirectErrorStream(true).start()
        process.waitFor()
        outDir.listFiles { f -> f.name.endsWith("_result.json") }?.maxByOrNull { it.lastModified() }
    }
}
```

> 以上为调用流程示意，完整实现（含 `linker64` 回退执行、`cancel()` 强杀进程与实时日志流读取）见 `android/app/src/main/java/com/nekospeed/android/data/NekoSpeedRunner.kt`。

### 6.2 数据模型

复用 Go 端输出的 `*_result.json` 结构（见 `ResultDto.kt`）：

```kotlin
@Serializable
data class Result(
    val version: String,
    val startTime: String,
    val elapsed: String,
    val total: Int,
    val succeeded: Int,
    val nodes: List<Node>
)

@Serializable
data class Node(
    val index: Int,
    val name: String,
    val type: String,
    val server: String,
    val latency: Latency,
    val media: List<Unlock>? = null,
    val ai: List<Unlock>? = null,
    val inbound: Geo? = null,
    val outbound: Geo? = null
)
```

### 6.3 UI 页面

| 页面 | 功能 |
|---|---|
| 输入页 | 订阅 URL / 粘贴订阅内容、选择测试项、高级参数、主题切换与历史入口 |
| 运行页 | 显示实时日志、进度与取消按钮（支持前台服务保活与状态恢复） |
| 结果页 | 四 Tab：节点列表 / 解锁 / 拓扑 / 图片（支持单图分享与保存到相册） |
| 历史页 | 查看已归档历史报告、重新加载详情与删除记录 |
| 节点列表 | 名称、延迟、平均速度、最高速度与状态 |
| 解锁矩阵 | 媒体 / AI 平台 × 节点，色块表示成功 / 失败 |
| 拓扑 | 入口 / 出口 IP、国家、ASN 与聚合统计 |

## 7. 功能范围

已实现核心功能：

- 全部测试：`speed` / `media` / `ai` / `topo` / `nat`
- 解锁矩阵（媒体 + AI 平台色块）
- 拓扑图（入口 / 出口聚合表）与渲染 PNG 预览/保存/分享
- 主题切换（System / Light / Dark / NekoSpeed）
- 订阅 URL 与粘贴内容双模式（订阅 URL 落盘传参，日志脱敏）、历史归档与 VPN 冲突检测

## 8. 构建流程

1. 在 NekoSpeed 项目根目录执行构建脚本（自动下载内置中文字体，并编译 + gzip 压缩 Go 二进制到 `android/app/src/main/assets/`）：

```bash
./scripts/build.sh
```

2. 在 Android Studio 打开 `android/` 目录或使用 Gradle Wrapper 编译 APK：

```bash
cd android
./gradlew assembleDebug
```

> 更多开发阶段与真机验证细节见 [docs/android-progress.md](android-progress.md)。

## 9. 注意事项

- **网络权限**：AndroidManifest 加 `INTERNET`
- **本地代理冲突**：若手机开着 Clash/V2ray 全局 VPN，测速结果可能失真；建议提示用户关闭
- **后台执行**：测试可能耗时 10 分钟，用 `WorkManager` 或前台服务避免被杀
- **权限**：Android 10+ 写文件用私有目录，无需额外权限
- **GPL 合规**：依赖 mihomo，分发 APK 需遵守 GPL-3.0，源码可访问
- **字体与渲染**：PNG 由 NekoSpeed 生成，手机上显示即可；UI 用原生 Compose 避免字体问题

## 10. 实现步骤

1. [x] 修改 `scripts/build.sh` 加 `android arm64` 目标
2. [x] 创建 `android/` 项目骨架
3. [x] 实现 `NekoSpeedRunner` 二进制调用
4. [x] 实现 `ResultRepository` JSON 解析
5. [x] 实现 Compose UI：输入页 / 运行页 / 结果页
6. [x] 实现解锁矩阵与拓扑表
7. [x] 实现主题切换（System / Light / Dark / NekoSpeed）
8. [x] 处理后台运行、VPN 冲突提示、分享 JSON/PNG（Session 3）
9. [x] 订阅拉取兼容：重试 / HTTP/1.1 降级 / 完整 UA
10. [x] 粘贴订阅内容模式（绕过 URL 拉取失败）

---

> 仅供学习研究使用，请勿用于违反当地法律法规的场景。
