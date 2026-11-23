package tester

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/pion/stun"

	"nekospeed/internal/model"
	"nekospeed/internal/nodepool"
)

// NAT 类型检测结果（写入 NodeResult.NAT）。
const (
	NATFullCone           = "FullCone"
	NATRestrictedCone     = "RestrictedCone"
	NATPortRestrictedCone = "PortRestrictedCone"
	NATSymmetric          = "Symmetric"
	NATUnknown            = "Unknown"
)

// stunServers 默认 STUN 服务器（优先完整支持 RFC 5780 CHANGE-REQUEST 的服务器，Google 仅作兜底）。
var stunServers = []string{
	"stun.miwifi.com:3478",
	"stun.hot-chilli.net:3478",
	"stunserver2025.stunprotocol.org:3478",
	"stun.voipgate.com:3478",
	"stun.l.google.com:19302",
}

// CHANGE-REQUEST 标志位（RFC 5780 Section 6）。
const (
	changeIP   = 0x04
	changePort = 0x02
)

// natTimeout NAT 检测整体超时。
const natTimeout = 15 * time.Second

// stunAttemptTimeout 单次 STUN 请求等待响应超时。
var stunAttemptTimeout = 3 * time.Second

var resolveHostDirectFn = nodepool.ResolveHostDirect

type stunResult struct {
	mapped   *stun.XORMappedAddress
	hasOther bool
}

// resolveSTUNAddr 解析 STUN 服务器地址（IP 字面量直接构造；域名优先直连 DNS 绕过 Clash TUN Fake-IP，失败回退系统解析）。
func resolveSTUNAddr(ctx context.Context, srv string) (*net.UDPAddr, error) {
	host, portStr, err := net.SplitHostPort(srv)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); ip != nil {
		return &net.UDPAddr{IP: ip, Port: port}, nil
	}
	rctx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	ipStr := resolveHostDirectFn(rctx, host)
	cancel()
	if ipStr != "" {
		if ip := net.ParseIP(ipStr); ip != nil {
			return &net.UDPAddr{IP: ip, Port: port}, nil
		}
	}
	return net.ResolveUDPAddr("udp", srv)
}

// NAT 基于 RFC 5780 流程经代理 UDP 检测 NAT 类型。
// UDP 必须走代理：d 需实现 nodepool.UDPProxyDialer，否则返回 Unknown。
func NAT(ctx context.Context, d model.Dialer) string {
	udp, ok := d.(nodepool.UDPProxyDialer)
	if !ok || !udp.SupportsUDP() {
		return NATUnknown
	}

	ctx, cancel := context.WithTimeout(ctx, natTimeout)
	defer cancel()

	resolved := make([]*net.UDPAddr, len(stunServers))
	resolveTried := make([]bool, len(stunServers))
	seenAddrs := make(map[string]bool, len(stunServers))
	getAddr := func(idx int) *net.UDPAddr {
		if !resolveTried[idx] {
			resolveTried[idx] = true
			if addr, err := resolveSTUNAddr(ctx, stunServers[idx]); err == nil && addr != nil {
				key := addr.String()
				if !seenAddrs[key] {
					seenAddrs[key] = true
					resolved[idx] = addr
				}
			}
		}
		return resolved[idx]
	}

	// test1（无 change-request）：优先选择返回 OTHER-ADDRESS / CHANGED-ADDRESS 的服务器作为 primary；
	// 若只有不返回 OTHER-ADDRESS 的服务器响应，暂存为 fallback 并在遍历完成后回退使用。
	var (
		pc             net.PacketConn
		primary        *net.UDPAddr
		mapped1        *stun.XORMappedAddress
		hasOther       bool
		fallbackConn   net.PacketConn
		fallbackAddr   *net.UDPAddr
		fallbackMapped *stun.XORMappedAddress
		failedAddrs    = map[string]bool{}
	)
	for i := range stunServers {
		if ctx.Err() != nil {
			break
		}
		addr := getAddr(i)
		if addr == nil {
			continue
		}
		conn, err := udp.ListenPacket(ctx, addr.String())
		if err != nil {
			failedAddrs[addr.String()] = true
			continue
		}
		res, err := stunTest(ctx, conn, addr, 0)
		if err != nil {
			conn.Close()
			failedAddrs[addr.String()] = true
			continue
		}
		if res.hasOther {
			if fallbackConn != nil {
				fallbackConn.Close()
				fallbackConn = nil
			}
			pc, primary, mapped1, hasOther = conn, addr, res.mapped, true
			break
		}
		if fallbackConn == nil {
			fallbackConn, fallbackAddr, fallbackMapped = conn, addr, res.mapped
		} else {
			conn.Close()
		}
	}
	if pc == nil {
		if fallbackConn == nil {
			return NATUnknown // test1 无响应：UDP 被阻断
		}
		pc, primary, mapped1 = fallbackConn, fallbackAddr, fallbackMapped
	} else if fallbackConn != nil {
		fallbackConn.Close()
	}
	defer pc.Close()

	// 经代理时无法获知真实本地地址（出口在代理服务器），跳过“映射==本地”比较，按有 NAT 处理。

	// test2（change ip+port）：仅在服务器支持 OTHER-ADDRESS 时可验证，能收到其他地址:端口发来的响应即 FullCone
	if hasOther {
		if _, err := stunTest(ctx, pc, primary, changeIP|changePort); err == nil {
			return NATFullCone
		}
	}

	// 向另一台 STUN 服务器（同一 socket）再做一次 mapping，判断 Symmetric
	for i := range stunServers {
		if ctx.Err() != nil {
			break
		}
		addr := getAddr(i)
		if addr == nil || addr.String() == primary.String() || failedAddrs[addr.String()] {
			continue
		}
		if res, err := stunTest(ctx, pc, addr, 0); err == nil {
			if !res.mapped.IP.Equal(mapped1.IP) || res.mapped.Port != mapped1.Port {
				return NATSymmetric
			}
			break
		}
	}

	// 无 OTHER-ADDRESS / CHANGED-ADDRESS 时无法可靠校验 CHANGE-REQUEST，不得误判为 PortRestrictedCone
	if !hasOther {
		return NATUnknown
	}

	// test3（change port only）：区分 RestrictedCone / PortRestrictedCone
	if _, err := stunTest(ctx, pc, primary, changePort); err == nil {
		return NATRestrictedCone
	}
	return NATPortRestrictedCone
}

// stunTest 发送一次 Binding Request（changeFlags 为 CHANGE-REQUEST 标志位，0 表示不带该属性），
// 最多重试 2 次，返回映射地址与 OTHER-ADDRESS 支持信息。
func stunTest(ctx context.Context, pc net.PacketConn, server *net.UDPAddr, changeFlags byte) (*stunResult, error) {
	req := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	if changeFlags != 0 {
		req.Add(stun.AttrChangeRequest, []byte{0, 0, 0, changeFlags})
	}
	req.Encode()

	buf := make([]byte, 1500)
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, err := pc.WriteTo(req.Raw, server); err != nil {
			return nil, err
		}
		res, err := readStunResponse(pc, req, buf)
		if err == nil {
			return res, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// readStunResponse 在等待窗口内读取匹配事务 ID 的 Binding Success 响应，
// 解析映射地址以及 RFC 5780 OTHER-ADDRESS（兼容回退 RFC 3489 CHANGED-ADDRESS）。
func readStunResponse(pc net.PacketConn, req *stun.Message, buf []byte) (*stunResult, error) {
	deadline := time.Now().Add(stunAttemptTimeout)
	for {
		if err := pc.SetReadDeadline(deadline); err != nil {
			return nil, err
		}
		n, _, err := pc.ReadFrom(buf)
		if err != nil {
			return nil, err
		}
		msg := &stun.Message{}
		if _, err := msg.Write(buf[:n]); err != nil {
			continue // 非 STUN 报文，丢弃
		}
		if msg.Type != stun.BindingSuccess || msg.TransactionID != req.TransactionID {
			continue
		}
		mapped := &stun.XORMappedAddress{}
		if err := mapped.GetFrom(msg); err != nil {
			// 部分服务器只回 MAPPED-ADDRESS
			var plain stun.MappedAddress
			if err2 := plain.GetFrom(msg); err2 != nil {
				return nil, err
			}
			mapped.IP, mapped.Port = plain.IP, plain.Port
		}
		var hasOther bool
		var other stun.OtherAddress
		if err := other.GetFrom(msg); err == nil && len(other.IP) > 0 && other.Port > 0 {
			hasOther = true
		} else {
			var changed stun.MappedAddress
			if err := changed.GetFromAs(msg, stun.AttrChangedAddress); err == nil && len(changed.IP) > 0 && changed.Port > 0 {
				hasOther = true
			}
		}
		return &stunResult{mapped: mapped, hasOther: hasOther}, nil
	}
}
