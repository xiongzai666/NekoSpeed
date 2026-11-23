package runner

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"nekospeed/internal/geo"
	"nekospeed/internal/model"
	"nekospeed/internal/report"
	"nekospeed/internal/subscription"
)

func TestFormatLocalISP(t *testing.T) {
	cases := []struct {
		g    model.GeoInfo
		want string
	}{
		{model.GeoInfo{City: "上海", ISP: "China Telecom"}, "上海电信"},
		{model.GeoInfo{City: "深圳", ISP: "China Unicom"}, "深圳联通"},
		{model.GeoInfo{Region: "陕西", ISP: "China Mobile"}, "陕西移动"},
		{model.GeoInfo{Region: "安徽省", ISP: "中国移动"}, "安徽移动"},
		{model.GeoInfo{City: "安庆桐城", ISP: "中国移动"}, "安庆桐城移动"},
		{model.GeoInfo{City: "北京", ISP: "中国电信"}, "北京电信"},
		{model.GeoInfo{City: "广州", ISP: "China Broadcasting Network"}, "广州广电"},
		{model.GeoInfo{City: "上海", ISP: ""}, "上海"},
		{model.GeoInfo{}, ""},
	}
	for _, c := range cases {
		got := formatLocalISP(c.g)
		if got != c.want {
			t.Errorf("formatLocalISP(%+v) = %q, want %q", c.g, got, c.want)
		}
	}
}

func TestMaxBandwidth(t *testing.T) {
	cases := []struct {
		results  []*model.NodeResult
		override string
		want     string
	}{
		{nil, "1Gbps", "1Gbps"},
		{nil, "", ""},
		{[]*model.NodeResult{{Speed: &model.SpeedResult{Max: 125_000_000}}}, "", "1Gbps"},
		{[]*model.NodeResult{{Speed: &model.SpeedResult{Max: 62_500_000}}}, "", "500Mbps"},
		{[]*model.NodeResult{{Speed: &model.SpeedResult{Max: 250_000_000}}}, "", "2Gbps"},
		{[]*model.NodeResult{{Speed: &model.SpeedResult{Max: 1}}}, "", "<1Mbps"},
	}
	for _, c := range cases {
		got := maxBandwidth(c.results, c.override)
		if got != c.want {
			t.Errorf("maxBandwidth(%v, %q) = %q, want %q", c.results, c.override, got, c.want)
		}
	}
}

func TestIsInfoNodeName(t *testing.T) {
	infoNames := []string{
		"剩余流量：104.04 GB",
		"套餐到期：长期有效",
		"距离下次重置剩余：12 天",
		"过滤掉 2 个超时节点",
		"官网：https://example.com",
		"最新网址发布页",
		"到期: 2027-10-14",
		"重置: 2026-12-14",
		"获取: 2026-11-22 10:41:08",
		"流量: 11.25 GB / 200 GB",
	}
	for _, name := range infoNames {
		if !isInfoNodeName(name) {
			t.Errorf("isInfoNodeName(%q) = false, want true", name)
		}
	}
	realNames := []string{
		"🇭🇰 香港1|1x",
		"🇭🇰 香港-01",
		"🇯🇵 日本IPLC-01",
		"🇺🇸 美国BGP专线",
		"🇸🇬 狮城 0.5x流量",
	}
	for _, name := range realNames {
		if isInfoNodeName(name) {
			t.Errorf("isInfoNodeName(%q) = true, want false", name)
		}
	}
}

func TestNodeResultOKAndSort(t *testing.T) {
	// 仅有本地 Inbound 解析、无任何代理探测成功的死节点，OK() 必须为 false
	dead := &model.NodeResult{
		Node:    model.Node{Index: 1, Name: "dead"},
		Inbound: &model.GeoInfo{IP: "1.2.3.4"},
		Media:   []model.UnlockResult{{Platform: "youtube", Status: model.StatusReqFailed}},
	}
	if dead.OK() {
		t.Fatal("仅含本地 Inbound 与 ReqFailed 的死节点 OK() 应为 false")
	}

	fast := &model.NodeResult{
		Node:    model.Node{Index: 2, Name: "fast"},
		Latency: &model.LatencyResult{AvgHTTPS: 120},
		Speed:   &model.SpeedResult{Avg: 50_000_000},
	}
	lowLat := &model.NodeResult{
		Node:    model.Node{Index: 3, Name: "lowLat"},
		Latency: &model.LatencyResult{AvgHTTPS: 40},
		Speed:   &model.SpeedResult{Avg: 10_000_000},
	}

	list := []*model.NodeResult{dead, lowLat, fast}
	if label := sortResults(list, "speed"); label != "速度降序" {
		t.Fatalf("sortResults speed label = %q", label)
	}
	if list[0].Node.Name != "fast" || list[2].Node.Name != "dead" {
		t.Fatalf("按速度降序结果异常: %s, %s, %s", list[0].Node.Name, list[1].Node.Name, list[2].Node.Name)
	}

	if label := sortResults(list, "latency"); label != "延迟升序" {
		t.Fatalf("sortResults latency label = %q", label)
	}
	if list[0].Node.Name != "lowLat" || list[2].Node.Name != "dead" {
		t.Fatalf("按延迟升序结果异常: %s, %s, %s", list[0].Node.Name, list[1].Node.Name, list[2].Node.Name)
	}

	if label := sortResults(list, ""); label != "订阅原序" {
		t.Fatalf("sortResults none label = %q", label)
	}
	if list[0].Node.Name != "dead" || list[1].Node.Name != "fast" || list[2].Node.Name != "lowLat" {
		t.Fatalf("恢复订阅原序异常: %s, %s, %s", list[0].Node.Name, list[1].Node.Name, list[2].Node.Name)
	}
}

func TestBuildTopologyBeforeSortPreservesOriginalClusterOrder(t *testing.T) {
	n0 := &model.NodeResult{
		Node:     model.Node{Index: 0, Name: "n0"},
		Inbound:  &model.GeoInfo{IP: "1.1.1.1", Stack: "4"},
		Outbound: &model.GeoInfo{IP: "9.9.9.9", Stack: "4"},
		Speed:    &model.SpeedResult{Avg: 10},
	}
	n1 := &model.NodeResult{
		Node:     model.Node{Index: 1, Name: "n1"},
		Inbound:  &model.GeoInfo{IP: "2.2.2.2", Stack: "4"},
		Outbound: &model.GeoInfo{IP: "8.8.8.8", Stack: "4"},
		Speed:    &model.SpeedResult{Avg: 100},
	}
	list := []*model.NodeResult{n0, n1}
	topo := geo.BuildTopology(list)
	sortResults(list, "speed")
	if list[0].Node.Name != "n1" {
		t.Fatalf("预期按速度排序后 n1 在首位")
	}
	if len(topo.Outbounds) != 2 || topo.Outbounds[0].NodeName != "n0" || topo.Outbounds[0].Cluster != 1 || topo.Outbounds[1].Cluster != 2 {
		t.Fatalf("排序前计算的拓扑簇顺序异常: %+v", topo.Outbounds)
	}
}

func TestReRenderRestoresMetaAndBareStamp(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().Truncate(time.Second)
	res := []*model.NodeResult{
		{
			Node:     model.Node{Index: 0, Name: "HK-1", Type: "ss", Server: "1.1.1.1"},
			Outbound: &model.GeoInfo{IP: "203.0.113.1", Country: "HK", Stack: "4"},
			Media:    []model.UnlockResult{{Platform: "youtube", Status: model.StatusUnlock, Region: "HK"}},
		},
	}
	topo := geo.BuildTopology(res)
	meta := report.RunMeta{
		Version:   "0.2.0",
		StartTime: now,
		Elapsed:   2 * time.Second,
		SubName:   "",
		Threads:   8,
		Single:    false,
		PingCount: 6,
		Filter:    "包含:HK",
		LocalISP:  "北京电信",
		Bandwidth: "500 Mbps",
		Theme:     "midnight",
	}
	jsonPath := filepath.Join(dir, "20261113_230000_result.json")
	if err := report.SaveJSONWithMeta(res, topo, meta, jsonPath); err != nil {
		t.Fatal(err)
	}
	files, err := ReRender(dir, ReRenderOptions{SortBy: "none"})
	if err != nil {
		t.Fatalf("ReRender 失败: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("ReRender 未返回生成文件列表")
	}
	if _, err := os.Stat(filepath.Join(dir, "20261113_230000_media.png")); err != nil {
		t.Fatalf("应生成裸时间戳前缀图片: %v", err)
	}
}

type closableTestDialer struct {
	id     string
	closed atomic.Int32
}

func (d *closableTestDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	return nil, context.Canceled
}

func (d *closableTestDialer) Close() error {
	d.closed.Add(1)
	return nil
}

func TestRunUsesLoadContextWithoutMutatingGlobalProxy(t *testing.T) {
	if err := subscription.SetProxy(""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = subscription.SetProxy("") })

	var proxyHits atomic.Int32
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits.Add(1)
		w.Header().Set("Content-Type", "text/yaml")
		_, _ = w.Write([]byte("proxies:\n  - { name: 'n1', type: ss, server: 1.2.3.4, port: 8388, cipher: aes-256-gcm, password: pw }\n"))
	}))
	defer proxySrv.Close()

	// Filter 故意不匹配，在 LoadContext 之后即刻返回“没有可用节点”，验证 LoadContext 走了 o.Proxy 且未污染全局代理
	_, err := Run(context.Background(), Options{
		Source:    "http://sub.invalid/runner-proxy.yaml",
		Proxy:     proxySrv.URL,
		Filter:    "definitely-no-match-xyz",
		Preflight: false,
	}, nil)
	if err == nil {
		t.Fatal("期望因无匹配节点返回错误")
	}
	if proxyHits.Load() != 1 {
		t.Fatalf("Run 应通过 LoadContext 经代理拉取 1 次，实际 %d", proxyHits.Load())
	}

	// 全局未设置代理时，Load 同一无效域名不应再命中 proxySrv
	if _, _, err := subscription.Load("http://sub.invalid/runner-proxy.yaml"); err == nil {
		t.Fatal("全局代理不应被 Run 修改")
	}
	if proxyHits.Load() != 1 {
		t.Fatalf("Load 不应再命中此前 Run 的每调用代理，hits=%d", proxyHits.Load())
	}
}

func TestRunNodesDecouplesSemFromSpeedSemAndClosesDialer(t *testing.T) {
	origDialer, origLat, origSpeed := newDialerFn, latencyTestFn, speedTestFn
	t.Cleanup(func() {
		newDialerFn = origDialer
		latencyTestFn = origLat
		speedTestFn = origSpeed
	})

	d1 := &closableTestDialer{id: "n1"}
	d2 := &closableTestDialer{id: "n2"}
	newDialerFn = func(n model.Node) (model.Dialer, error) {
		if n.Name == "n1" {
			return d1, nil
		}
		return d2, nil
	}

	n2Probed := make(chan struct{})
	n1InSpeed := make(chan struct{})
	releaseSpeed := make(chan struct{})
	var n1SpeedOnce, n2ProbeOnce atomic.Bool

	latencyTestFn = func(_ context.Context, d model.Dialer, _ string, _ int) *model.LatencyResult {
		cd := d.(*closableTestDialer)
		if cd.id == "n2" && n2ProbeOnce.CompareAndSwap(false, true) {
			close(n2Probed)
		}
		return &model.LatencyResult{AvgHTTPS: 50, HTTPSPing: []int{50}}
	}

	var activeSpeed atomic.Int32
	var maxActiveSpeed atomic.Int32
	speedTestFn = func(_ context.Context, d model.Dialer, _ string, _, _ int, _ int64) *model.SpeedResult {
		cd := d.(*closableTestDialer)
		cur := activeSpeed.Add(1)
		for {
			prev := maxActiveSpeed.Load()
			if cur <= prev || maxActiveSpeed.CompareAndSwap(prev, cur) {
				break
			}
		}
		defer activeSpeed.Add(-1)

		// 测速期间 dialer 不得提前关闭
		if cd.closed.Load() != 0 {
			t.Errorf("节点 %s 测速期间 dialer 已被关闭", cd.id)
		}
		if cd.id == "n1" && n1SpeedOnce.CompareAndSwap(false, true) {
			close(n1InSpeed)
			<-releaseSpeed
		}
		return &model.SpeedResult{Avg: 1000, Max: 1000, PerSecond: []float64{1000}}
	}

	doneCh := make(chan []*model.NodeResult, 1)
	go func() {
		nodes := []model.Node{
			{Index: 1, Name: "n1"},
			{Index: 2, Name: "n2"},
		}
		doneCh <- runNodes(context.Background(), nodes, []string{"speed"}, Options{
			Concurrency: 1, // 故意设为 1：若未解耦 sem，n1 阻塞在 speedTestFn 时 n2 绝对无法开始延迟探测
			Threads:     1,
			Duration:    1,
			PingCount:   1,
		}, nil, nil, nil)
	}()

	select {
	case <-n1InSpeed:
	case <-time.After(3 * time.Second):
		t.Fatal("等待 n1 进入测速超时")
	}

	select {
	case <-n2Probed:
	case <-time.After(2 * time.Second):
		close(releaseSpeed)
		t.Fatal("Concurrency=1 时 n1 进入测速后应已释放 sem，n2 未能并发进入延迟探测")
	}

	close(releaseSpeed)

	var results []*model.NodeResult
	select {
	case results = <-doneCh:
	case <-time.After(3 * time.Second):
		t.Fatal("runNodes 结束超时")
	}

	if maxActiveSpeed.Load() != 1 {
		t.Fatalf("串行测速最大并发应为 1，实际 %d", maxActiveSpeed.Load())
	}
	if d1.closed.Load() != 1 || d2.closed.Load() != 1 {
		t.Fatalf("每个节点结束后 dialer.Close() 应各调用 1 次，实际 d1=%d d2=%d", d1.closed.Load(), d2.closed.Load())
	}
	if len(results) != 2 || results[0].Speed == nil || results[1].Speed == nil {
		t.Fatalf("节点测速结果异常: %+v", results)
	}
}

func TestTestNodeClosesDialerAndReleasesSemOnAllPaths(t *testing.T) {
	origDialer, origLat, origNAT := newDialerFn, latencyTestFn, natTestFn
	t.Cleanup(func() {
		newDialerFn = origDialer
		latencyTestFn = origLat
		natTestFn = origNAT
	})

	t.Run("EarlyReturnOnPacketLoss", func(t *testing.T) {
		d := &closableTestDialer{id: "dead"}
		newDialerFn = func(model.Node) (model.Dialer, error) { return d, nil }
		latencyTestFn = func(context.Context, model.Dialer, string, int) *model.LatencyResult {
			return &model.LatencyResult{PacketLoss: 1.0, AvgHTTPS: 0}
		}
		res := runNodes(context.Background(), []model.Node{{Index: 1, Name: "dead"}}, []string{"speed"}, Options{Concurrency: 1}, nil, nil, nil)
		if d.closed.Load() != 1 {
			t.Fatalf("丢包熔断路径应关闭 dialer 1 次，实际 %d", d.closed.Load())
		}
		if res[0].Err == "" {
			t.Fatal("丢包熔断应写入 Err")
		}
	})

	t.Run("CancelWaitingSpeedSem", func(t *testing.T) {
		d := &closableTestDialer{id: "wait-speed"}
		newDialerFn = func(model.Node) (model.Dialer, error) { return d, nil }
		latencyTestFn = func(context.Context, model.Dialer, string, int) *model.LatencyResult {
			return &model.LatencyResult{AvgHTTPS: 20, HTTPSPing: []int{20}}
		}
		ctx, cancel := context.WithCancel(context.Background())
		speedSem := make(chan struct{}, 1)
		speedSem <- struct{}{} // 占满 speedSem

		var relCount atomic.Int32
		done := make(chan struct{})
		go func() {
			defer close(done)
			testNode(ctx, model.Node{Index: 1, Name: "wait"}, &model.NodeResult{}, []string{"speed"}, testOpts{
				speedSem: speedSem,
				releaseProbe: func() {
					relCount.Add(1)
				},
			})
		}()
		time.Sleep(30 * time.Millisecond)
		if relCount.Load() != 1 {
			t.Fatalf("等待 speedSem 前应已释放 probe sem，relCount=%d", relCount.Load())
		}
		cancel()
		<-done
		if d.closed.Load() != 1 {
			t.Fatalf("等待 speedSem 取消退出时应关闭 dialer 1 次，实际 %d", d.closed.Load())
		}
	})
}
