package runner

import (
	"testing"

	"nekospeed/internal/model"
)

func TestFormatLocalISP(t *testing.T) {
	cases := []struct {
		g    model.GeoInfo
		want string
	}{
		{model.GeoInfo{City: "上海", ISP: "China Telecom"}, "上海电信"},
		{model.GeoInfo{City: "深圳", ISP: "China Unicom"}, "深圳联通"},
		{model.GeoInfo{Region: "陕西", ISP: "China Mobile"}, "陕西移动"},
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
	}
	for _, name := range infoNames {
		if !isInfoNodeName(name) {
			t.Errorf("isInfoNodeName(%q) = false, want true", name)
		}
	}
	realNames := []string{
		"🇭🇰 香港1|1x",
		"🇯🇵 日本IPLC-01",
		"🇺🇸 美国BGP专线",
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
}
