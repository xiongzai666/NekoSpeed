package tester

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/stun"

	"nekospeed/internal/model"
)

// directDialer 用 net.Dialer 做 DIRECT 拨号，模拟 model.Dialer。
func directDialer() model.Dialer {
	nd := &net.Dialer{Timeout: 5 * time.Second}
	return model.DialFunc(nd.DialContext)
}

// newPingServer 返回模拟 generate_204 的测试服务器。
func newPingServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Millisecond) // 人为制造可测量的延迟
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newSpeedServer 返回流式输出大文件的测试服务器。
func newSpeedServer(t *testing.T) *httptest.Server {
	t.Helper()
	chunk := make([]byte, 64*1024)
	for i := range chunk {
		chunk[i] = byte(i)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", 64*1024*1024)) // 64MB，测速窗口内读不完
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for {
			select {
			case <-r.Context().Done():
				return
			default:
			}
			if _, err := w.Write(chunk); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLatencyDirect(t *testing.T) {
	srv := newPingServer(t)
	res := Latency(context.Background(), directDialer(), srv.URL, 3)

	if res.AvgHTTPS <= 0 {
		t.Fatalf("AvgHTTPS 应为非零正值，实际 %d（loss=%v）", res.AvgHTTPS, res.PacketLoss)
	}
	if len(res.HTTPSPing) != 3 || len(res.TLSRTT) != 3 {
		t.Fatalf("成功 3 次应有 3 条记录，实际 HTTPSPing=%d TLSRTT=%d", len(res.HTTPSPing), len(res.TLSRTT))
	}
	if res.PacketLoss != 0 {
		t.Fatalf("本地直连不应丢包，实际 %v", res.PacketLoss)
	}
	if res.HTTPCode != http.StatusNoContent {
		t.Fatalf("HTTPCode 应为 204，实际 %d", res.HTTPCode)
	}
}

func TestLatencyAllFailed(t *testing.T) {
	// 指向一个不可能成功的地址，验证全失败时 Avg 为 0、丢包率为 1
	dialer := model.DialFunc(func(context.Context, string, string) (net.Conn, error) {
		return nil, fmt.Errorf("dial refused")
	})
	res := Latency(context.Background(), dialer, "http://example.invalid/generate_204", 2)
	if res.AvgHTTPS != 0 || res.AvgTLSRTT != 0 {
		t.Fatalf("全部失败时 Avg 应为 0，实际 HTTPS=%d TLS=%d", res.AvgHTTPS, res.AvgTLSRTT)
	}
	if res.PacketLoss != 1 {
		t.Fatalf("丢包率应为 1，实际 %v", res.PacketLoss)
	}
}

func TestSpeedDirect(t *testing.T) {
	srv := newSpeedServer(t)
	const duration = 3
	res := Speed(context.Background(), directDialer(), srv.URL, 4, duration)

	if len(res.PerSecond) != duration {
		t.Fatalf("PerSecond 长度应为 %d，实际 %d", duration, len(res.PerSecond))
	}
	if res.Avg <= 0 {
		t.Fatalf("Avg 应为正值，实际 %v", res.Avg)
	}
	if res.Max <= 0 || res.Max < res.Avg {
		t.Fatalf("Max 应 >= Avg 且为正，实际 Max=%v Avg=%v", res.Max, res.Avg)
	}
	if res.Threads != 4 {
		t.Fatalf("Threads 应为 4，实际 %d", res.Threads)
	}
}

func TestSpeedBadURLFallback(t *testing.T) {
	// 单一坏 URL：不应 panic / 挂死，结果为 0
	dialer := model.DialFunc(func(context.Context, string, string) (net.Conn, error) {
		return nil, fmt.Errorf("dial refused")
	})
	done := make(chan *model.SpeedResult, 1)
	go func() {
		done <- Speed(context.Background(), dialer, "http://example.invalid/file.bin", 2, 2)
	}()
	select {
	case res := <-done:
		if res.Avg != 0 {
			t.Fatalf("全失败时 Avg 应为 0，实际 %v", res.Avg)
		}
		if len(res.PerSecond) != 2 {
			t.Fatalf("PerSecond 长度应为 2，实际 %d", len(res.PerSecond))
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Speed 在坏 URL 下挂死")
	}
}

func TestSpeedWithLimitStopsEarlyAndComputesTrueAvg(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 32*1024)
		for {
			select {
			case <-r.Context().Done():
				return
			default:
				if _, err := w.Write(buf); err != nil {
					return
				}
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
	}))
	t.Cleanup(srv.Close)

	start := time.Now()
	const capBytes = 1 << 20
	res := SpeedWithLimit(context.Background(), directDialer(), srv.URL, 2, 8, capBytes)
	elapsed := time.Since(start)

	if elapsed > 4*time.Second {
		t.Fatalf("达到流量上限后未提前结束，耗时 %s", elapsed)
	}
	if res.Avg <= 0 {
		t.Fatalf("Avg 应为正值，实际 %v", res.Avg)
	}
	if res.Max < res.Avg {
		t.Fatalf("Max(%v) 不应小于 Avg(%v)", res.Max, res.Avg)
	}
	var sum float64
	for _, v := range res.PerSecond {
		sum += v
	}
	if sum < float64(capBytes) {
		t.Fatalf("总计字节 (%v) 低于触发上限 %d", sum, capBytes)
	}
}

func TestLatencyEarlyShortCircuitAndTimeout(t *testing.T) {
	if pingTimeout != 6*time.Second {
		t.Fatalf("pingTimeout = %s, want 6s", pingTimeout)
	}

	var calls atomic.Int32
	failDialer := model.DialFunc(func(context.Context, string, string) (net.Conn, error) {
		calls.Add(1)
		return nil, fmt.Errorf("dial refused")
	})
	res := Latency(context.Background(), failDialer, "http://example.invalid/generate_204", 4)
	if calls.Load() != 2 {
		t.Fatalf("count=4 连续初始失败应熔断为 2 次调用，实际 %d", calls.Load())
	}
	if res.PacketLoss != 1.0 || res.AvgHTTPS != 0 || len(res.HTTPSPing) != 0 {
		t.Fatalf("熔断返回结果异常: %+v", res)
	}

	// 若首次成功、后续失败，不应提前熔断
	srv := newPingServer(t)
	var mixedCalls atomic.Int32
	mixedDialer := model.DialFunc(func(ctx context.Context, network, addr string) (net.Conn, error) {
		if mixedCalls.Add(1) == 1 {
			return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, network, addr)
		}
		return nil, fmt.Errorf("intermittent failure")
	})
	res2 := Latency(context.Background(), mixedDialer, srv.URL, 4)
	if mixedCalls.Load() != 4 {
		t.Fatalf("已有成功样本时应跑满 4 次，实际 %d", mixedCalls.Load())
	}
	if len(res2.HTTPSPing) != 1 || res2.PacketLoss != 0.75 {
		t.Fatalf("混合丢包结果异常: %+v", res2)
	}
}

func TestSTUNServersOrderAndDirectResolution(t *testing.T) {
	want := []string{
		"stun.miwifi.com:3478",
		"stun.hot-chilli.net:3478",
		"stunserver2025.stunprotocol.org:3478",
		"stun.voipgate.com:3478",
		"stun.l.google.com:19302",
	}
	if !slices.Equal(stunServers, want) {
		t.Fatalf("stunServers = %v, want %v", stunServers, want)
	}

	orig := resolveHostDirectFn
	t.Cleanup(func() { resolveHostDirectFn = orig })

	var called atomic.Bool
	resolveHostDirectFn = func(_ context.Context, host string) string {
		called.Store(true)
		if host == "stun.miwifi.com" {
			return "203.0.113.55"
		}
		return ""
	}

	litAddr, err := resolveSTUNAddr(context.Background(), "127.0.0.1:3478")
	if err != nil || litAddr.IP.String() != "127.0.0.1" || litAddr.Port != 3478 {
		t.Fatalf("字面量 IP 解析异常: addr=%v err=%v", litAddr, err)
	}
	if called.Load() {
		t.Fatal("字面量 IP 不应触发 ResolveHostDirect")
	}

	hostAddr, err := resolveSTUNAddr(context.Background(), "stun.miwifi.com:3478")
	if err != nil || hostAddr.IP.String() != "203.0.113.55" || hostAddr.Port != 3478 {
		t.Fatalf("域名直连解析异常: addr=%v err=%v", hostAddr, err)
	}
	if !called.Load() {
		t.Fatal("域名解析应通过 ResolveHostDirect 绕过 Fake-IP")
	}
}

type mockUDPDialer struct{}

func (mockUDPDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, addr)
}

func (mockUDPDialer) ListenPacket(_ context.Context, _ string) (net.PacketConn, error) {
	return net.ListenPacket("udp", "127.0.0.1:0")
}

func (mockUDPDialer) SupportsUDP() bool { return true }

type stunServerBehavior struct {
	mappedPort int
	hasOther   bool
	useChanged bool
	allowTest2 bool
	allowTest3 bool
}

func startMockSTUNServer(t *testing.T, b stunServerBehavior) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })

	go func() {
		buf := make([]byte, 1500)
		for {
			n, raddr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			req := &stun.Message{}
			if _, err := req.Write(buf[:n]); err != nil {
				continue
			}
			var flags byte
			if v, err := req.Get(stun.AttrChangeRequest); err == nil && len(v) == 4 {
				flags = v[3]
			}
			if flags == (changeIP|changePort) && !b.allowTest2 {
				continue
			}
			if flags == changePort && !b.allowTest3 {
				continue
			}

			resp := stun.MustBuild(req, stun.BindingSuccess, &stun.XORMappedAddress{
				IP:   net.ParseIP("198.51.100.10"),
				Port: b.mappedPort,
			})
			if b.hasOther {
				otherIP := net.ParseIP("198.51.100.99")
				if b.useChanged {
					_ = (&stun.MappedAddress{IP: otherIP, Port: 3479}).AddToAs(resp, stun.AttrChangedAddress)
				} else {
					_ = (&stun.OtherAddress{IP: otherIP, Port: 3479}).AddTo(resp)
				}
			}
			resp.Encode()
			_, _ = pc.WriteTo(resp.Raw, raddr)
		}
	}()
	return pc.LocalAddr().String()
}

func TestNATDetectionRFC5780(t *testing.T) {
	origServers := stunServers
	origTimeout := stunAttemptTimeout
	t.Cleanup(func() {
		stunServers = origServers
		stunAttemptTimeout = origTimeout
	})
	stunAttemptTimeout = 80 * time.Millisecond

	t.Run("FullCone", func(t *testing.T) {
		s1 := startMockSTUNServer(t, stunServerBehavior{mappedPort: 40001, hasOther: true, allowTest2: true})
		stunServers = []string{s1}
		if got := NAT(context.Background(), mockUDPDialer{}); got != NATFullCone {
			t.Fatalf("got %q, want %q", got, NATFullCone)
		}
	})

	t.Run("RestrictedCone", func(t *testing.T) {
		s1 := startMockSTUNServer(t, stunServerBehavior{mappedPort: 40001, hasOther: true, allowTest3: true})
		s2 := startMockSTUNServer(t, stunServerBehavior{mappedPort: 40001})
		stunServers = []string{s1, s2}
		if got := NAT(context.Background(), mockUDPDialer{}); got != NATRestrictedCone {
			t.Fatalf("got %q, want %q", got, NATRestrictedCone)
		}
	})

	t.Run("PortRestrictedCone", func(t *testing.T) {
		s1 := startMockSTUNServer(t, stunServerBehavior{mappedPort: 40001, hasOther: true})
		s2 := startMockSTUNServer(t, stunServerBehavior{mappedPort: 40001})
		stunServers = []string{s1, s2}
		if got := NAT(context.Background(), mockUDPDialer{}); got != NATPortRestrictedCone {
			t.Fatalf("got %q, want %q", got, NATPortRestrictedCone)
		}
	})

	t.Run("ChangedAddressFallback", func(t *testing.T) {
		s1 := startMockSTUNServer(t, stunServerBehavior{mappedPort: 40001, hasOther: true, useChanged: true, allowTest3: true})
		s2 := startMockSTUNServer(t, stunServerBehavior{mappedPort: 40001})
		stunServers = []string{s1, s2}
		if got := NAT(context.Background(), mockUDPDialer{}); got != NATRestrictedCone {
			t.Fatalf("got %q, want %q", got, NATRestrictedCone)
		}
	})

	t.Run("PreferServerWithOtherAddress", func(t *testing.T) {
		sNoOther := startMockSTUNServer(t, stunServerBehavior{mappedPort: 40001, hasOther: false})
		sWithOther := startMockSTUNServer(t, stunServerBehavior{mappedPort: 40001, hasOther: true, allowTest2: true})
		stunServers = []string{sNoOther, sWithOther}
		if got := NAT(context.Background(), mockUDPDialer{}); got != NATFullCone {
			t.Fatalf("应优先选用返回 OTHER-ADDRESS 的服务器作为 primary，实际 %q", got)
		}
	})

	t.Run("NoOtherAddress_SameMappingReturnsUnknown", func(t *testing.T) {
		s1 := startMockSTUNServer(t, stunServerBehavior{mappedPort: 40001, hasOther: false})
		s2 := startMockSTUNServer(t, stunServerBehavior{mappedPort: 40001, hasOther: false})
		stunServers = []string{s1, s2}
		if got := NAT(context.Background(), mockUDPDialer{}); got != NATUnknown {
			t.Fatalf("无 OTHER-ADDRESS 时不得误判为 PortRestrictedCone，got %q, want %q", got, NATUnknown)
		}
	})

	t.Run("NoOtherAddress_SymmetricConfirmed", func(t *testing.T) {
		s1 := startMockSTUNServer(t, stunServerBehavior{mappedPort: 40001, hasOther: false})
		s2 := startMockSTUNServer(t, stunServerBehavior{mappedPort: 40002, hasOther: false})
		stunServers = []string{s1, s2}
		if got := NAT(context.Background(), mockUDPDialer{}); got != NATSymmetric {
			t.Fatalf("即使无 OTHER-ADDRESS，第二台服务器端口不同仍应判定 Symmetric，got %q", got)
		}
	})
}
