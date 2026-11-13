<div align="center">

<img src="docs/nekospeed-icon.png" width="120" alt="NekoSpeed 图标">

# NekoSpeed

机场（代理订阅）批量测速与检测工具。基于 [mihomo](https://github.com/MetaCubeX/mihomo) 内核拨号，支持一键输出 MiaoKo 风格 PNG 结果图与结构化 JSON，并提供零外部依赖的内嵌 Web UI 与原生 Android 客户端。

[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8.svg)](https://go.dev)
[![mihomo](https://img.shields.io/badge/mihomo-v1.19.34-2ea44f.svg)](https://github.com/MetaCubeX/mihomo)
[![CI & Release](https://github.com/xiongzai666/NekoSpeed/actions/workflows/ci-release.yml/badge.svg)](https://github.com/xiongzai666/NekoSpeed/actions/workflows/ci-release.yml)
[![Sync Upstream & Mihomo](https://github.com/xiongzai666/NekoSpeed/actions/workflows/sync-mihomo.yml/badge.svg)](https://github.com/xiongzai666/NekoSpeed/actions/workflows/sync-mihomo.yml)
[![Android](https://img.shields.io/badge/Android-8.0+-3DDC84.svg)](android/)

</div>

> 本仓库基于 [KaWaIDeSuNe/NekoSpeed](https://github.com/KaWaIDeSuNe/NekoSpeed) 持续维护，重点扩展了模块化内嵌 Web UI、接口访问令牌鉴权（`-web-token`）、物理网卡自动绑定（防本地 TUN 干扰）、高精度多源 GeoIP 识别与冷却机制、深色高对比度主题渲染，以及每日自动同步上游与 `mihomo` 内核的 GitHub Actions 发布流水线。
>
> **开源协议**：因核心依赖 `mihomo`（GPL-3.0）静态编译进产物，本项目整体遵循 **GPL-3.0** 协议开源（见 [LICENSE](LICENSE)）。

---

## 目录

- [核心特性](#核心特性)
- [快速开始](#快速开始)
- [内嵌 Web UI 指南](#内嵌-web-ui-指南)
- [CLI 用法与参数参考](#cli-用法与参数参考)
- [输出产物与主题](#输出产物与主题)
- [核心工程机制](#核心工程机制)
- [Android 客户端](#android-客户端)
- [GitHub Actions 自动化流水线](#github-actions-自动化流水线)
- [仓库结构与文档索引](#仓库结构与文档索引)
- [开源协议与第三方合规](#开源协议与第三方合规)

---

## 核心特性

- **吞吐与延迟测速**：支持多线程 / 单线程下载测速（每秒速度序列、平均速度、峰值速度、单节点流量上限 `-max-mb`）、TLS RTT 与 HTTPS 延迟测量（基于 `httptrace` 精确计时）。
- **并发探测与下载串行保护**：延迟、流媒体/AI 解锁、拓扑与 NAT 支持多节点并发探测（默认 `-concurrency 6`）；大流量下载测速通过独立信号量自动串行执行，兼顾整体耗时与带宽测试准确度。
- **流媒体解锁检测（10 项）**：YouTube、Netflix、Disney+、B 站港澳台、DAZN、Abema、巴哈姆特、Steam、Spotify、TikTok。
- **AI 服务可用性检测（8 项）**：OpenAI (ChatGPT)、Copilot、Gemini、Llama (MetaAI)、Claude、Perplexity、Sora、Grok。
- **入口 / 出口拓扑分析**：直连探测节点入口 IP 与代理实测落地真实 IP（Cloudflare Trace + 多源 GeoIP / 本地 MMDB），按接入簇聚合并输出入口 / 出口拓扑表。
- **UDP NAT 类型探测**：基于 STUN（RFC 5780）识别 `FullCone`、`Restricted`、`PortRestricted`、`Symmetric`。
- **智能过滤与命名**：自动跳过“剩余流量 / 套餐到期 / 官网 / 重置”等提示类伪节点（`-skip-info`），支持正则白名单（`-filter`）与黑名单（`-exclude`），并自动提取订阅名称作为文件名前缀。
- **21 款渲染主题**：包含 `default`、16 款无底纹浅色色相主题与 4 款深色高对比度主题，文字颜色随底色亮度自动反白或深灰适配。

---

## 快速开始

### 方式一：下载预编译产物（推荐）

前往 [GitHub Releases](https://github.com/xiongzai666/NekoSpeed/releases) 下载对应平台单文件二进制或归档包（含 `SHA256SUMS.txt` 校验）：

- **Windows**：直接双击 `nekospeed-windows-amd64.exe`（无命令行参数时自动在 `http://127.0.0.1:18189` 启动内嵌 Web UI 并打开默认浏览器）；也可在 PowerShell / CMD 中通过 CLI 运行。
- **macOS / Linux**：解压后赋予可执行权限（`chmod +x nekospeed-*`），在终端执行 `-web` 或 CLI 命令。

### 方式二：从源码构建

需要 **Go 1.25+**（`go.mod` 当前依赖 `github.com/metacubex/mihomo v1.19.34`）：

```bash
# 仅构建当前平台二进制
go build -o nekospeed ./cmd/nekospeed

# 交叉编译桌面全平台（Windows / macOS / Linux）及 Android（需配置 NDK）
./scripts/build.sh
```

跨平台产物默认输出至 `dist/` 目录，包含 `windows-amd64`、`windows-arm64`、`darwin-amd64`、`darwin-arm64`、`linux-amd64`、`linux-arm64`。

---

## 内嵌 Web UI 指南

内嵌 Web UI 的前端资源（`internal/webui/static/` 中的 `index.html`、`app.css`、`app.js`）通过 `go:embed` 直接打包进单文件可执行程序，无需部署额外静态目录。

### 启动与访问控制

```bash
# 仅本机回环访问（默认）
./nekospeed -web 127.0.0.1:18189

# 监听局域网或远程端口时，建议配置 -web-token（或环境变量 NEKOSPEED_WEB_TOKEN）开启鉴权
./nekospeed -web 0.0.0.0:18189 -web-token "your-secret-token"
```

> **安全说明**：监听非回环地址（如 `0.0.0.0`）且未设置 `-web-token` / `NEKOSPEED_WEB_TOKEN` 时，控制台会输出安全警告。开启令牌后，所有 `/api/*` 与 `/files/*` 请求均需携带 `Authorization: Bearer <token>`、`X-NekoSpeed-Token`、`?token=` 或登录 Cookie；浏览器首次访问遇 `401` 时会自动弹出令牌输入框并保存在本地会话中。

### 操作工作流

1. **三模式订阅输入**：通过顶部「订阅链接 / 本地文件 / 粘贴内容」分段切换器导入订阅，支持直接拖拽 `.yaml` / `.txt` 文件或粘贴多行节点；切换订阅来源或替换 URL 时，未手动锁定的订阅名称会自动重置并重新识别。
2. **测前挑节点**：点击「挑节点」即可在不触发测速的情况下预览全部节点，支持关键词搜索、一键排除倍率节点以及按需勾选目标节点。
3. **节流流式看板**：测试期间通过 SSE（`event: node`）实时推送单节点结果，前端按动画帧合批刷新表格并维护最近 1500 行终端日志，动态展示可用节点数、最高峰值速度、累计流量与预计剩余时间（ETA）。
4. **免重测即时重绘**：测完后无需重新发起网络请求，可直接更换配色主题、排序规则、自定义水印或带宽标题前缀（例如 `1Gbps` 或 `上海电信@1Gbps`）重新渲染并覆盖/保存全套 PNG。
5. **导出与历史回看**：支持一键复制高清 PNG 到剪贴板（含非 HTTPS 环境兼容回退）、导出 UTF-8 BOM CSV、下载 JSON，以及在顶部历史记录条随时切换查看或删除过往报告（保存在 `out/web/<时间戳>/`）。

---

## CLI 用法与参数参考

### 常用命令示例

```bash
# 全量测试（测速 + 流媒体 + AI + 拓扑 + NAT）
./nekospeed -sub "https://example.com/sub" -o out/

# 仅测速并按平均速度降序排序，排除倍率节点
./nekospeed -sub "https://example.com/sub" -tests speed -sort speed -exclude "2x|3x|倍率"

# 使用本地 Clash 配置 + 过滤地区 + 走代理拉取订阅 + 指定离线 GeoIP 数据库
./nekospeed -config clash.yaml -tests speed,media -filter "香港|日本" -proxy "http://127.0.0.1:7890" -mmdb GeoLite2-ASN.mmdb
```

### 全部参数（`cmd/nekospeed/main.go`）

| 参数 | 默认值 | 说明 |
|---|---|---|
| `-sub` | - | 订阅链接或本地文件路径（自动识别 Clash YAML 或分享链接） |
| `-config` | - | 本地 Clash 配置文件路径（等价于 `-sub` 本地文件） |
| `-proxy` | 环境变量 `HTTPS_PROXY` | 拉取订阅时使用的 HTTP/SOCKS5 代理（如 `http://127.0.0.1:7890`，不影响测速直连） |
| `-tests` | `all` | 测试项，逗号分隔：`speed,media,ai,topo,nat` 或 `all` |
| `-threads` | `8` | 测速下载并发线程数（`1-32`） |
| `-duration` | `10` | 单节点测速时长（秒，`1-30`） |
| `-max-mb` | `0` | 单节点下载测速流量上限（MB，`0` 表示不限制） |
| `-single` | `false` | 单线程测速（等价于 `-threads 1`） |
| `-filter` | - | 节点名白名单正则 |
| `-exclude` | - | 节点名黑名单正则（如 `倍率\|到期\|官网`） |
| `-skip-info` | `true` | 自动跳过“剩余流量 / 到期时间 / 官网”等提示类伪节点 |
| `-sort` | 保持订阅原序 | 结果排序方式：`speed`（平均速度降序）、`latency`（延迟升序）、留空保持原序 |
| `-concurrency` | `6` | 并发探测的节点数（下载测速内部受独立信号量保护，单节点串行执行） |
| `-o` | `out` | 输出目录 |
| `-mmdb` | - | MaxMind MMDB 路径（支持 City / ASN / Country 等库，缺失字段在线补全） |
| `-font` | 自动探测 | 自定义中文字体路径（Windows 探测 `%WINDIR%\Fonts` 等，跨平台自动匹配系统字体） |
| `-watermark` | 无 | 图片水印文字 |
| `-bandwidth` | 自动折算 | 本地带宽或标题后缀（如 `1Gbps` 或 `上海电信@1Gbps`），未指定时按峰值折算 |
| `-name` | 自动提取 | 覆盖自动提取的订阅名称，用于文件名前缀与图片标题 |
| `-theme` | `default` | 渲染主题名（如 `default`、`sky_light`、`nord_dark` 等，见下文清单） |
| `-speed-url` | 内置大文件 | 自定义下载测速文件 URL（默认 Cloudflare / Google 大文件） |
| `-ping-url` | `http://www.gstatic.com/generate_204` | HTTPS/TLS 延迟测试地址 |
| `-ping-count` | `4` | 每节点延迟测试次数 |
| `-preflight` | `true` | 测试前采样多公共 DNS 并 TCP 探活，锁定可用服务器 IP 并过滤 Fake-IP |
| `-web` | - | 启动内嵌 Web UI 并指定监听地址（如 `127.0.0.1:18189` 或 `:18189`） |
| `-web-token` | 环境变量 `NEKOSPEED_WEB_TOKEN` | Web UI 接口鉴权令牌；非空时保护 `/api/*` 与 `/files/*` |

### 支持的订阅格式

- **Clash / Mihomo YAML**：标准 `proxies:` 节点列表，支持 `mihomo` 全系协议。
- **分享链接订阅（Base64 编码或纯文本逐行）**：`ss://`、`ssr://`、`vmess://`、`vless://`、`trojan://`、`hysteria://` / `hy://`、`hysteria2://` / `hy2://`、`tuic://`、`anytls://`、`snell://`。

---

## 输出产物与主题

每次测试会在输出目录下按 `<订阅名>_<YYYYMMDD_HHMMSS>_*`（未提取到订阅名时按 `<YYYYMMDD_HHMMSS>_*`）生成以下文件：

| 文件名 | 内容说明 |
|---|---|
| `*_speed.png` | 测速结果图（延迟色块、平均/峰值速度、每秒速度迷你柱状图、UDP NAT 类型） |
| `*_media.png` | 10 项流媒体平台解锁矩阵图 |
| `*_ai.png` | 8 项 AI 平台可用性检测矩阵图 |
| `*_full.png` | 综合宽图（同时开启 `speed` 与 `media`/`ai` 时生成） |
| `*_topology.png` | 入口 / 出口拓扑聚合表（开启 `topo` 时生成） |
| `*_result.json` | 完整结构化测试结果（用于归档、`ReRender` 重绘或 Android/第三方消费） |

**内置 21 款主题**：
- **经典默认**：`default`
- **浅色系（16 款）**：`rose_light`、`peach_light`、`amber_light`、`yellow_light`、`lime_light`、`green_light`、`mint_light`、`teal_light`、`cyan_light`、`sky_light`、`azure_light`、`blue_light`、`indigo_light`、`violet_light`、`purple_light`、`pink_light`
- **深色系（4 款）**：`nord_dark`、`tokyo_dark`、`forest_dark`、`sunset_dark`

---

## 核心工程机制

针对国内网络环境与本地代理干扰，程序在运行时默认启用以下防护（排查思路与真实案例详见 [问题排查与防护机制](docs/troubleshooting.md)）：

1. **物理网卡自动绑定（防 TUN 套代理）**：自动扫描系统默认路由对应的物理网卡（过滤 `Mihomo`、`Clash`、`utun`、`wintun`、`sing-box`、`tailscale` 等虚拟接口），通过 `dialer.DefaultInterface` 直绑物理网卡发送测速与预检流量。
2. **多公共 DNS + TCP 预检**：并发查询阿里、腾讯、114、Cloudflare、Google DNS，剔除 `198.18.0.0/15` Fake-IP 并对候选 IP 做 TCP 握手探活，规避 DDNS 轮换池中的不可达 IP。
3. **高精度本地与节点 GeoIP 链**：
   - **本机网络归属**：优先由太平洋电脑网（自适应 GBK 转码）取得公网 IPv4，再传给纯真 CZ88（精确到区县级并过滤 `"市辖区"`）或百度大数据（省市拆分）解析中文简称（如 `安庆桐城移动`）。
   - **节点入口 / 出口**：直连数据源内置 3 分钟失败冷却防止不可达或限额接口逐节点空等超时；国内库缺失数字 ASN 时自动以 2 秒短超时从 `ip-api.com` / `api.ip.sb` 补全；代理出口优先通过 Cloudflare Trace 测定真实落地 IP 与机房三字码，`-mmdb` 缺少城市或 ASN 时自动在线补全。
4. **100% 丢包快速熔断**：延迟探测全部失败时记录真实底层错误并跳过后续测速、解锁与拓扑探测，避免无效等待和“全 0 假成功”。

---

## Android 客户端

`android/` 目录提供基于 Jetpack Compose + Material 3 的原生 Android 客户端，内部嵌入交叉编译后的 Go 二进制与中文字体，实现离线包内一键调用与结果渲染：

- **主要特性**：支持订阅 URL / 粘贴内容双输入、5 项测试全覆盖、`Node / Unlock / Topology / Images` 四 Tab 报告、PNG 保存至系统相册与分享、历史归档管理、前台服务（`NekoSpeedService`）保活与通知栏取消、VPN 冲突检测。
- **兼容性**：覆盖 `arm64` / `amd64` 双架构（Go 二进制经 gzip 压缩至约 10 MB），内置 `NotoSansCJKsc-Regular.otf` 兜底厂商 ROM 字体兼容问题，并在 Android 14+ SELinux 限制下自动通过 `/system/bin/linker64` 拉起二进制。
- **常用构建命令**：

```bash
./scripts/build.sh            # 交叉编译并压缩 Android arm64/amd64 二进制，自动下载内置字体
cd android
./gradlew assembleDebug       # 构建调试版 APK
./gradlew assembleRelease     # 构建正式版 APK（需自备 android/keystore.properties）
```

> 详细架构与开发记录见 [Android 客户端开发文档](docs/android-client.md) 和 [Android 实施进度](docs/android-progress.md)。

---

## GitHub Actions 自动化流水线

工作流配置位于 `.github/workflows/`：

1. **[Sync Upstream & Mihomo Auto Release](.github/workflows/sync-mihomo.yml)**：
   - 每日 UTC `02:00`（北京时间 `10:00`）定时检查，亦支持在 Actions 页面通过 `Run workflow` 手动触发（可选 `force_release` 强制基于当前代码重发版）。
   - 自动拉取上游 `KaWaIDeSuNe/NekoSpeed` 的 `main` 分支：可快进/无冲突则直接合并；若存在文件冲突，则中止合并并将上游提交推至 `sync/upstream-main` 自动创建 PR，防止覆盖本地改进。
   - 查询 `MetaCubeX/mihomo` 最新正式版 Tag 并更新依赖，运行全量单测与 6 平台交叉编译后自动发布至 Releases；若同 UTC 日期已有同名 Tag，自动追加短提交号（如 `v2026.11.13-v1.19.34-<sha>`）避免冲突。
2. **[CI & Tag Release](.github/workflows/ci-release.yml)**：
   - 监听 `main` 分支推送、PR、手动触发与 `v*` 标签推送，执行全量单元测试并在打标签或手动发布时构建多平台压缩包与 `SHA256SUMS.txt`。

---

## 仓库结构与文档索引

```text
NekoSpeed/
├── cmd/nekospeed/                 # CLI 参数解析与 Web UI 入口（Windows 无参双击自启 Web UI）
├── internal/
│   ├── runner/                    # 测试编排、并发与串行信号量控制、ReRender 重绘、标题格式化
│   ├── webui/                     # Web UI HTTP 服务端、SSE 推送、令牌鉴权与 static/ 前端资源
│   ├── subscription/              # 订阅拉取、重试/HTTP1.1 降级、Clash YAML 与分享链接解析
│   ├── nodepool/                  # mihomo 代理封装、物理网卡自动绑定、多 DNS 预检防 Fake-IP
│   ├── tester/                    # 延迟（httptrace）、下载测速与 STUN NAT 类型探测
│   ├── unlock/                    # 流媒体（10 项）与 AI 服务（8 项）解锁检测
│   ├── geo/                       # 本地与节点 GeoIP（纯真/百度/太平洋/CF Trace/国际源/MMDB）及拓扑聚合
│   ├── render/                    # 纯 Go PNG 报告渲染引擎（21 款主题、系统字体自适应）
│   ├── report/                    # JSON 报告持久化与契约校验
│   └── model/                     # 共享数据模型与拨号器抽象
├── android/                       # Android Jetpack Compose 客户端工程
├── scripts/                       # build.sh（全平台及 Android 二进制构建）与 package.sh
├── .github/workflows/             # CI 测试、上游/mihomo 同步与多平台发布工作流
└── docs/
    ├── troubleshooting.md         # 常见故障排查与核心工程防护说明
    ├── android-client.md          # Android 客户端架构、构建与模块说明
    ├── android-progress.md        # Android 各阶段实施进度与真机测试记录
    └── desktop-client.md          # 原生桌面客户端（Fyne）技术规划方案（未实现）
```

---

## 开源协议与第三方合规

本项目整体遵循 **GPL-3.0**（见 [LICENSE](LICENSE)）。分发 CLI、Web UI 或 Android APK 二进制产物时，须一并提供本仓库对应源码。

| 组件 / 数据 | 许可 / 用途说明 |
|---|---|
| `github.com/metacubex/mihomo` (`v1.19.34`) | GPL-3.0；节点协议解析与底层拨号（静态链接，决定本仓主协议） |
| `NotoSansCJKsc-Regular.otf`（Android 内置） | SIL Open Font License 1.1；厂商 ROM 字体异常时的 PNG 渲染兜底 |
| `golang.org/x/image`、`golang.org/x/text`、`github.com/miekg/dns` | BSD-3-Clause；图像渲染、GBK 转码与多源 DNS 解析 |
| `gopkg.in/yaml.v3`、`github.com/pion/stun`、`github.com/oschwald/maxminddb-golang` | MIT；YAML 配置、STUN NAT 探测与 MMDB 读取 |
| Android 端（Compose / Navigation / Coil / Coroutines 等） | Apache-2.0 |
| GeoIP 数据库与在线接口 | 使用 `GeoLite2` mmdb 需遵循 MaxMind CC BY-SA 4.0 署名要求；未提供 mmdb 时默认请求纯真 CZ88、百度大数据、太平洋电脑网、Cloudflare Trace、`ip-api.com`、`api.ip.sb`、`ipwho.is`、`ipinfo.io` 等公开服务，请注意各自非商业/限频条款 |

**免责声明**：本项目仅用于网络质量测试与学习研究，请遵守当地法律法规与服务商条款；测速及解锁结果受本地网络、节点负载与第三方平台策略变更影响，仅供参考。
