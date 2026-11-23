package geo

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"nekospeed/internal/model"
)

func gi(ip, country, asn, org string) *model.GeoInfo {
	return &model.GeoInfo{IP: ip, Country: country, ASN: asn, Org: org, Stack: stackOf(ip)}
}

func TestBuildTopology(t *testing.T) {
	node := func(name string) model.Node { return model.Node{Name: name, Server: "srv-" + name} }
	results := []*model.NodeResult{
		{Node: node("A"), Inbound: gi("1.1.1.1", "HK", "AS1", "isp1"), Outbound: gi("2.2.2.2", "JP", "AS2", "isp2")},
		{Node: node("B"), Inbound: gi("1.1.1.1", "HK", "AS1", "isp1"), Outbound: gi("2.2.2.3", "JP", "AS2", "isp2")},
		{Node: node("C"), Inbound: gi("3.3.3.3", "SG", "AS3", "isp3"), Outbound: gi("2.2.2.2", "JP", "AS2", "isp2")},
		{Node: node("D"), Inbound: gi("1.1.1.1", "HK", "AS1", "isp1"), Outbound: gi("9.9.9.9", "US", "AS9", "isp9")},
		{Node: node("E"), Inbound: gi("3.3.3.3", "SG", "AS3", "isp3")}, // 无出口
		{Node: node("F")}, // 无入口，打断簇
		{Node: node("G"), Inbound: gi("3.3.3.3", "SG", "AS3", "isp3"), Outbound: gi("2.2.2.2", "JP", "AS2", "isp2")},
	}

	topo := BuildTopology(results)
	if topo == nil {
		t.Fatal("BuildTopology 返回 nil")
	}

	// 入口：按首次出现顺序编号，Cluster 取首次出现的簇号
	if len(topo.Inbounds) != 2 {
		t.Fatalf("入口数 = %d, 期望 2", len(topo.Inbounds))
	}
	in1, in2 := topo.Inbounds[0], topo.Inbounds[1]
	if in1.Index != 1 || in1.Geo.IP != "1.1.1.1" || in1.Cluster != 1 || in1.OutCount != 3 {
		t.Errorf("入口1 = %+v, 期望 {Index:1 IP:1.1.1.1 Cluster:1 OutCount:3}", in1)
	}
	if in2.Index != 2 || in2.Geo.IP != "3.3.3.3" || in2.Cluster != 2 || in2.OutCount != 2 {
		t.Errorf("入口2 = %+v, 期望 {Index:2 IP:3.3.3.3 Cluster:2 OutCount:2}", in2)
	}

	// 出口：按出口 IP 首次出现顺序（2.2.2.2 < 2.2.2.3 < 9.9.9.9）稳定排列
	type want struct {
		name    string
		inIdx   int
		cluster int
		outIP   string
	}
	wants := []want{
		{"A", 1, 1, "2.2.2.2"},
		{"C", 2, 2, "2.2.2.2"},
		{"G", 2, 5, "2.2.2.2"}, // E(3.3.3.3) 开簇 4，F 打断后 G 再开簇 5
		{"B", 1, 1, "2.2.2.3"},
		{"D", 1, 3, "9.9.9.9"}, // 断开后再用 1.1.1.1，新簇 3
	}
	if len(topo.Outbounds) != len(wants) {
		t.Fatalf("出口数 = %d, 期望 %d", len(topo.Outbounds), len(wants))
	}
	for i, w := range wants {
		o := topo.Outbounds[i]
		if o.NodeName != w.name || o.InboundIndex != w.inIdx || o.Cluster != w.cluster || o.Geo.IP != w.outIP {
			t.Errorf("出口[%d] = %+v, 期望 {Name:%s InboundIndex:%d Cluster:%d IP:%s}",
				i, o, w.name, w.inIdx, w.cluster, w.outIP)
		}
	}
}

func TestBuildTopologyStackMerge(t *testing.T) {
	results := []*model.NodeResult{
		{Node: model.Node{Name: "v4"}, Inbound: &model.GeoInfo{IP: "1.1.1.1", Stack: "4"}},
		{Node: model.Node{Name: "v6"}, Inbound: &model.GeoInfo{IP: "1.1.1.1", Stack: "6"}},
	}
	topo := BuildTopology(results)
	if len(topo.Inbounds) != 1 {
		t.Fatalf("入口数 = %d, 期望 1", len(topo.Inbounds))
	}
	if topo.Inbounds[0].Geo.Stack != "46" {
		t.Errorf("Stack = %q, 期望 %q", topo.Inbounds[0].Geo.Stack, "46")
	}
	// 同栈不合并
	results[1].Inbound.Stack = "4"
	if got := BuildTopology(results).Inbounds[0].Geo.Stack; got != "4" {
		t.Errorf("Stack = %q, 期望 %q", got, "4")
	}
}

func TestBuildTopologyEmpty(t *testing.T) {
	topo := BuildTopology(nil)
	if topo == nil || len(topo.Inbounds) != 0 || len(topo.Outbounds) != 0 {
		t.Errorf("空输入应返回空 Topology, 得到 %+v", topo)
	}
}

// ipAPIServer 返回一个仿 ip-api.com 的 httptest 服务器与请求计数器。
// 路径 /json/{ip} 返回该 IP 的归属；/json/ 返回代理出口自身信息。
func ipAPIServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Query().Get("fields") != ipAPIFields {
			http.Error(w, "bad fields", http.StatusBadRequest)
			return
		}
		ip := r.URL.Path[len("/json/"):]
		if ip == "" {
			ip = "203.0.113.7" // 模拟代理落地 IP
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"success","countryCode":"HK","query":%q,"as":"AS13335 Cloudflare Inc","org":""}`, ip)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func withBaseURL(t *testing.T, base string) {
	t.Helper()
	old := ipAPIBaseURL
	ipAPIBaseURL = base
	t.Cleanup(func() { ipAPIBaseURL = old })
}

func TestLookupOnlineAndCache(t *testing.T) {
	var hits atomic.Int32
	srv := ipAPIServer(t, &hits)
	withBaseURL(t, srv.URL)

	r, err := NewResolver("")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ctx := context.Background()
	g := r.Lookup(ctx, "1.2.3.4", nil)
	if g.IP != "1.2.3.4" || g.Country != "HK" || g.ASN != "AS13335" || g.Org != "Cloudflare Inc" || g.Stack != "4" {
		t.Errorf("Lookup = %+v", g)
	}
	// 第二次命中缓存，不再发请求
	g2 := r.Lookup(ctx, "1.2.3.4", nil)
	if g2 != g {
		t.Errorf("缓存结果不一致: %+v vs %+v", g2, g)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("请求次数 = %d, 期望 1（缓存去重）", n)
	}
}

func TestLookupConcurrentDedup(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(50 * time.Millisecond) // 放大并发窗口
		fmt.Fprint(w, `{"status":"success","countryCode":"HK","query":"5.6.7.8","as":"AS13335 Cloudflare Inc","org":""}`)
	}))
	t.Cleanup(srv.Close)
	withBaseURL(t, srv.URL)

	r, _ := NewResolver("")
	defer r.Close()

	var wg sync.WaitGroup
	outs := make([]model.GeoInfo, 32)
	for i := range outs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			outs[i] = r.Lookup(context.Background(), "5.6.7.8", nil)
		}(i)
	}
	wg.Wait()
	if n := hits.Load(); n != 1 {
		t.Errorf("并发相同 IP 请求次数 = %d, 期望 1（inflight 去重）", n)
	}
	for i, g := range outs {
		if g.IP != "5.6.7.8" || g.ASN != "AS13335" {
			t.Errorf("outs[%d] = %+v", i, g)
		}
	}
}

func TestLookupStackV6(t *testing.T) {
	var hits atomic.Int32
	srv := ipAPIServer(t, &hits)
	withBaseURL(t, srv.URL)
	r, _ := NewResolver("")
	defer r.Close()
	g := r.Lookup(context.Background(), "2001:db8::1", nil)
	if g.Stack != "6" {
		t.Errorf("Stack = %q, 期望 %q", g.Stack, "6")
	}
}

func TestLookupLocalNetwork(t *testing.T) {
	var hits atomic.Int32
	srv := ipAPIServer(t, &hits)
	withBaseURL(t, srv.URL)

	r, _ := NewResolver("")
	defer r.Close()

	g := r.LookupLocalNetwork(context.Background())
	if g.IP != "203.0.113.7" || g.Country != "HK" {
		t.Errorf("LookupLocalNetwork = %+v", g)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("请求次数 = %d, 期望 1", n)
	}
}

func TestResolveNodeOnline(t *testing.T) {
	var hits atomic.Int32
	srv := ipAPIServer(t, &hits)
	withBaseURL(t, srv.URL)

	r, _ := NewResolver("")
	defer r.Close()

	// 假代理拨号器：记录调用并直连 httptest 服务器
	var dialed atomic.Int32
	dialer := model.DialFunc(func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialed.Add(1)
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	})

	node := model.Node{Name: "n1", Server: "93.184.216.34"} // 字面量 IP，免 DNS
	in, out := r.ResolveNode(context.Background(), node, dialer)

	if in == nil || in.IP != "93.184.216.34" || in.Country != "HK" {
		t.Errorf("入口 = %+v", in)
	}
	if out == nil || out.IP != "203.0.113.7" || out.ASN != "AS13335" {
		t.Errorf("出口 = %+v", out)
	}
	if dialed.Load() == 0 {
		t.Error("出口查询未经过代理 dialer")
	}
}

func TestResolveNodeIPInfoFallback(t *testing.T) {
	// ip-api 始终失败，应回退 ipinfo
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"fail","message":"reserved range"}`)
	}))
	t.Cleanup(failSrv.Close)
	withBaseURL(t, failSrv.URL)

	infoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ip":"198.51.100.9","country":"JP","org":"AS2516 KDDI CORPORATION"}`)
	}))
	t.Cleanup(infoSrv.Close)
	oldInfo := ipinfoURL
	ipinfoURL = infoSrv.URL
	t.Cleanup(func() { ipinfoURL = oldInfo })

	r, _ := NewResolver("")
	defer r.Close()
	dialer := model.DialFunc(func(ctx context.Context, network, addr string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	})

	// 入口：ip-api 失败 → 降级结果（仅 IP/Stack）
	in, out := r.ResolveNode(context.Background(), model.Node{Name: "n", Server: "10.0.0.1"}, dialer)
	if in == nil || in.IP != "10.0.0.1" || in.Country != "" {
		t.Errorf("入口降级结果 = %+v", in)
	}
	if out == nil || out.IP != "198.51.100.9" || out.ASN != "AS2516" || out.Org != "KDDI CORPORATION" || out.Country != "JP" {
		t.Errorf("ipinfo 回退出口 = %+v", out)
	}
}

func TestResolveNodeNoDialer(t *testing.T) {
	var hits atomic.Int32
	srv := ipAPIServer(t, &hits)
	withBaseURL(t, srv.URL)
	r, _ := NewResolver("")
	defer r.Close()

	in, out := r.ResolveNode(context.Background(), model.Node{Name: "n", Server: "8.8.8.8"}, nil)
	if in == nil || in.IP != "8.8.8.8" {
		t.Errorf("入口 = %+v", in)
	}
	if out != nil {
		t.Errorf("无 dialer 时出口应为 nil, 得到 %+v", out)
	}
}

func TestNewResolverBadMMDB(t *testing.T) {
	if _, err := NewResolver("/nonexistent/GeoLite2-City.mmdb"); err == nil {
		t.Error("打开不存在的 mmdb 应报错")
	}
}

func TestMergeStack(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"4", "4", "4"},
		{"6", "6", "6"},
		{"4", "6", "46"},
		{"6", "4", "46"},
		{"46", "4", "46"},
		{"", "4", "4"},
		{"4", "", "4"},
	}
	for _, c := range cases {
		if got := mergeStack(c.a, c.b); got != c.want {
			t.Errorf("mergeStack(%q,%q) = %q, 期望 %q", c.a, c.b, got, c.want)
		}
	}
}

func TestRealDomesticAnhuiTongchengIP(t *testing.T) {
	r, err := NewResolver("")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 目标测试 IP：用户真实安徽安庆桐城 IP
	targetIP := "120.242.230.110"
	g := r.Lookup(ctx, targetIP, nil)
	t.Logf("实测国内 IP 结果: %+v", g)

	if g.Country != "CN" {
		t.Errorf("期望 Country=CN, 实际得到 %s", g.Country)
	}
	if !strings.Contains(g.Region, "安徽") {
		t.Errorf("期望 Region 包含 '安徽', 实际得到 %s", g.Region)
	}
	if !strings.Contains(g.City, "安庆") && !strings.Contains(g.City, "桐城") {
		t.Errorf("期望 City 包含 '安庆' 或 '桐城', 实际得到 %s", g.City)
	}
	if !strings.Contains(g.ISP, "移动") {
		t.Errorf("期望 ISP 包含 '移动', 实际得到 %s", g.ISP)
	}
}

func TestDomesticAPIsRejectOverseasIPAsCN(t *testing.T) {
	baiduSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"0","data":[{"location":"美国 加利福尼亚州","origip":"8.8.8.8"}]}`)
	}))
	t.Cleanup(baiduSrv.Close)
	pcSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ip":"8.8.8.8","pro":"","city":"","addr":"美国"}`)
	}))
	t.Cleanup(pcSrv.Close)

	oldBaidu, oldPC := baiduBaseURL, pconlineBaseURL
	baiduBaseURL, pconlineBaseURL = baiduSrv.URL, pcSrv.URL
	t.Cleanup(func() { baiduBaseURL, pconlineBaseURL = oldBaidu, oldPC })

	r, err := NewResolver("")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ctx := context.Background()
	if _, err := r.lookupBaidu(ctx, "8.8.8.8", nil); err == nil {
		t.Error("海外 IP 不应由 lookupBaidu 误标为 CN")
	}
	if _, err := r.lookupPCOnline(ctx, "8.8.8.8", nil); err == nil {
		t.Error("海外 IP 不应由 lookupPCOnline 误标为 CN")
	}
}

func TestResolveOutboundCFTraceSkipsDomesticAndCachesMeta(t *testing.T) {
	var domesticHits, metaHits atomic.Int32
	domesticSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		domesticHits.Add(1)
		http.Error(w, "unexpected domestic call", http.StatusInternalServerError)
	}))
	t.Cleanup(domesticSrv.Close)

	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(failSrv.Close)

	cfSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ip=198.51.100.42\nloc=JP\ncolo=NRT\n")
	}))
	t.Cleanup(cfSrv.Close)

	whoisSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metaHits.Add(1)
		fmt.Fprint(w, `{"ip":"198.51.100.42","success":true,"country_code":"JP","region":"Tokyo","city":"Tokyo","connection":{"asn":2516,"org":"KDDI","isp":"KDDI CORPORATION"}}`)
	}))
	t.Cleanup(whoisSrv.Close)

	oldCZ, oldBD, oldPC, oldMir6, oldCF, oldWhois := cz88BaseURL, baiduBaseURL, pconlineBaseURL, mir6BaseURL, cfTraceURL, ipwhoisBaseURL
	oldAPI, oldIPSB, oldGeoJS := ipAPIBaseURL, ipsbBaseURL, geojsBaseURL
	cz88BaseURL, baiduBaseURL, pconlineBaseURL, mir6BaseURL = domesticSrv.URL, domesticSrv.URL, domesticSrv.URL, domesticSrv.URL
	cfTraceURL, ipwhoisBaseURL = cfSrv.URL, whoisSrv.URL
	ipAPIBaseURL, ipsbBaseURL, geojsBaseURL = failSrv.URL, failSrv.URL, failSrv.URL
	t.Cleanup(func() {
		cz88BaseURL, baiduBaseURL, pconlineBaseURL, mir6BaseURL = oldCZ, oldBD, oldPC, oldMir6
		cfTraceURL, ipwhoisBaseURL = oldCF, oldWhois
		ipAPIBaseURL, ipsbBaseURL, geojsBaseURL = oldAPI, oldIPSB, oldGeoJS
	})

	r, err := NewResolver("")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	dialer := model.DialFunc(func(ctx context.Context, network, addr string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	})

	g1 := r.resolveOutbound(context.Background(), dialer)
	g2 := r.resolveOutbound(context.Background(), dialer)
	if g1 == nil || g1.Country != "JP" || g1.ASN != "AS2516" || g1.City != "Tokyo" {
		t.Fatalf("resolveOutbound(1) = %+v", g1)
	}
	if g2 == nil || g2.Country != "JP" || g2.ASN != "AS2516" {
		t.Fatalf("resolveOutbound(2) = %+v", g2)
	}
	if domesticHits.Load() != 0 {
		t.Errorf("非 CN CF 落地不应请求国内 GeoIP 接口, 实际请求 %d 次", domesticHits.Load())
	}
	if metaHits.Load() != 1 {
		t.Errorf("同出口 IP 元数据补全应命中缓存去重, 实际请求 %d 次", metaHits.Load())
	}
}

func TestResolveOutboundCFTraceDoesNotLeakColoAsCity(t *testing.T) {
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(failSrv.Close)

	cfSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ip=198.51.100.77\nloc=VN\ncolo=HKG\n")
	}))
	t.Cleanup(cfSrv.Close)

	t.Run("EmptyCityKeepsEmpty", func(t *testing.T) {
		whoisSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"ip":"198.51.100.77","success":true,"country_code":"VN","region":"","city":"","connection":{"asn":45899,"org":"VNPT Corp","isp":"VNPT"}}`)
		}))
		t.Cleanup(whoisSrv.Close)

		oldCZ, oldBD, oldPC, oldMir6, oldCF, oldWhois := cz88BaseURL, baiduBaseURL, pconlineBaseURL, mir6BaseURL, cfTraceURL, ipwhoisBaseURL
		oldAPI, oldIPSB, oldGeoJS := ipAPIBaseURL, ipsbBaseURL, geojsBaseURL
		cz88BaseURL, baiduBaseURL, pconlineBaseURL, mir6BaseURL = failSrv.URL, failSrv.URL, failSrv.URL, failSrv.URL
		cfTraceURL, ipwhoisBaseURL = cfSrv.URL, whoisSrv.URL
		ipAPIBaseURL, ipsbBaseURL, geojsBaseURL = failSrv.URL, failSrv.URL, failSrv.URL
		t.Cleanup(func() {
			cz88BaseURL, baiduBaseURL, pconlineBaseURL, mir6BaseURL = oldCZ, oldBD, oldPC, oldMir6
			cfTraceURL, ipwhoisBaseURL = oldCF, oldWhois
			ipAPIBaseURL, ipsbBaseURL, geojsBaseURL = oldAPI, oldIPSB, oldGeoJS
		})

		r, err := NewResolver("")
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()

		out := r.resolveOutbound(context.Background(), nil)
		if out == nil {
			t.Fatal("resolveOutbound 返回 nil")
		}
		if out.Country != "VN" {
			t.Errorf("期望 Country=VN, 得到 %q", out.Country)
		}
		if out.City != "" || out.Region != "" {
			t.Errorf("colo 不得冒充城市或省区, 期望 City='' Region='', 得到 City=%q Region=%q", out.City, out.Region)
		}
		if out.ASN != "AS45899" {
			t.Errorf("期望补全 ASN=AS45899, 得到 %q", out.ASN)
		}
	})

	t.Run("MismatchedMetaCountryDoesNotPolluteCity", func(t *testing.T) {
		whoisSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"ip":"198.51.100.77","success":true,"country_code":"HK","region":"Hong Kong","city":"Hong Kong","connection":{"asn":45899,"org":"VNPT Corp","isp":"VNPT"}}`)
		}))
		t.Cleanup(whoisSrv.Close)

		oldCZ, oldBD, oldPC, oldMir6, oldCF, oldWhois := cz88BaseURL, baiduBaseURL, pconlineBaseURL, mir6BaseURL, cfTraceURL, ipwhoisBaseURL
		oldAPI, oldIPSB, oldGeoJS := ipAPIBaseURL, ipsbBaseURL, geojsBaseURL
		cz88BaseURL, baiduBaseURL, pconlineBaseURL, mir6BaseURL = failSrv.URL, failSrv.URL, failSrv.URL, failSrv.URL
		cfTraceURL, ipwhoisBaseURL = cfSrv.URL, whoisSrv.URL
		ipAPIBaseURL, ipsbBaseURL, geojsBaseURL = failSrv.URL, failSrv.URL, failSrv.URL
		t.Cleanup(func() {
			cz88BaseURL, baiduBaseURL, pconlineBaseURL, mir6BaseURL = oldCZ, oldBD, oldPC, oldMir6
			cfTraceURL, ipwhoisBaseURL = oldCF, oldWhois
			ipAPIBaseURL, ipsbBaseURL, geojsBaseURL = oldAPI, oldIPSB, oldGeoJS
		})

		r, err := NewResolver("")
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()

		out := r.resolveOutbound(context.Background(), nil)
		if out == nil {
			t.Fatal("resolveOutbound 返回 nil")
		}
		if out.Country != "VN" {
			t.Errorf("期望 Country=VN, 得到 %q", out.Country)
		}
		if out.City != "" || out.Region != "" {
			t.Errorf("国家冲突的元数据不应污染 City/Region, 得到 City=%q Region=%q", out.City, out.Region)
		}
	})
}

func TestLookupPCOnlineGBKAndYitong(t *testing.T) {
	gbkBytes, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(`{"ip":"120.242.231.100","pro":"安徽省","city":"安庆市","addr":"安徽省安庆市 移通"}`))
	if err != nil {
		t.Fatal(err)
	}
	pcSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(gbkBytes)
	}))
	t.Cleanup(pcSrv.Close)

	oldPC := pconlineBaseURL
	pconlineBaseURL = pcSrv.URL
	t.Cleanup(func() { pconlineBaseURL = oldPC })

	r, err := NewResolver("")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	g, err := r.lookupPCOnline(context.Background(), "", nil)
	if err != nil {
		t.Fatalf("lookupPCOnline 失败: %v", err)
	}
	if g.IP != "120.242.231.100" || g.Country != "CN" || g.Region != "安徽省" || g.City != "安庆市" || g.ISP != "中国移动" {
		t.Errorf("lookupPCOnline = %+v", g)
	}
}

func TestLookupLocalNetworkPrefersDomesticPrecisionWithIP(t *testing.T) {
	gbkBytes, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(`{"ip":"120.242.231.100","pro":"安徽省","city":"安庆市","addr":"安徽省安庆市 移通"}`))
	if err != nil {
		t.Fatal(err)
	}
	pcSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(gbkBytes)
	}))
	t.Cleanup(pcSrv.Close)

	czSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.URL.Query().Get("ip")
		if ip == "" {
			fmt.Fprint(w, `{"code":5001,"success":false,"message":"Required String parameter 'ip' is not present"}`)
			return
		}
		fmt.Fprintf(w, `{"code":200,"success":true,"data":{"ip":%q,"countryCode":"CN","country":"中国","province":"安徽","city":"安庆","districts":"桐城市","isp":"中国移动","asn":"AS9808"}}`, ip)
	}))
	t.Cleanup(czSrv.Close)

	oldPC, oldCZ := pconlineBaseURL, cz88BaseURL
	pconlineBaseURL, cz88BaseURL = pcSrv.URL, czSrv.URL
	t.Cleanup(func() { pconlineBaseURL, cz88BaseURL = oldPC, oldCZ })

	r, err := NewResolver("")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	g := r.LookupLocalNetwork(context.Background())
	if g.IP != "120.242.231.100" || g.Country != "CN" || g.Region != "安徽" || g.City != "安庆桐城" || g.ISP != "中国移动" {
		t.Errorf("LookupLocalNetwork = %+v", g)
	}
}

func TestGeoCooldownDistrictSplitAndASNFill(t *testing.T) {
	czSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":200,"success":true,"data":{"ip":"120.242.231.100","countryCode":"CN","country":"中国","province":"安徽","city":"合肥","districts":"市辖区","isp":"中国移动","asn":"CHINAMOBILE-CN"}}`)
	}))
	t.Cleanup(czSrv.Close)

	ipsbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ip":"120.242.231.100","country_code":"CN","region":"Anhui","city":"Hefei","asn":9808,"asn_organization":"China Mobile","isp":"China Mobile"}`)
	}))
	t.Cleanup(ipsbSrv.Close)

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"success","countryCode":"CN","regionName":"安徽","city":"合肥市","query":"120.242.231.100","as":"AS9808 China Mobile","org":"China Mobile","isp":"China Mobile"}`)
	}))
	t.Cleanup(apiSrv.Close)

	oldCZ, oldIPSB, oldAPI := cz88BaseURL, ipsbBaseURL, ipAPIBaseURL
	cz88BaseURL, ipsbBaseURL, ipAPIBaseURL = czSrv.URL, ipsbSrv.URL, apiSrv.URL
	t.Cleanup(func() { cz88BaseURL, ipsbBaseURL, ipAPIBaseURL = oldCZ, oldIPSB, oldAPI })

	r, err := NewResolver("")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	// 1. 市辖区过滤 + 在线 ASN 补全
	g := r.Lookup(context.Background(), "120.242.231.100", nil)
	if g.City != "合肥" || g.ASN != "AS9808" || g.ISP != "中国移动" {
		t.Errorf("Lookup = %+v, 期望 City=合肥 ASN=AS9808", g)
	}

	// 2. 百度省市拆分
	reg, city := splitBaiduLoc("安徽省安庆市")
	if reg != "安徽省" || city != "安庆市" {
		t.Errorf("splitBaiduLoc = (%q, %q)", reg, city)
	}

	// 3. 冷却与业务拒绝区分
	r.markSourceFailed("ipwhois", "https://ipwho.is")
	if !r.isSourceCooling("ipwhois") {
		t.Error("非本地源失败后应进入冷却")
	}
	r.clearSourceFailed("ipwhois")
	if r.isSourceCooling("ipwhois") {
		t.Error("clearSourceFailed 后不应继续冷却")
	}
	if isTransientSourceErr(fmt.Errorf("pconline 非中国大陆 IP: 美国")) {
		t.Error("海外拒绝错误不应触发源冷却")
	}

	// 4. mergeGeoMissing 保留主来源已有字段并补全缺失字段
	merged := mergeGeoMissing(
		model.GeoInfo{IP: "1.1.1.1", Stack: "4", Country: "CN", City: "安庆桐城"},
		model.GeoInfo{Country: "US", City: "Guangzhou", ASN: "AS9808", Org: "CMCC", ISP: "中国移动"},
	)
	if merged.Country != "CN" || merged.City != "安庆桐城" || merged.ASN != "AS9808" || merged.ISP != "中国移动" {
		t.Errorf("mergeGeoMissing = %+v", merged)
	}
}

func TestLookupIPv6SkipsIPv4OnlyDomesticAndRejectsMismatch(t *testing.T) {
	var baiduHits, pcHits atomic.Int32

	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(failSrv.Close)

	czSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":5101,"success":false,"message":"今日免登录查询次数已用完，请登录查询更多","data":null}`)
	}))
	t.Cleanup(czSrv.Close)

	baiduSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		baiduHits.Add(1)
		fmt.Fprint(w, `{"status":"0","data":[{"location":"安徽省安庆市 移动","origip":"120.242.231.100"}]}`)
	}))
	t.Cleanup(baiduSrv.Close)

	pcSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pcHits.Add(1)
		fmt.Fprint(w, `{"ip":"120.242.231.100","pro":"安徽省","city":"安庆市","addr":"安徽省安庆市 移通"}`)
	}))
	t.Cleanup(pcSrv.Close)

	whoisSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ip":"2602:fa4f:b00:50e7:3aed:dfa3:a813:5830","success":true,"country_code":"JP","region":"Tokyo","city":"Tokyo","connection":{"asn":61112,"org":"AkileCloud LLC","isp":"AkileCloud LLC"}}`)
	}))
	t.Cleanup(whoisSrv.Close)

	oldCZ, oldBD, oldPC, oldWhois := cz88BaseURL, baiduBaseURL, pconlineBaseURL, ipwhoisBaseURL
	oldAPI, oldIPSB, oldGeoJS, oldMir6 := ipAPIBaseURL, ipsbBaseURL, geojsBaseURL, mir6BaseURL
	cz88BaseURL, baiduBaseURL, pconlineBaseURL, ipwhoisBaseURL = czSrv.URL, baiduSrv.URL, pcSrv.URL, whoisSrv.URL
	ipAPIBaseURL, ipsbBaseURL, geojsBaseURL, mir6BaseURL = failSrv.URL, failSrv.URL, failSrv.URL, failSrv.URL
	t.Cleanup(func() {
		cz88BaseURL, baiduBaseURL, pconlineBaseURL, ipwhoisBaseURL = oldCZ, oldBD, oldPC, oldWhois
		ipAPIBaseURL, ipsbBaseURL, geojsBaseURL, mir6BaseURL = oldAPI, oldIPSB, oldGeoJS, oldMir6
	})

	r, err := NewResolver("")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ctx := context.Background()
	targetV6 := "2602:fa4f:B00:50e7:3aed:dfa3:a813:5830"
	g := r.Lookup(ctx, targetV6, nil)
	if g.Stack != "6" || g.Country != "JP" || g.ASN != "AS61112" || !sameIP(g.IP, targetV6) || g.IP == "120.242.231.100" {
		t.Fatalf("IPv6 Lookup 结果异常: %+v", g)
	}
	if baiduHits.Load() != 0 || pcHits.Load() != 0 {
		t.Fatalf("IPv6 查询不应请求仅支持 IPv4 的国内接口: baidu=%d, pconline=%d", baiduHits.Load(), pcHits.Load())
	}

	if _, err := r.lookupPCOnline(ctx, targetV6, nil); err == nil || isTransientSourceErr(err) {
		t.Fatalf("lookupPCOnline(IPv6) 应拒绝且不触发冷却, got err=%v", err)
	}
	if _, err := r.lookupBaidu(ctx, targetV6, nil); err == nil || isTransientSourceErr(err) {
		t.Fatalf("lookupBaidu(IPv6) 应拒绝且不触发冷却, got err=%v", err)
	}
	if baiduHits.Load() != 0 || pcHits.Load() != 0 {
		t.Fatalf("IPv6 快速拒绝不应发 HTTP 请求: baidu=%d, pconline=%d", baiduHits.Load(), pcHits.Load())
	}

	if _, err := r.lookupPCOnline(ctx, "8.8.8.8", nil); err == nil || isTransientSourceErr(err) {
		t.Fatalf("lookupPCOnline 返回不匹配 IP 时应报错且不触发冷却, got err=%v", err)
	}
}

func TestDecodeCNDivisionAndNormalizeISP(t *testing.T) {
	cases := []struct {
		code       uint32
		wantRegion string
		wantCity   string
	}{
		{340881, "安徽省", "安庆桐城"},
		{110108, "北京市", "北京海淀区"},
		{440309, "广东省", "深圳龙华区"},
	}
	for _, tc := range cases {
		reg, city := decodeCNDivision(tc.code)
		if reg != tc.wantRegion || city != tc.wantCity {
			t.Errorf("decodeCNDivision(%d) = (%q, %q), 期望 (%q, %q)", tc.code, reg, city, tc.wantRegion, tc.wantCity)
		}
	}
	for in, want := range map[string]string{
		"移动":  "中国移动",
		"电信":  "中国电信",
		"联通":  "中国联通",
		"广电":  "中国广电",
		"教育网": "中国教育网",
	} {
		if got := normalizeCNISP(in); got != want {
			t.Errorf("normalizeCNISP(%q) = %q, 期望 %q", in, got, want)
		}
	}
}

func TestOnlineLookupPrefersInternationalForOverseasAndRefinesCN(t *testing.T) {
	var domesticHits atomic.Int32

	domesticSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		domesticHits.Add(1)
		http.Error(w, "unexpected domestic call", http.StatusInternalServerError)
	}))
	t.Cleanup(domesticSrv.Close)

	czSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.URL.Query().Get("ip")
		if ip == "203.0.113.99" {
			fmt.Fprint(w, `{"code":200,"success":true,"data":{"ip":"203.0.113.99","countryCode":"US","country":"美国","province":"加利福尼亚州","city":"洛杉矶","districts":"未知","isp":"IT7","asn":"AS25820"}}`)
			return
		}
		fmt.Fprint(w, `{"code":200,"success":true,"data":{"ip":"120.242.231.100","countryCode":"CN","country":"中国","province":"安徽省","city":"安庆市","districts":"桐城市","isp":"中国移动","asn":"CHINAMOBILE-CN"}}`)
	}))
	t.Cleanup(czSrv.Close)

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := strings.TrimPrefix(r.URL.Path, "/json/")
		if ip == "120.242.231.100" {
			fmt.Fprint(w, `{"status":"success","countryCode":"CN","regionName":"广东","city":"广州市","query":"120.242.231.100","as":"AS9808 China Mobile","org":"China Mobile","isp":"China Mobile"}`)
			return
		}
		fmt.Fprintf(w, `{"status":"success","countryCode":"US","regionName":"California","city":"Los Angeles","query":%q,"as":"AS13335 Cloudflare Inc","org":"Cloudflare","isp":"Cloudflare"}`, ip)
	}))
	t.Cleanup(apiSrv.Close)

	oldCZ, oldBD, oldPC, oldMir6, oldAPI := cz88BaseURL, baiduBaseURL, pconlineBaseURL, mir6BaseURL, ipAPIBaseURL
	cz88BaseURL, baiduBaseURL, pconlineBaseURL, mir6BaseURL, ipAPIBaseURL = domesticSrv.URL, domesticSrv.URL, domesticSrv.URL, domesticSrv.URL, apiSrv.URL
	t.Cleanup(func() {
		cz88BaseURL, baiduBaseURL, pconlineBaseURL, mir6BaseURL, ipAPIBaseURL = oldCZ, oldBD, oldPC, oldMir6, oldAPI
	})

	r, err := NewResolver("")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ctx := context.Background()
	// 1. 海外 IP 仅走国际源，不触发任何国内接口
	gUS := r.Lookup(ctx, "1.2.3.4", nil)
	if gUS.Country != "US" || gUS.ASN != "AS13335" {
		t.Fatalf("海外 IP 结果异常: %+v", gUS)
	}
	if domesticHits.Load() != 0 {
		t.Fatalf("海外 IP 不应触发国内接口, 实际调用 %d 次", domesticHits.Load())
	}

	// 2. CN IP 先由国际源判定为 CN + 拿 ASN，再由国内高精源补全区县与运营商
	cz88BaseURL = czSrv.URL
	gCN := r.Lookup(ctx, "120.242.231.100", nil)
	if gCN.Country != "CN" || gCN.Region != "安徽省" || gCN.City != "安庆桐城" || gCN.ISP != "中国移动" || gCN.ASN != "AS9808" || gCN.Org != "China Mobile" {
		t.Fatalf("CN IP 融合结果异常: %+v", gCN)
	}

	// 3. CZ88 返回 "未知" 区县应被自动过滤
	gUnknown, err := r.lookupCZ88(ctx, "203.0.113.99", nil)
	if err != nil || gUnknown.City != "洛杉矶" {
		t.Fatalf("CZ88 '未知' 过滤异常: g=%+v err=%v", gUnknown, err)
	}
}

// minimalTestMMDB 构造一个可通过 maxminddb.Open 校验的最小合法 MMDB 字节序列。
func minimalTestMMDB() []byte {
	var buf []byte
	// 1 node * 24-bit record = 6 zero bytes + 16 zero separator bytes
	buf = append(buf, make([]byte, 22)...)
	buf = append(buf, []byte("\xab\xcd\xefMaxMind.com")...)
	appendStr := func(s string) {
		buf = append(buf, byte(0x40|len(s)))
		buf = append(buf, []byte(s)...)
	}
	buf = append(buf, 0xe5) // Map with 5 keys
	appendStr("node_count")
	buf = append(buf, 0xc1, 0x01) // uint32(1)
	appendStr("record_size")
	buf = append(buf, 0xa1, 24) // uint16(24)
	appendStr("ip_version")
	buf = append(buf, 0xa1, 6) // uint16(6)
	appendStr("binary_format_major_version")
	buf = append(buf, 0xa1, 2) // uint16(2)
	appendStr("binary_format_minor_version")
	buf = append(buf, 0xa0) // uint16(0)
	return buf
}

func TestUpdateDBsFallbackAndValidation(t *testing.T) {
	validMMDB := minimalTestMMDB()

	relSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(relSrv.Close)

	upSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "corrupt") {
			fmt.Fprint(w, "<html>not a mmdb</html>")
			return
		}
		_, _ = w.Write(validMMDB)
	}))
	t.Cleanup(upSrv.Close)

	oldRel, oldCN, oldCity, oldASN, oldIPinfo := GeoIPReleaseBaseURL, GeoCNUpstreamURL, GeoLiteCityUpstreamURL, GeoLiteASNUpstreamURL, IPinfoLiteDownloadBaseURL
	GeoIPReleaseBaseURL = relSrv.URL
	GeoCNUpstreamURL = upSrv.URL + "/GeoCN.mmdb"
	GeoLiteCityUpstreamURL = upSrv.URL + "/GeoLite2-City.mmdb"
	GeoLiteASNUpstreamURL = upSrv.URL + "/GeoLite2-ASN.mmdb"
	IPinfoLiteDownloadBaseURL = upSrv.URL + "/corrupt/ipinfo_lite.mmdb"
	t.Cleanup(func() {
		GeoIPReleaseBaseURL, GeoCNUpstreamURL, GeoLiteCityUpstreamURL, GeoLiteASNUpstreamURL, IPinfoLiteDownloadBaseURL = oldRel, oldCN, oldCity, oldASN, oldIPinfo
	})

	dir := t.TempDir()
	res, err := UpdateDBs(context.Background(), UpdateOptions{Dir: dir})
	if err != nil {
		t.Fatalf("UpdateDBs 失败: %v", err)
	}
	if !res.Status.FullReady || res.Status.Mode != "offline" {
		t.Fatalf("UpdateDBs 后状态异常: %+v", res.Status)
	}
	for _, name := range []string{geoCNFileName, geoLiteASNFileName, geoLiteCityName} {
		if _, statErr := os.Stat(filepath.Join(dir, name)); statErr != nil {
			t.Errorf("期望写入 %s: %v", name, statErr)
		}
	}

	// 验证无效 HTML 响应不会覆盖已有的合法 MMDB，且错误中绝不泄露 Token
	GeoCNUpstreamURL = upSrv.URL + "/corrupt/GeoCN.mmdb"
	GeoLiteCityUpstreamURL = upSrv.URL + "/corrupt/GeoLite2-City.mmdb"
	GeoLiteASNUpstreamURL = upSrv.URL + "/corrupt/GeoLite2-ASN.mmdb"
	secret := "secret-tok-xyz"
	_, errFail := UpdateDBs(context.Background(), UpdateOptions{Dir: dir, IPinfoToken: secret})
	if errFail == nil {
		t.Fatal("所有源均返回损坏数据时应报错")
	}
	if strings.Contains(errFail.Error(), secret) {
		t.Fatalf("错误消息中不得包含 Token: %v", errFail)
	}
	if b, readErr := os.ReadFile(filepath.Join(dir, geoCNFileName)); readErr != nil || len(b) != len(validMMDB) {
		t.Fatalf("损坏响应不得覆盖原有合法 %s", geoCNFileName)
	}
}

func TestDefaultDBDirPriorityAndPortableFallback(t *testing.T) {
	// 1. 环境变量优先级最高
	t.Setenv("NEKOSPEED_GEOIP_DIR", "/custom/geoip-dir")
	if got := DefaultDBDir(); got != "/custom/geoip-dir" {
		t.Fatalf("NEKOSPEED_GEOIP_DIR 覆盖失效: got %q", got)
	}
	t.Setenv("NEKOSPEED_GEOIP_DIR", "")

	// 2. 当前工作目录存在 geoip/ 时优先返回 "geoip"
	cwdWithGeo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwdWithGeo, "geoip"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cwdWithGeo)
	if got := DefaultDBDir(); got != "geoip" {
		t.Fatalf("CWD 存在 geoip 目录时期望返回 \"geoip\", 实际 %q", got)
	}

	// 3. 切换至无 geoip 的 CWD，验证临时测试二进制（os.TempDir / go-build）被跳过
	cwdEmpty := t.TempDir()
	t.Chdir(cwdEmpty)
	if got := DefaultDBDir(); got != "geoip" {
		t.Fatalf("测试二进制应跳过 exeDir 回退并返回 \"geoip\", 实际 %q", got)
	}

	// 4. 模拟便携版可执行文件从其他 CWD 启动：存在同级 <exeDir>/geoip
	portableRoot := t.TempDir()
	portableGeo := filepath.Join(portableRoot, "geoip")
	if err := os.MkdirAll(portableGeo, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeExe := filepath.Join(portableRoot, "nekospeed.exe")
	gotSibling := resolveDefaultDBDir("", func() (string, error) { return fakeExe, nil }, filepath.Join(portableRoot, "unrelated-temp"))
	if gotSibling != portableGeo {
		t.Fatalf("便携版存在同级 geoip 时期望返回 %q, 实际 %q", portableGeo, gotSibling)
	}

	// 5. 模拟便携版首次运行（同级 geoip 尚未创建，但 exeDir 为有效便携安装目录）
	freshRoot := t.TempDir()
	freshExe := filepath.Join(freshRoot, "nekospeed.exe")
	gotFresh := resolveDefaultDBDir("", func() (string, error) { return freshExe, nil }, filepath.Join(freshRoot, "unrelated-temp"))
	if want := filepath.Join(freshRoot, "geoip"); gotFresh != want {
		t.Fatalf("便携安装目录首次运行期望返回 %q, 实际 %q", want, gotFresh)
	}

	// 6. go-build 或 temp 目录内可执行文件应被跳过
	gotGoBuild := resolveDefaultDBDir("", func() (string, error) {
		return filepath.Join(portableRoot, "go-build1234", "b001", "nekospeed.exe"), nil
	}, filepath.Join(portableRoot, "unrelated-temp"))
	if gotGoBuild != "geoip" {
		t.Fatalf("go-build 路径应跳过 exeDir 回退, 实际 %q", gotGoBuild)
	}
	gotTemp := resolveDefaultDBDir("", func() (string, error) { return fakeExe, nil }, portableRoot)
	if gotTemp != "geoip" {
		t.Fatalf("TempDir 内路径应跳过 exeDir 回退, 实际 %q", gotTemp)
	}
}

func TestUpdateDBsParallelAndPartialFailure(t *testing.T) {
	validMMDB := minimalTestMMDB()

	var active atomic.Int32
	var maxActive atomic.Int32
	allThreeStarted := make(chan struct{})
	var startedCount atomic.Int32

	relSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(relSrv.Close)

	upSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := active.Add(1)
		for {
			prev := maxActive.Load()
			if cur <= prev || maxActive.CompareAndSwap(prev, cur) {
				break
			}
		}
		defer active.Add(-1)

		if startedCount.Add(1) == 3 {
			close(allThreeStarted)
		}
		select {
		case <-allThreeStarted:
		case <-time.After(1500 * time.Millisecond):
		}

		if strings.Contains(r.URL.Path, "fail-city") {
			http.Error(w, "city unavailable", http.StatusBadGateway)
			return
		}
		_, _ = w.Write(validMMDB)
	}))
	t.Cleanup(upSrv.Close)

	oldRel, oldCN, oldCity, oldASN, oldIPinfo := GeoIPReleaseBaseURL, GeoCNUpstreamURL, GeoLiteCityUpstreamURL, GeoLiteASNUpstreamURL, IPinfoLiteDownloadBaseURL
	GeoIPReleaseBaseURL = relSrv.URL
	GeoCNUpstreamURL = upSrv.URL + "/GeoCN.mmdb"
	GeoLiteASNUpstreamURL = upSrv.URL + "/GeoLite2-ASN.mmdb"
	GeoLiteCityUpstreamURL = upSrv.URL + "/fail-city/GeoLite2-City.mmdb"
	IPinfoLiteDownloadBaseURL = upSrv.URL + "/ipinfo_lite.mmdb"
	t.Cleanup(func() {
		GeoIPReleaseBaseURL, GeoCNUpstreamURL, GeoLiteCityUpstreamURL, GeoLiteASNUpstreamURL, IPinfoLiteDownloadBaseURL = oldRel, oldCN, oldCity, oldASN, oldIPinfo
	})

	var progressDone []int
	dir := t.TempDir()
	res, err := UpdateDBs(context.Background(), UpdateOptions{
		Dir: dir,
		OnProgress: func(stage string, done, total int) {
			if total != 3 {
				t.Errorf("OnProgress total = %d, 期望 3", total)
			}
			progressDone = append(progressDone, done)
		},
	})
	if err != nil {
		t.Fatalf("部分失败（2/3 成功）时不应返回 error: %v", err)
	}
	if maxActive.Load() < 2 {
		t.Errorf("期望 3 个数据库并发下载 (maxActive >= 2), 实际 maxActive = %d", maxActive.Load())
	}
	if len(res.Updated) != 2 || len(res.Failed) != 1 {
		t.Fatalf("部分更新结果异常: updated=%v failed=%v", res.Updated, res.Failed)
	}
	if res.Status.Mode != "hybrid" || !res.Status.Ready || res.Status.FullReady {
		t.Fatalf("部分更新后状态期望 hybrid, 实际 %+v", res.Status)
	}
	// 期望单调进度 0 -> 1 -> 2 -> 3 -> 3
	if len(progressDone) < 4 || progressDone[0] != 0 || progressDone[len(progressDone)-1] != 3 {
		t.Fatalf("OnProgress 进度序列异常: %v", progressDone)
	}
	for i := 1; i < len(progressDone); i++ {
		if progressDone[i] < progressDone[i-1] {
			t.Fatalf("OnProgress 进度未单调递增: %v", progressDone)
		}
	}
}
