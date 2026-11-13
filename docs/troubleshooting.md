# NekoSpeed 问题排查记录

## 案例：anytls 订阅本地测试全部失败（丢包 100%、速度为 0），VPS 正常

### 现象

- 订阅：`https://ding.81dlg.com/dongligang/...`（动力港，42 个 anytls 节点）
- **VPS（境外）**：42/42 成功，速度/延迟/出口全部正常
- **本地 Mac（中国移动网络）**：所有节点 `packetLoss=1`、`avgHTTPS=0`、速度全 0、`outbound` 为空，但显示"成功"
- 同一时刻本地 Clash Verge 使用同一订阅也不通（502）

### 根因

**机场使用 DDNS 高频轮换服务器 IP 抗封锁，IP 池中部分旧 IP 已被 GFW 封死。本地测试成败取决于"解析那一刻轮到的 IP 是否还活着"。**

证据链：

1. 节点配置正常：`type: anytls`、`sni: jd.com`、`skip-cert-verify: true`，mihomo **v1.19.27+** 已支持 anytls（v1.19.29 验证可用）
2. 失败时临时加调试日志可见真实错误：
   ```
   failed to create session: dial tcp 168.107.10.200:51401: operation was canceled
   ```
3. `nc`/`ping` 验证该 IP **全端口 + ICMP 均不可达**；`traceroute` 死在中国移动国际出口（`223.120.x`）—— 典型 GFW 国际出口封 IP
4. 域名是 DDNS 链：`dlg-hk.dlg365.com → trans8888-all.trans8888.com → ddnsXXX.woaitutu520.com → A 记录`，**TTL 仅 5 秒**，IP 在 Oracle Cloud 池内不断更换
5. Go 解析器复现：多次查询返回死/活 IP 混合列表，mihomo `ResolveIPv4` 从列表中**随机选取**（`randv2.IntN`），选中死 IP 即全挂
6. IP 轮换到活 IP（如 `168.107.53.107`）后立即重测 → 延迟 172 ms、速度 15.6 MB/s、出口香港，**完全正常**
7. VPS 从不失败：境外无封锁，池中所有 IP 均可达

### 关键排查手法

- `inbound`（入口 IP）**只是本地 DNS 解析 + 直连查询**，不经过代理，有值不代表代理工作；判断代理是否真通看 **`outbound`（出口 IP）是否为空**（`internal/geo/geoip.go:184-200`，outbound 失败静默返回 nil）
- NekoSpeed 对拨号错误完全静默：`latency.go` 只计数丢包、`speed.go` 直接吞错、`geoip.go` outbound 失败无输出。排查时需临时在 `internal/nodepool/pool.go` 的 `proxyDialer.DialContext` 加 `fmt.Fprintf(os.Stderr, ...)` 打印真实错误
- 本机若开着 Clash Verge 等 TUN 代理先排除干扰（本次 TUN 实际未接管路由，utun 无地址无路由表项，`route -n get <ip>` 确认走物理网卡）

### 解决方案

| 方案 | 说明 |
|---|---|
| **直接重跑** | 最简单。TTL 5 秒，重跑时大概率换到活 IP（本次就是这么通的） |
| 临时钉 hosts | `sudo sh -c 'echo "168.107.53.107 dlg-hk.dlg365.com" >> /etc/hosts'`，用完即删（IP 随时失效） |
| 用 VPS 测 | 境外网络无封锁，结果稳定，推荐长期使用 |
| 向机场反馈 | IP 池中被封 IP 占比高（本例 Oracle 韩国段被移动方向封锁较多） |

### 代码层面已实现的工程防护与改进

针对上述问题以及本地代理环境干扰，当前版本已在代码层面内置以下自动化防护机制：

1. **多公共 DNS 采样 + TCP 预检锁定活 IP（`internal/nodepool/preflight.go`）**：
   - 测试前并发向阿里（`223.5.5.5`）、腾讯（`119.29.29.29`）、114、Cloudflare、Google 等多组 DNS 发起解析，收集 DDNS 候选 IP 池并执行 TCP 握手探测，自动将节点 `server` 替换为首个可达的真实 IP。
   - 自动识别并剔除 `198.18.0.0/15`（Clash Fake-IP 段）、回环及链路本地地址，防止 DNS 劫持污染节点入口。
2. **自动绑定物理网卡绕过本地 TUN（`internal/nodepool/netbind.go`）**：
   - 启动时自动探测默认路由对应的真实物理网卡（排除 `Mihomo`、`Meta`、`Clash`、`utun`、`wintun`、`sing-box`、`tailscale` 等虚拟网卡），并设置 `mihomo` 的 `dialer.DefaultInterface`。
   - 底层通过 `IP_UNICAST_IF`（Windows）/ `IP_BOUND_IF`（macOS）/ `SO_BINDTODEVICE`（Linux）强制所有节点拨号与预检流量直走物理网卡，彻底解决本地开启 TUN 模式导致的“代理套代理”失真。
3. **100% 丢包快速熔断与错误透传（`internal/runner/runner.go` & `internal/tester/latency.go`）**：
   - 延迟测试全部失败（丢包率 100%）时，将底层真实拨号错误记入 `NodeResult.Err`，并自动跳过后续耗时的下载测速、流媒体/AI 解锁、出口 IP 与 NAT 探测，避免无效等待与“全 0 假成功”。
4. **完整支持 `anytls://` 分享链接与协议展示（`internal/subscription/sharelink.go` & `internal/render/render.go`）**：
   - 支持直接解析 `anytls://` 分享链接（自动提取密码、SNI、ALPN、`insecure`/`skip-cert-verify` 等参数），并在渲染图表中规范显示为 `AnyTLS`。
