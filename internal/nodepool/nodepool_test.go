package nodepool

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"slices"
	"testing"
	"time"

	"github.com/metacubex/mihomo/component/resolver"
	C "github.com/metacubex/mihomo/constant"
	"github.com/miekg/dns"
	"nekospeed/internal/model"
)

func TestIsFakeIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"198.18.0.1", true},
		{"198.19.255.254", true},
		{"127.0.0.1", true},
		{"0.0.0.0", true},
		{"1.1.1.1", false},
		{"198.20.0.1", false},
		{"2001:2::3af", true},
		{"2001:2::1", true},
		{"fc00::1", true},
		{"fd12:3456::1", true},
		{"::1", true},
		{"::", true},
		{"2602:fa4f:B00:50e7:3aed:dfa3:a813:5830", false},
		{"2404:c140:2202::25:d6a", false},
		{"2001:4860:4860::8888", false},
		{"2001:db8::1", false},
		{"not-an-ip", false},
	}
	for _, c := range cases {
		if got := IsFakeIPString(c.ip); got != c.want {
			t.Errorf("IsFakeIPString(%q) = %v, want %v", c.ip, got, c.want)
		}
	}
	if !IsFakeIPAddr(netip.Addr{}) {
		t.Error("zero netip.Addr should be treated as fake/invalid")
	}
}

func TestIsUDPOnlyNode(t *testing.T) {
	if !isUDPOnlyNode(model.Node{Type: "hy2"}) {
		t.Error("hy2 should be UDP-only")
	}
	if !isUDPOnlyNode(model.Node{Type: "vmess", Raw: map[string]any{"type": "tuic"}}) {
		t.Error("raw type tuic should override and be UDP-only")
	}
	if isUDPOnlyNode(model.Node{Type: "vless"}) {
		t.Error("vless should not be UDP-only")
	}
}

func TestRawPort(t *testing.T) {
	cases := []struct {
		raw  map[string]any
		want uint16
	}{
		{nil, 0},
		{map[string]any{"port": 443}, 443},
		{map[string]any{"port": int64(8443)}, 8443},
		{map[string]any{"port": float64(10443)}, 10443},
		{map[string]any{"port": "2053"}, 2053},
		{map[string]any{"port": "bad"}, 0},
	}
	for _, c := range cases {
		if got := rawPort(c.raw); got != c.want {
			t.Errorf("rawPort(%v) = %d, want %d", c.raw, got, c.want)
		}
	}
}

func TestAddrToMetadata(t *testing.T) {
	md, err := addrToMetadata("1.2.3.4:443", C.TCP)
	if err != nil || md.DstIP != netip.MustParseAddr("1.2.3.4") || md.Host != "" || md.DstPort != 443 {
		t.Fatalf("IP metadata = %+v, err=%v", md, err)
	}
	md2, err := addrToMetadata("example.com:8443", C.UDP)
	if err != nil || md2.Host != "example.com" || md2.DstIP.IsValid() || md2.DstPort != 8443 || md2.NetWork != C.UDP {
		t.Fatalf("Host metadata = %+v, err=%v", md2, err)
	}
	if _, err := addrToMetadata("bad-addr", C.TCP); err == nil {
		t.Error("expected error for missing port")
	}
}

func TestProbeIPsPartialPortSuccessAndRanking(t *testing.T) {
	orig := tcpProbeFn
	t.Cleanup(func() { tcpProbeFn = orig })

	ipDead := netip.MustParseAddr("1.1.1.1")
	ipOne := netip.MustParseAddr("2.2.2.2")
	ipBoth := netip.MustParseAddr("3.3.3.3")

	tcpProbeFn = func(_ context.Context, ip netip.Addr, port uint16) bool {
		switch ip {
		case ipBoth:
			return port == 443 || port == 8443
		case ipOne:
			return port == 443
		default:
			return false
		}
	}

	got := probeIPs(context.Background(), []netip.Addr{ipDead, ipOne, ipBoth}, []uint16{443, 8443})
	if len(got) != 2 || got[0] != ipBoth || got[1] != ipOne {
		t.Fatalf("probeIPs = %v, expected [%v %v]", got, ipBoth, ipOne)
	}
}

func TestResetHosts(t *testing.T) {
	t.Cleanup(ResetHosts)
	hv, err := resolver.NewHostValueByIPs([]netip.Addr{netip.MustParseAddr("1.2.3.4")})
	if err != nil {
		t.Fatalf("NewHostValueByIPs: %v", err)
	}
	if err := resolver.DefaultHosts.Insert("stale-node.example.com", hv); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if _, ok := resolver.DefaultHosts.Search("stale-node.example.com", false); !ok {
		t.Fatal("插入后应可命中 DefaultHosts")
	}
	ResetHosts()
	if _, ok := resolver.DefaultHosts.Search("stale-node.example.com", false); ok {
		t.Fatal("ResetHosts 后不应再命中残留条目")
	}
}

type stubCloseProxy struct {
	C.Proxy
	closed bool
	err    error
}

func (s *stubCloseProxy) Close() error {
	s.closed = true
	return s.err
}

func TestProxyDialerClose(t *testing.T) {
	wantErr := errors.New("close boom")
	stub := &stubCloseProxy{err: wantErr}
	var d model.Dialer = &proxyDialer{proxy: stub}
	closer, ok := d.(io.Closer)
	if !ok {
		t.Fatal("*proxyDialer 应实现 io.Closer")
	}
	if err := closer.Close(); !errors.Is(err, wantErr) || !stub.closed {
		t.Fatalf("Close() = %v (closed=%v), want %v", err, stub.closed, wantErr)
	}

	// nil proxy 不应 panic
	if err := (&proxyDialer{}).Close(); err != nil {
		t.Fatalf("空 proxy Close() = %v, want nil", err)
	}

	// 真实 mihomo 解析出的拨号器也应支持 Close
	realDialer, err := NewDialer(model.Node{
		Name: "ss-close-test",
		Raw: map[string]any{
			"name": "ss-close-test", "type": "ss", "server": "127.0.0.1", "port": 8388,
			"cipher": "aes-256-gcm", "password": "pw",
		},
	})
	if err != nil {
		t.Fatalf("NewDialer: %v", err)
	}
	if c, ok := realDialer.(io.Closer); !ok {
		t.Fatal("NewDialer 返回值应实现 io.Closer")
	} else if err := c.Close(); err != nil {
		t.Fatalf("realDialer.Close() = %v", err)
	}
}

func TestSampleIPsAAAAFallbackAndFilterFakeIP(t *testing.T) {
	origDNS, origLookup := queryDNSFn, lookupIPFn
	t.Cleanup(func() {
		queryDNSFn = origDNS
		lookupIPFn = origLookup
	})

	realV6DNS := netip.MustParseAddr("2606:4700:4700::1111")
	realV6Sys := netip.MustParseAddr("2001:4860:4860::8888")

	queryDNSFn = func(_ context.Context, srv, _ string, qtype uint16) []netip.Addr {
		if qtype == dns.TypeA {
			return nil
		}
		if srv == preflightDNServers[0] {
			return []netip.Addr{
				netip.MustParseAddr("2001:2::1"), // Fake-IPv6 应过滤
				netip.MustParseAddr("fc00::1"),   // ULA Fake-IPv6 应过滤
				realV6DNS,
			}
		}
		return nil
	}
	lookupIPFn = func(_ context.Context, network, _ string) ([]net.IP, error) {
		if network == "ip4" {
			return []net.IP{net.ParseIP("198.18.0.5")}, nil // Fake-IPv4 应过滤，导致 A 为空
		}
		if network == "ip6" {
			return []net.IP{
				net.ParseIP("fd12::1"),
				net.ParseIP("::1"),
				net.ParseIP(realV6Sys.String()),
			}, nil
		}
		return nil, nil
	}

	got := sampleIPs(context.Background(), "v6only.example.com")
	if len(got) != 2 || !slices.Contains(got, realV6DNS) || !slices.Contains(got, realV6Sys) {
		t.Fatalf("sampleIPs IPv6 回退异常: got=%v, want [%v, %v]", got, realV6DNS, realV6Sys)
	}
}

func TestProbeHostSkipsSleepWhenZeroIPs(t *testing.T) {
	origDNS, origLookup := queryDNSFn, lookupIPFn
	t.Cleanup(func() {
		queryDNSFn = origDNS
		lookupIPFn = origLookup
	})

	queryDNSFn = func(context.Context, string, string, uint16) []netip.Addr { return nil }
	lookupIPFn = func(context.Context, string, string) ([]net.IP, error) { return nil, nil }

	start := time.Now()
	got := probeHost(context.Background(), "nxdomain.example.invalid", map[uint16]struct{}{443: {}})
	elapsed := time.Since(start)
	if len(got) != 0 {
		t.Fatalf("expected nil, got %v", got)
	}
	if elapsed >= 2*time.Second {
		t.Fatalf("零 IP 时不应睡眠 6s，实际耗时 %s", elapsed)
	}
}

func TestPreflightServersIPv6OnlyTCPNodePinned(t *testing.T) {
	origDNS, origLookup, origProbe := queryDNSFn, lookupIPFn, tcpProbeFn
	t.Cleanup(func() {
		queryDNSFn = origDNS
		lookupIPFn = origLookup
		tcpProbeFn = origProbe
		ResetHosts()
	})
	ResetHosts()

	v6 := netip.MustParseAddr("2606:4700::6810:85e5")
	queryDNSFn = func(_ context.Context, srv, _ string, qtype uint16) []netip.Addr {
		if qtype == dns.TypeAAAA && srv == preflightDNServers[0] {
			return []netip.Addr{v6}
		}
		return nil
	}
	lookupIPFn = func(context.Context, string, string) ([]net.IP, error) { return nil, nil }
	tcpProbeFn = func(_ context.Context, ip netip.Addr, port uint16) bool {
		return ip == v6 && port == 443
	}

	nodes := []model.Node{{
		Name:   "v6-vless",
		Type:   "Vless",
		Server: "v6node.example.com",
		Raw:    map[string]any{"type": "vless", "server": "v6node.example.com", "port": 443},
	}}
	res := PreflightServers(context.Background(), nodes)
	if res.PinnedHosts != 1 || len(res.FailedHosts) != 0 {
		t.Fatalf("IPv6-only 节点预检应钉住，实际 %+v", res)
	}
	hv, ok := resolver.DefaultHosts.Search("v6node.example.com", false)
	if !ok {
		t.Fatal("DefaultHosts 中应存在 v6node.example.com")
	}
	if !slices.Contains(hv.IPs, v6) {
		t.Fatalf("DefaultHosts 钉住的 IPv6 异常: ips=%v", hv.IPs)
	}
}
