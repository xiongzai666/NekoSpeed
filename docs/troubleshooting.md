# NekoSpeed 问题排查与工程防护说明

> 返回主文档：[README.md](../README.md)

---

## 一、 常见问题速查

| 现象 | 常见原因 | 处理方式 |
|---|---|---|
| 本地开启 Clash / v2rayN TUN 模式后测速走代理或入口 IP 变成 `198.18.x.x` | 虚拟网卡接管默认路由或本地 DNS 返回 Fake-IP | 检查启动日志是否出现 `已绑定物理网卡`；保持默认 `-preflight=true`（自动过滤 `198.18.0.0/15` 并直绑物理网卡）；若仍异常，排查本地防火墙是否拦截直连物理网卡出站 |
| 本地家宽归属地显示为英文城市（如 `Wuhu移动`）或仅到市级未到区县 | 纯真 CZ88 免登录每日限额耗尽（`code: 5101`）或国内接口临时不可达，回退至国际 IPv6 库 | 正常现象，内置 3 分钟源冷却机制会在当前轮次跳过限额源并依次尝试百度大数据 / 太平洋电脑网 / `api.ip.sb` / `ip-api.com`；也可在 Web UI 或通过 `-bandwidth "安庆桐城移动@1Gbps"` 手动指定标题前缀 |
| 部分节点 `丢包 100%`、延迟与速度全为 0，但在境外 VPS 正常 | 机场域名走高频 DDNS 轮换，部分解析 IP 已被网络阻断（见下文案例复盘） | 保持 `-preflight=true` 自动锁定存活 IP；或重跑重试、换用境外机器测试 |
| Windows / Android 渲染 PNG 报告中文显示方块或失败 | 精简系统或定制 ROM（如 HyperOS）魔改系统字体导致 OpenType 解析报错 | 使用 `-font <字体文件路径>` 手动指定 `NotoSansCJKsc-Regular.otf` 或系统 `msyh.ttc`；Android APK 已内置思源黑体自动兜底 |
| 局域网访问 Web UI 提示输入令牌，或 HTTP 下点击“复制图片”失败 | 启动了 `-web-token` / `NEKOSPEED_WEB_TOKEN`，或非 HTTPS 环境下浏览器限制 `ClipboardItem` API | 在弹窗输入对应令牌（或 URL 追加 `?token=...`）；非安全上下文下前端会自动提示下载 PNG 替代写入剪贴板 |

---

## 二、 典型案例复盘：DDNS 轮换封锁导致 `anytls` 订阅本地全挂

### 1. 现象

- **测试对象**：某包含 42 个 `anytls` 节点的订阅。
- **境外 VPS**：42/42 全部测试成功，延迟、速度与落地出口正常。
- **本地宽带（中国移动）**：所有节点均出现 `packetLoss=1`、`avgHTTPS=0`、速度为 0、`outbound` 为空，但旧版未感知失败错误而仍标记为“成功”；同时本地客户端使用该订阅亦无法连通（502）。

### 2. 排查与根因

**节点域名使用 TTL 极短（5 秒）的 DDNS 链高频轮换池内 IP，其中部分服务器 IP 已被国际出口阻断；本地能否测通取决于 DNS 解析瞬间轮到的 IP 是否仍存活。**

关键证据链：

1. 节点配置本身正常（`type: anytls`、`skip-cert-verify: true`，`mihomo` **v1.19.27+** 已支持 `anytls`）。
2. 底层拨号报错为 `failed to create session: dial tcp <pool-ip>:51401: operation was canceled`。
3. 对该 pool-ip 执行 `nc` / `ping` 显示全端口与 ICMP 不可达，`traceroute` 止步于运营商国际出口；而同一 DDNS 域名随后轮换到另一存活 IP 时，测速立刻恢复（延迟 `172 ms`、速度 `15.6 MB/s`、出口归属正常）。
4. 早期 Go 解析器与 `mihomo` 默认解析从 IP 列表中随机选取，一旦抽中死 IP 即整体失败；境外服务器无入口封锁，因此从不触发。

### 3. 排查经验与手工应急

- `inbound`（入口 IP）依赖直连查询与节点域名解析，有值不代表代理通道可用；**判断代理链路是否真实通畅应看延迟成功与否及 `outbound`（落地 IP）**。
- 当未修代码时，重跑等待 DDNS 换到新 IP、临时写 `/etc/hosts` 绑定确认存活的 IP，或改在境外 VPS 测试是最快手段（新版已通过 `-preflight` 自动化解决）。

---

## 三、 代码层面已实现的工程防护机制

针对上述案例及常见生产环境问题，仓库当前已内置五项防护：

1. **多公共 DNS 采样 + TCP 预检锁定活 IP（`internal/nodepool/preflight.go`）**
   - 测试前并发向阿里（`223.5.5.5`）、腾讯（`119.29.29.29`）、114、Cloudflare、Google 等公共 DNS 解析节点域名，对候选 IP 池逐一执行 TCP 握手探活，自动钉住首个可达的真实 IP。
   - 自动识别并剔除 `198.18.0.0/15`（Fake-IP 段）、回环与链路本地地址。
2. **物理网卡自动绑定绕过本地 TUN（`internal/nodepool/netbind.go`）**
   - 自动识别默认路由所属真实物理网卡（排除 `Mihomo`、`Meta`、`Clash`、`utun`、`wintun`、`sing-box`、`tailscale` 等虚拟网卡），注入 `mihomo` 的 `dialer.DefaultInterface`。
   - 底层经 `IP_UNICAST_IF`（Windows）/ `IP_BOUND_IF`（macOS）/ `SO_BINDTODEVICE`（Linux）强制预检与拨号直连物理出口。
3. **100% 丢包快速熔断与错误透传（`internal/runner/runner.go` & `internal/tester/latency.go`）**
   - 节点全部延迟测试失败时，保留真实拨号错误信息至 `NodeResult.Err`，并跳过下载测速、流媒体/AI 解锁、拓扑与 NAT 探测，避免无意义超时等待。
4. **多源 GeoIP 精度、GBK 兼容、ASN 补全与源冷却（`internal/geo/geoip.go`）**
   - 本机公网归属先经太平洋电脑网（GBK 自动转码）获取公网 IPv4，再传给纯真 CZ88（带区县精度并过滤 `"市辖区"`）与百度大数据（省市拆分），解决空参数调纯真/百度固定失败及 GBK 乱码问题。
   - 为直连 GeoIP 源引入 3 分钟失败冷却（区分网络/限额异常与“非中国大陆”正常拒绝），并在国内库缺少数字 `ASN` 或精简 `-mmdb` 缺字段时短超时在线补全；代理出口先走 Cloudflare Trace 锁定真实落地与机房码，再以 `5s` 单步 / `20s` 总预算容灾回退。
5. **完整支持 `anytls://` 分享链接与规范渲染（`internal/subscription/sharelink.go` & `internal/render/render.go`）**
   - 直接解析 `anytls://` 链接中的密码、SNI、ALPN、`insecure`/`skip-cert-verify` 参数，并在报告图中显示为 `AnyTLS`。
