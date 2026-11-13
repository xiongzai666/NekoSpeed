package nodepool

import (
	"context"
	"net"
	"net/netip"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/metacubex/mihomo/component/dialer"
	"github.com/miekg/dns"
)

// NetworkEnv 描述本机当前检测到的物理网卡与 TUN 虚拟网卡状态。
type NetworkEnv struct {
	PhysicalInterface string   `json:"physicalInterface"` // 绑定的真实物理网卡名（如 "WLAN" / "以太网" / "en0"）
	PhysicalIP        string   `json:"physicalIP"`        // 物理网卡 IPv4 地址
	TUNDetected       bool     `json:"tunDetected"`       // 是否检测到激活的代理 TUN 虚拟网卡
	TUNInterfaces     []string `json:"tunInterfaces"`     // 检测到的 TUN 虚拟网卡名列表
}

var (
	tunNameRe  = regexp.MustCompile(`(?i)(meta|mihomo|clash|wintun|utun|tun|tap|sing-box|singbox|nekoray|flclash|v2ray|xray|proxifier)`)
	virtSkipRe = regexp.MustCompile(`(?i)(meta|mihomo|clash|wintun|utun|tun|tap|sing-box|singbox|nekoray|flclash|v2ray|xray|docker|vethernet|wsl|hyper-v|vmware|vmnet|virtualbox|vbox|tailscale|zerotier|wireguard|loopback|teredo|isatap|bluetooth|npcap|radmin|hamachi)`)

	envMu     sync.Mutex
	cachedEnv NetworkEnv
	envAt     time.Time
)

// RefreshNetworkEnv 重新探测本机网卡环境，并将真实物理网卡写入 mihomo 的 DefaultInterface，
// 从而在本地开启 Clash TUN 模式时通过 IP_UNICAST_IF / IP_BOUND_IF / SO_BINDTODEVICE 直接从物理网卡出站，避免代理套代理。
func RefreshNetworkEnv() NetworkEnv {
	envMu.Lock()
	defer envMu.Unlock()

	env := detectNetworkEnv()
	cachedEnv = env
	envAt = time.Now()
	if canBindInterface() && env.PhysicalInterface != "" {
		dialer.DefaultInterface.Store(env.PhysicalInterface)
	} else {
		dialer.DefaultInterface.Store("")
	}
	return env
}

// canBindInterface 判断当前平台与权限下是否可安全绑定底层网卡：
// - Windows (IP_UNICAST_IF) 与 macOS (IP_BOUND_IF) 普通用户即可直接绑定；
// - Linux (SO_BINDTODEVICE) 需 root 权限，非 root 下跳过以免报 EPERM；
// - Android 交由系统 VpnService 管控，不绑定底层网卡名。
func canBindInterface() bool {
	switch runtime.GOOS {
	case "windows", "darwin":
		return true
	case "linux":
		return os.Geteuid() == 0
	default:
		return false
	}
}

// GetNetworkEnv 获取当前本机网卡绑定状态（带 10 秒轻量缓存）。
func GetNetworkEnv() NetworkEnv {
	envMu.Lock()
	if time.Since(envAt) < 10*time.Second && (cachedEnv.PhysicalInterface != "" || !envAt.IsZero()) {
		res := cachedEnv
		envMu.Unlock()
		return res
	}
	envMu.Unlock()
	return RefreshNetworkEnv()
}

// probeOutboundIP 通过 UDP 路由表探测系统当前默认出站 IPv4（不产生实际网络包）。
func probeOutboundIP() netip.Addr {
	conn, err := net.DialTimeout("udp4", "223.5.5.5:53", 300*time.Millisecond)
	if err != nil {
		return netip.Addr{}
	}
	defer conn.Close()
	if ua, ok := conn.LocalAddr().(*net.UDPAddr); ok && ua.IP != nil {
		if a, ok2 := netip.AddrFromSlice(ua.IP.To4()); ok2 {
			return a.Unmap()
		}
	}
	return netip.Addr{}
}

func detectNetworkEnv() NetworkEnv {
	var env NetworkEnv
	ifaces, err := net.Interfaces()
	if err != nil {
		return env
	}

	routeIP := probeOutboundIP()

	type candidate struct {
		name  string
		ip    string
		score int
	}
	var best candidate

	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil || len(addrs) == 0 {
			continue
		}

		var ipv4 netip.Addr
		hasFakeSubnet := false
		matchesRoute := false
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil {
				continue
			}
			addr, ok := netip.AddrFromSlice(ip)
			if !ok {
				continue
			}
			addr = addr.Unmap()
			if !addr.Is4() {
				continue
			}
			if IsFakeIPAddr(addr) {
				hasFakeSubnet = true
				continue
			}
			if addr.IsLinkLocalUnicast() || addr.IsLoopback() {
				continue
			}
			if routeIP.IsValid() && addr == routeIP {
				matchesRoute = true
			}
			if !ipv4.IsValid() {
				ipv4 = addr
			}
		}

		if tunNameRe.MatchString(ifc.Name) || hasFakeSubnet {
			env.TUNDetected = true
			env.TUNInterfaces = append(env.TUNInterfaces, ifc.Name)
			continue
		}
		if virtSkipRe.MatchString(ifc.Name) || !ipv4.IsValid() {
			continue
		}
		// 排除 CGNAT 100.64.0.0/10（常见于 Tailscale / 部分 TUN 默认网段）
		b := ipv4.As4()
		if b[0] == 100 && b[1] >= 64 && b[1] <= 127 {
			continue
		}

		score := 10
		if matchesRoute {
			score += 100
		}
		if b[0] == 192 && b[1] == 168 {
			score += 30
		} else if b[0] == 10 {
			score += 25
		} else if b[0] == 172 && b[1] >= 16 && b[1] <= 31 {
			score += 20
		}
		// 物理网卡通过路由器 DHCP 获取的地址极少以 .1 结尾（.1 通常是 VirtualBox/VMware 宿主机虚拟网关）
		if b[3] != 1 {
			score += 10
		}
		lower := strings.ToLower(ifc.Name)
		if strings.Contains(lower, "wlan") || strings.Contains(lower, "wi-fi") || strings.Contains(lower, "wifi") ||
			strings.Contains(lower, "以太网") || strings.Contains(lower, "乙太網路") ||
			strings.Contains(lower, "本地连接") || strings.Contains(lower, "區域連線") ||
			strings.Contains(lower, "ethernet") || strings.HasPrefix(lower, "en") || strings.HasPrefix(lower, "eth") {
			score += 15
		}
		if score > best.score {
			best = candidate{name: ifc.Name, ip: ipv4.String(), score: score}
		}
	}

	env.PhysicalInterface = best.name
	env.PhysicalIP = best.ip
	return env
}

// IsFakeIPAddr 判断 IPv4 地址是否属于 Clash Fake-IP 网段（198.18.0.0/15）或无效保留地址。
func IsFakeIPAddr(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || a.IsUnspecified() || a.IsLoopback() {
		return true
	}
	if a.Is4() {
		b := a.As4()
		// 198.18.0.0/15 (198.18.0.0 ~ 198.19.255.255)
		if b[0] == 198 && (b[1] == 18 || b[1] == 19) {
			return true
		}
	}
	return false
}

// IsFakeIPString 判断字符串形式的 IP 是否为 Fake-IP。
func IsFakeIPString(s string) bool {
	a, err := netip.ParseAddr(strings.TrimSpace(s))
	if err != nil {
		return false
	}
	return IsFakeIPAddr(a)
}

// DialDirectContext 使用绑定了本机真实物理网卡的拨号器发起直连 TCP/UDP 连接，绕过本机 TUN 虚拟网卡。
func DialDirectContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if iface := dialer.DefaultInterface.Load(); iface != "" {
		conn, err := dialer.DialContext(ctx, network, addr, dialer.WithInterface(iface))
		if err == nil {
			return conn, nil
		}
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	return d.DialContext(ctx, network, addr)
}

// QueryADirect 通过绑定物理网卡向指定公共 DNS 查询 host 的真实 A 记录（自动过滤 198.18.0.0/15 Fake-IP）。
func QueryADirect(ctx context.Context, server, host string) []netip.Addr {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(host), dns.TypeA)
	m.RecursionDesired = true

	// 优先尝试经物理网卡的 UDP 或 TCP DNS 查询；若检测到 TUN 开启，额外尝试 TCP 53 以防 UDP 53 被 dns-hijack 拦截。
	env := GetNetworkEnv()
	protos := []string{"udp", "tcp"}
	if env.TUNDetected {
		protos = []string{"tcp", "udp"}
	}

	for _, proto := range protos {
		rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		var (
			r   *dns.Msg
			err error
		)
		if iface := dialer.DefaultInterface.Load(); iface != "" {
			conn, dialErr := dialer.DialContext(rctx, proto, server, dialer.WithInterface(iface))
			if dialErr == nil {
				dc := &dns.Conn{Conn: conn}
				_ = dc.SetDeadline(time.Now().Add(3 * time.Second))
				if writeErr := dc.WriteMsg(m); writeErr == nil {
					r, err = dc.ReadMsg()
				} else {
					err = writeErr
				}
				_ = dc.Close()
			} else {
				err = dialErr
			}
		}
		if r == nil {
			c := &dns.Client{Net: proto, Timeout: 3 * time.Second}
			r, _, err = c.ExchangeContext(rctx, m, server)
		}
		cancel()
		if err != nil || r == nil {
			continue
		}
		var out []netip.Addr
		for _, rr := range r.Answer {
			if a, ok := rr.(*dns.A); ok {
				if addr, ok2 := netip.AddrFromSlice(a.A); ok2 {
					addr = addr.Unmap()
					if !IsFakeIPAddr(addr) {
						out = append(out, addr)
					}
				}
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

// ResolveHostDirect 直连解析节点服务器域名为真实 IPv4/IPv6 地址，自动绕过 Fake-IP。
func ResolveHostDirect(ctx context.Context, host string) string {
	// 1. 先尝试系统解析，若返回的不是 198.18.0.0/15 Fake-IP 则直接采用
	lctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	addrs, err := net.DefaultResolver.LookupIP(lctx, "ip", host)
	cancel()
	if err == nil {
		for _, a := range addrs {
			if a4 := a.To4(); a4 != nil {
				if addr, ok := netip.AddrFromSlice(a4); ok && !IsFakeIPAddr(addr) {
					return addr.String()
				}
			}
		}
		for _, a := range addrs {
			if addr, ok := netip.AddrFromSlice(a); ok && !IsFakeIPAddr(addr) {
				return addr.String()
			}
		}
	}
	// 2. 若系统解析被 Fake-IP 污染或失败，通过绑定物理网卡直查公共 DNS
	for _, srv := range preflightDNServers {
		if ips := QueryADirect(ctx, srv, host); len(ips) > 0 {
			return ips[0].String()
		}
	}
	return ""
}
