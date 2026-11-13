package nodepool

import (
	"context"
	"net/netip"
	"testing"

	C "github.com/metacubex/mihomo/constant"
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
