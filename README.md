<div align="center">

<img src="docs/nekospeed-icon.png" width="120" alt="NekoSpeed 图标">

# NekoSpeed

机场（代理订阅）测速工具。参考 [miaospeed](https://github.com/AirportR/miaospeed) 架构（mihomo 内核作拨号层），输入订阅链接或 Clash 配置，一次性跑完测试并输出 MiaoKo 风格的 PNG 结果图 + JSON。

[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8.svg)](https://go.dev)
[![mihomo](https://img.shields.io/badge/mihomo-v1.19.33-2ea44f.svg)](https://github.com/MetaCubeX/mihomo)
[![CI & Release](https://github.com/xiongzai666/NekoSpeed/actions/workflows/ci-release.yml/badge.svg)](https://github.com/xiongzai666/NekoSpeed/actions/workflows/ci-release.yml)
[![Sync Upstream & Mihomo](https://github.com/xiongzai666/NekoSpeed/actions/workflows/sync-mihomo.yml/badge.svg)](https://github.com/xiongzai666/NekoSpeed/actions/workflows/sync-mihomo.yml)
[![Android](https://img.shields.io/badge/Android-8.0+-3DDC84.svg)](android/)

</div>

> 本项目以 **GPL-3.0** 开源。因核心依赖 [mihomo](https://github.com/MetaCubeX/mihomo)（GPL-3.0）静态链接进二进制，本项目整体遵循 GPL-3.0 授权。详见 [LICENSE](LICENSE) 与[开源与合规](#开源与合规)章节。
>
> 本仓库基于上游 [KaWaIDeSuNe/NekoSpeed](https://github.com/KaWaIDeSuNe/NekoSpeed) 持续维护，拓展了内嵌 WebUI 交互工作流、自动绑定物理网卡防 TUN 干扰、深色高对比度主题渲染，以及每日自动同步上游代码与最新 `mihomo` 内核的 GitHub Actions 发布流水线。

## 目录

- [功能](#功能)
- [用法](#用法)
- [参数](#参数)
- [支持的订阅格式](#支持的订阅格式)
- [输出](#输出)
- [排障](#排障)
- [构建](#构建)
- [Windows 部署](#windows-部署)
- [自动化同步与构建 (GitHub Actions)](#自动化同步与构建-github-actions)
- [Android 客户端](#android-客户端)
- [架构](#架构)
- [开源与合规](#开源与合规)
- [说明](#说明)

---

## 功能

- **测速**：多/单线程下载测速（每秒速度序列、平均、峰值、单节点最大流量上限 `-max-mb`）、TLS RTT、HTTPS 延迟（httptrace 精确计时，TLS1.3）
- **并发探测与下载串行保护**：支持多节点并发（默认 `-concurrency 6`）同时探测延迟、流媒体/AI 解锁、拓扑与 NAT，而大流量下载测速通过独立信号量自动串行执行，兼顾筛查效率与带宽准确性
- **本地网络标注**：自动探测本机公网 IP 的运营商/城市，并在测速图/综合图标题显示（如 `上海电信@1Gbps`），可手动指定带宽（如 `1Gbps`）或完整标题前缀（如 `上海电信@1Gbps`）
- **流媒体解锁**：YouTube / Netflix / Disney+ / B站港澳台 / TikTok / DAZN / Abema / 巴哈姆特 / Steam 货币 / Spotify
- **AI 解锁**：ChatGPT (OpenAI) / Copilot / Gemini / MetaAI / Claude / Perplexity / Sora / Grok
- **线路分析**：入口（节点服务器 IP 归属）/ 出口（落地 IP 归属）聚合表，GeoLite2 mmdb 或在线 API
- **UDP NAT 类型**：STUN（RFC 5780）检测 FullCone / Restricted / PortRestricted / Symmetric
- **内嵌 Web UI**：`-web`（或 Windows 直接双击 `nekospeed.exe`）一键启动浏览器操作界面，零额外文件依赖：
  - **智能订阅输入**：支持订阅 URL、本地文件路径、拖拽上传 `.yaml`/`.txt` 文件，或直接粘贴多行分享链接 / Clash YAML 内容
  - **测前挑节点 (`🔍 挑节点`)**：无需跑测速即可快速解析订阅节点列表，支持关键词搜索、一键排除高倍率节点、自由勾选想要测试的节点
  - **单节点实时流式上屏**：每完成一个节点即通过 SSE (`event: node`) 实时写入交互式表格，并动态更新累计消耗流量与预计剩余时间（ETA）
  - **免重测秒级重绘 (`✨ 免重测立即重绘`)**：0.3 秒内按新主题、排序方式、自定义水印或带宽标签重新生成全套 PNG 图并自动持久化保存
  - **便捷分享与导出**：支持一键复制高清 PNG 到系统剪贴板（可直接粘贴至 Telegram / 微信 / QQ）、一键导出 UTF-8 BOM CSV 表格、启动时自动加载最近一次历史报告
- **自动绑定物理网卡（防 TUN 代理套代理）**：自动探测本机默认路由对应的真实物理网卡并注入 mihomo `dialer.DefaultInterface`（底层 `IP_UNICAST_IF` / `IP_BOUND_IF` / `SO_BINDTODEVICE`），配合多公共 DNS 预检自动过滤 `198.18.0.0/15` Fake-IP，即使本机开启 Clash / V2rayN TUN 模式也能直连节点真实入口
- **IP 预检**：多公共 DNS 采样节点服务器 IP + TCP 探测，锁定当前可达 IP，避免 DDNS 轮换 IP 池中被封锁的死 IP 导致整节点失败
- **智能过滤与排序**：默认自动跳过“剩余流量 / 套餐到期 / 官网”等提示类伪节点（`-skip-info`），支持正则白名单（`-filter`）、黑名单（`-exclude`）及按速度/延迟排序（`-sort`）
- **输出文件名**：自动从订阅 URL/文件/内容中提取订阅名称，作为输出文件前缀（如 `xx机场_20260724_105252_speed.png`），也可用 `-name` 手动指定
- **多款主题**：内置 `default`（经典玫瑰粉，无背景花纹）、`sky_light`（经典浅蓝，无背景花纹）、14 款柔和浅色主题与 4 款高对比度深色主题（`nord_dark` / `tokyo_dark` / `forest_dark` / `sunset_dark`），可通过 `-theme` 指定或设为 `random` 随机

---

## 用法

```bash
# Windows 下直接双击 nekospeed.exe 或命令行启用内嵌 Web UI（推荐）
./nekospeed -web 127.0.0.1:18189

# 全量测试（速度+流媒体+AI+线路+NAT）
./nekospeed -sub "https://example.com/sub" -o out/

# 只测速度，按平均速度降序排列，排除倍率节点
./nekospeed -sub "https://example.com/sub" -tests speed -sort speed -exclude "2x|3x|倍率"

# 本地 Clash 配置 + 过滤节点 + 通过本地代理拉取订阅 + GeoIP 数据库
./nekospeed -config clash.yaml -tests speed,media -filter "香港|日本" -proxy "http://127.0.0.1:7890" -mmdb GeoLite2-ASN.mmdb
```

## 参数

| 参数 | 默认 | 说明 |
|---|---|---|
| `-sub` | - | 订阅链接或本地文件（Clash YAML 或分享链接订阅，自动识别） |
| `-config` | - | 本地 Clash 配置文件（等价 `-sub` 本地文件） |
| `-proxy` | - | 拉取订阅时使用的 HTTP/SOCKS5 代理（如 `http://127.0.0.1:7890`，不影响节点测速直连） |
| `-tests` | `all` | 逗号分隔：`speed,media,ai,topo,nat` 或 `all` |
| `-threads` | `8` | 测速下载线程数（1-32） |
| `-duration` | `10` | 单节点测速时长秒（1-30） |
| `-max-mb` | `0` | 单节点下载测速最大流量上限（MB，`0` 为不限制） |
| `-single` | `false` | 单线程测速（等价 `-threads 1`） |
| `-filter` | - | 节点名保留正则（白名单） |
| `-exclude` | - | 节点名排除正则（黑名单，如 `倍率\|到期\|官网`） |
| `-skip-info` | `true` | 自动跳过“剩余流量/套餐到期/官网/重置”等提示类伪节点 |
| `-sort` | 原序 | 结果排序方式：`speed`（按平均速度降序）、`latency`（按延迟升序）、留空保持订阅原序 |
| `-concurrency` | `6` | 同时测试的节点数（延迟/解锁/拓扑并发执行，大流量下载测速内部自动串行防抢占带宽） |
| `-o` | `out` | 输出目录 |
| `-mmdb` | - | MaxMind GeoIP2 mmdb 路径（不给则用 ip-api.com / ipinfo.io） |
| `-font` | - | 自定义中文字体路径（默认自动探测系统雅黑 / 黑体 / PingFang / Noto CJK） |
| `-watermark` | 无 | 图片水印文字（默认无水印） |
| `-bandwidth` | 自动 | 本地带宽或标题后缀（如 `1Gbps` 或 `上海电信@1Gbps`），未指定时取所有节点测速峰值折算 |
| `-name` | 自动 | 订阅名称，用于输出文件名前缀与图片标题（覆盖自动提取） |
| `-theme` | `default` | 渲染主题，如 `default`（经典粉）/ `sky_light`（经典蓝）/ `mint_light` / `nord_dark` / `random` |
| `-speed-url` | - | 自定义测速文件 URL（默认 Cloudflare / Google 大文件） |
| `-ping-url` | `http://www.gstatic.com/generate_204` | 延迟测试地址 |
| `-ping-count` | `4` | 每节点延迟测试次数 |
| `-preflight` | `true` | 测试前多公共 DNS 采样服务器 IP 并 TCP 探测，锁定可达真实 IP（过滤 Fake-IP 与被封死 IP） |
| `-web` | - | 启动内嵌 Web UI（如 `127.0.0.1:18189` 或 `:18189`；Windows 下无参数双击自动以此模式启动） |

## 支持的订阅格式

- Clash / Mihomo YAML（`proxies:` 列表），支持 `anytls`、`hysteria2`、`tuic`、`vless` 等 mihomo 全部协议
- 分享链接订阅（base64 或明文逐行）：`ss://` `ssr://` `vmess://` `vless://` `trojan://` `hysteria2://`/`hy2://` `tuic://` `anytls://` `snell://`

## 输出

输出目录下按时间戳生成：

| 文件 | 说明 |
|---|---|
| `*_speed.png` | 测速图（延迟色块、平均/最高速度、每秒速度迷你柱状图、UDP 类型） |
| `*_media.png` | 流媒体解锁图 |
| `*_ai.png` | AI 可用性检测图 |
| `*_full.png` | 综合图（测速 + 解锁合并宽图，选 speed 且选 media/ai 时生成） |
| `*_topology.png` | 线路分析图（入口/出口表，选 topo 时生成） |
| `*_result.json` | 全部原始数据（延迟序列、每秒速度、解锁状态、GeoIP、NAT） |

## 排障

本地测试出现"全部节点丢包 100%、速度为 0"或 DDNS 轮换死 IP 问题时，请参考 [docs/troubleshooting.md](docs/troubleshooting.md)。

---

## 构建

需要 Go 1.24+（因依赖 `mihomo v1.19.33`）：

```bash
go build -o nekospeed ./cmd/nekospeed
```

### 跨平台编译（含 Windows / macOS / Linux）

```bash
./scripts/build.sh
```

产物在 `dist/`：`nekospeed-windows-amd64.exe`、`nekospeed-windows-arm64.exe`、`nekospeed-darwin-amd64`、`nekospeed-darwin-arm64`、`nekospeed-linux-amd64`、`nekospeed-linux-arm64`。

## Windows 部署

**方式一（直接下载预编译版，推荐）**：前往 [GitHub Releases](https://github.com/xiongzai666/NekoSpeed/releases) 下载 `nekospeed-windows-amd64.exe`（或压缩包 `nekospeed-windows-amd64.zip`），**单文件零依赖、无需安装任何运行库**。

**方式二（本机源码构建）**：安装 [Go for Windows](https://go.dev/dl/) 后在 PowerShell 执行：

```powershell
go build -o nekospeed.exe ./cmd/nekospeed
```

命令行使用（PowerShell / CMD 均可）：

```powershell
.\nekospeed.exe -sub "https://example.com/sub" -o out
.\nekospeed.exe -sub "https://example.com/sub" -tests speed -sort speed
```

### Windows 部署与使用内嵌 WebUI

WebUI 已内嵌在单文件二进制中（`go:embed`），无需任何额外静态文件：

- **直接双击启动**：在 Windows 资源管理器中直接双击 `nekospeed.exe`（或 `nekospeed-windows-amd64.exe`），程序将自动在 `http://127.0.0.1:18189` 启动内嵌 WebUI 并调用系统默认浏览器打开。
- **命令行指定端口或局域网监听**：

  ```powershell
  # 仅本机访问（默认）
  .\nekospeed-windows-amd64.exe -web 127.0.0.1:18189

  # 允许局域网其他设备访问
  .\nekospeed-windows-amd64.exe -web 0.0.0.0:18189
  ```

- **WebUI 核心工作流**：
  1. **输入订阅**：粘贴订阅 URL、本地文件路径、拖拽上传 `.yaml`/`.txt`，或点击「粘贴内容/上传」直接粘贴多行节点。
  2. **按需挑节点 (`🔍 挑节点`)**：测前一键解析订阅，弹窗支持搜索、排除高倍率节点、自由勾选目标节点。
  3. **实时流式看板**：测试过程中每测完一个节点即刻上屏，实时刷新可用节点数、最高峰值速度、累计消耗流量与预计剩余时间（ETA）。
  4. **免重测秒级重绘 (`✨ 免重测立即重绘`)**：跑完测试后无需重跑网络，0.3 秒内即可切换浅色/深色主题、调整排序、修改水印或替换标题带宽后缀（如填写 `1Gbps` 或 `上海电信@1Gbps`）。
  5. **一键分享与归档**：支持一键复制高清 PNG 到系统剪贴板、导出 CSV 表格、下载 JSON，以及在历史记录抽屉中随时回看和对比过往测速报告。

**Windows 注意事项：**

- **字体自适应**：自动按 `%WINDIR%\Fonts`（兼容非 C 盘系统）与 `%LOCALAPPDATA%\Microsoft\Windows\Fonts` 探测微软雅黑（`msyh.ttc` / `msyh.ttf`）、黑体（`simhei.ttf`）、等线（`Deng.ttf`）及新宋体（`simsun.ttc`），覆盖 Windows 7 ~ Windows 11 及精简版系统；如需自定义字体可用 `-font` 指定。
- **TUN 模式自动绕过**：程序启动时会自动探测本机默认路由对应的真实物理网卡并绑定出站（控制台输出 `🛡️ 已绑定物理网卡: <网卡名>`），同时在预检阶段过滤 `198.18.0.0/15` Fake-IP。即使本机开着 Clash Verge / Clash Nyanpasu / v2rayN 的 TUN 模式，也无需手动关闭即可直连测速。
- 结果图默认写到 `out\` 目录（WebUI 模式保存在 `out\web\<时间戳>\`），Windows 防火墙若提示联网请选择允许。

---

## 自动化同步与构建 (GitHub Actions)

本仓库配置了完整的 CI/CD 自动化工作流（位于 `.github/workflows/`）：

1. **[Sync Upstream & Latest Mihomo](.github/workflows/sync-mihomo.yml)**：
   - **触发方式**：每日定时（UTC `02:00` / 北京时间 `10:00`）自动巡检，或通过 Actions 页面手动点击 `Run workflow` 触发（支持勾选 `force_release` 强制重发版）。
   - **自动同步上游仓库**：自动拉取上游 `KaWaIDeSuNe/NekoSpeed` 的最新提交并合并至本仓库（遇冲突自动保留本仓库增强特性）。
   - **自动升级 `mihomo` 内核**：自动查询 `MetaCubeX/mihomo` 最新正式版 Release Tag，执行 `go get` 与 `go mod tidy` 升级内核，并在通过全套单元测试与 6 平台交叉编译验证后自动推送代码与标签（格式：`v<YYYY.MM.DD>-<mihomo_tag>`）。
2. **[CI & Release](.github/workflows/ci-release.yml)**：
   - **触发方式**：推送到 `main` 分支、提交 Pull Request、推送 `v*` 标签或被同步工作流调用。
   - **多平台产物发布**：自动编译 `windows-amd64`、`windows-arm64`、`darwin-amd64`、`darwin-arm64`、`linux-amd64`、`linux-arm64` 共 6 个平台的单文件二进制，打包 `.zip` / `.tar.gz` 归档及 `SHA256SUMS.txt` 校验文件并发布至 [GitHub Releases](https://github.com/xiongzai666/NekoSpeed/releases)。

---

## Android 客户端

Android 原生客户端（Jetpack Compose + 预编译 Go 二进制），源码位于 `android/`，桌面端预编译二进制发布在 [GitHub Releases](https://github.com/xiongzai666/NekoSpeed/releases)（上游 Android APK 可参见 [上游 Releases](https://github.com/KaWaIDeSuNe/NekoSpeed/releases)）。

- **功能**：订阅测速（URL / 粘贴内容）、五项测试（测速/流媒体/AI/拓扑/NAT）、节点/解锁/拓扑/图片四 Tab 报告、报告图预览与保存相册、历史记录（自动归档可查看/删除）、前台服务保活、VPN 冲突检测
- **适配**：arm64 / amd64 双架构；内置 NotoSansCJKsc 中文字体（厂商 ROM 如 HyperOS 魔改系统字体时兜底）；兼容 Android 14+ SELinux（经 `linker64` 执行）
- **隐私**：订阅 URL 落盘后以文件路径传给二进制，订阅 token 不暴露于进程命令行；订阅解析失败日志脱敏
- **构建**：

  ```bash
  ./scripts/build.sh          # 交叉编译 arm64/amd64 二进制 → assets（含字体下载）
  cd android
  ./gradlew assembleDebug     # 调试版
  ./gradlew assembleRelease   # 正式版（需 android/keystore.properties 签名配置）
  ```

- **签名**：正式版需自备 keystore（生成后务必备份，丢失将无法升级覆盖已安装版本）

---

## 架构

```
cmd/nekospeed     CLI 与 WebUI 入口编排（Windows 双击自启 WebUI）
internal/
  runner          测试全流程编排、并发控制与免重测重绘（ReRender）
  webui           内嵌单文件 WebUI 服务端与前端（SSE 实时节点流、挑节点预览、历史归档）
  subscription    订阅拉取、多源回退、智能识别与分享链接解析（含 anytls:// 等）
  nodepool        mihomo adapter.ParseProxy 拨号封装 + 物理网卡自动绑定 + 多 DNS 预检防 Fake-IP
  tester          延迟（httptrace）/ 下载测速（独立信号量串行保护 + 流量上限）/ STUN NAT
  unlock          流媒体与 AI 平台解锁检测
  geo             GeoIP（mmdb / 在线 API 并发缓存）与出入口聚合、本机 ISP 识别
  render          PNG 表格图手绘引擎（x/image，支持浅色花纹主题与深色高对比度自适应文字）
  report          JSON 报告序列化与读取
  model           共享数据结构
```

---

## 开源与合规

本项目以 **GPL-3.0** 发布（见 [LICENSE](LICENSE)），以下为第三方依赖与数据的合规说明：

### mihomo（GPL-3.0）

- 版本：`github.com/metacubex/mihomo v1.19.34`
- 用途：`internal/nodepool` 通过 `adapter.ParseProxy` 解析节点配置并作拨号层，支持 mihomo 全部代理协议（anytls 等）
- 合规：mihomo 以 GPL-3.0 授权，本项目静态链接并整体以 GPL-3.0 开源，即满足 GPL-3.0 的分发义务。分发二进制（CLI / WebUI / Android APK）时请一并提供本仓库源码。

### 中文字体（SIL OFL 1.1）

- Android 客户端内置 `NotoSansCJKsc-Regular.otf`（思源黑体，Google Noto CJK 项目）
- 许可：[SIL Open Font License 1.1](https://openfontlicense.org/)，允许自由使用、修改与再分发（再分发需保留许可声明）
- 用途：厂商 ROM 系统字体（如 HyperOS）可能无法被正确解析时，经 `-font` 参数兜底渲染报告图

### GeoIP 数据与在线 API

- `GeoLite2` mmdb 数据库（如使用）：MaxMind 要求署名，[CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/)，需保留 MaxMind 版权声明
- 在线查询（未提供 mmdb 时）：`ip-api.com`（非商业用途免费）、`ipinfo.io`（免费版需按条款署名），请自行确认各自使用条款

### 其他第三方依赖

均为宽松许可，按各自 LICENSE 要求保留版权声明：

| 依赖 | 许可 |
|---|---|
| `golang.org/x/image` | BSD-3-Clause |
| `github.com/miekg/dns` | BSD-3-Clause |
| `gopkg.in/yaml.v3` | MIT |
| `github.com/pion/stun` | MIT |
| `github.com/oschwald/maxminddb-golang` | MIT |
| Android：Compose / Navigation / Coil 等 | Apache-2.0 |

### 免责声明

- 本项目仅供网络质量测试与学习交流，请遵守所在地区法律法规与机场服务条款
- 测速与解锁检测基于各平台公开页面特征，结果仅供参考，平台改版可能导致检测项失效

---

## 说明

- 测速与解锁结果受本地网络、节点负载影响，仅供参考。
