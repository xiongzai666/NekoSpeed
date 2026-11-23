package render

import (
	"image/png"
	"os"
	"strings"
	"testing"
	"time"

	"nekospeed/internal/model"
)

func TestExtractNodeFlag(t *testing.T) {
	code, clean := extractNodeFlag("🇯🇵 [vip1]日本1|2X")
	if code != "JP" || clean != "[vip1]日本1|2X" {
		t.Fatalf("expected JP / [vip1]日本1|2X, got %q / %q", code, clean)
	}
	code2, clean2 := extractNodeFlag("香港 IPLC 01")
	if code2 != "HK" || clean2 != "香港 IPLC 01" {
		t.Fatalf("expected HK / 香港 IPLC 01, got %q / %q", code2, clean2)
	}
}

func TestRandomThemeAndNames(t *testing.T) {
	if len(ThemeNames()) == 0 {
		t.Fatal("ThemeNames 不应为空")
	}
	if rt := RandomTheme(); rt.Name == "" {
		t.Fatal("RandomTheme 应返回有名称的主题")
	}
	if th := ThemeByName("not_exist"); th.Name != "default" {
		t.Fatalf("不存在的主题应回退 default, 实际 %q", th.Name)
	}
	if th := ThemeByName("rose_light"); th.Name != "rose_light" {
		t.Fatalf("ThemeByName 应返回 rose_light, 实际 %q", th.Name)
	}
}

func TestLocalISPTitle(t *testing.T) {
	cases := []struct {
		base, defaultSuffix, localISP, bandwidth string
		single                                   bool
		want                                     string
	}{
		{"NekoSpeed | speed - ", "订阅速度", "", "", false, "NekoSpeed | speed - 订阅速度"},
		{"NekoSpeed | speed - ", "订阅速度", "上海电信", "1Gbps", false, "NekoSpeed | speed - 上海电信@1Gbps"},
		{"NekoSpeed | speed - ", "订阅速度", "上海电信", "1Gbps", true, "NekoSpeed | speed - 单线程上海电信@1Gbps"},
		{"NekoSpeed | speed - ", "订阅速度", "上海电信", "", false, "NekoSpeed | speed - 上海电信"},
		{"NekoSpeed | speed - ", "订阅速度", "", "1Gbps", false, "NekoSpeed | speed - 1Gbps"},
		{"NekoSpeed | speed - ", "订阅速度", "", "上海电信@1Gbps", false, "NekoSpeed | speed - 上海电信@1Gbps"},
		{"NekoSpeed | fftest - ", "综合测试", "北京联通", "500Mbps", false, "NekoSpeed | fftest - 北京联通@500Mbps"},
	}
	for _, c := range cases {
		got := LocalISPTitle(c.base, c.defaultSuffix, c.localISP, c.bandwidth, c.single)
		if got != c.want {
			t.Errorf("LocalISPTitle(%q, %q, %q, %q, %v) = %q, want %q",
				c.base, c.defaultSuffix, c.localISP, c.bandwidth, c.single, got, c.want)
		}
	}
}

func TestRenderTables(t *testing.T) {
	img, err := renderTables(Meta{
		Version:   "0.1.0",
		Threads:   1,
		Total:     2,
		Succeeded: 2,
		StartTime: time.Now(),
		Theme:     DefaultTheme(),
	}, "Background Test", "footer", []*Table{
		{
			Section: "Section",
			Cols:    []Col{{Header: "Name"}, {Header: "Value"}},
			Rows: [][]Cell{
				{{Text: "A", Align: AlignLeft}, {Text: "100", Align: AlignCenter}},
				{{Text: "B", Align: AlignLeft}, {Text: "200", Align: AlignCenter}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/render-bg-test.png"
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("渲染图片为空")
	}
	t.Logf("渲染测试通过，图片: %s, 大小: %d bytes", path, info.Size())
}

func TestFontCacheKeysByPathAndFontFace(t *testing.T) {
	f1, err := loadFont("")
	if err != nil {
		t.Skipf("无可用系统字体: %v", err)
	}
	// 第二次传入不同的不存在自定义路径，不应和 default key 混淆；至少返回有效字体，且同 key 命中缓存。
	f2, err := loadFont(t.TempDir() + "/missing.ttf")
	if err != nil {
		t.Fatal(err)
	}
	fc1, err := face(f1, 14)
	if err != nil {
		t.Fatal(err)
	}
	fc2, err := face(f2, 14)
	if err != nil {
		t.Fatal(err)
	}
	if fc1 == nil || fc2 == nil {
		t.Fatal("face 不应为 nil")
	}
}

func TestLocalizeCountryCoverage(t *testing.T) {
	if len(countryNames) < 75 {
		t.Fatalf("countryNames 覆盖数 = %d, 期望至少 75", len(countryNames))
	}
	cases := map[string]string{
		"NG": "尼日利亚", "PL": "波兰", "FI": "芬兰", "UA": "乌克兰", "RO": "罗马尼亚",
		"CZ": "捷克", "AT": "奥地利", "IE": "爱尔兰", "NO": "挪威", "DK": "丹麦",
		"BE": "比利时", "PT": "葡萄牙", "GR": "希腊", "HU": "匈牙利", "BG": "保加利亚",
		"LU": "卢森堡", "IS": "冰岛", "EE": "爱沙尼亚", "LV": "拉脱维亚", "LT": "立陶宛",
		"MX": "墨西哥", "CL": "智利", "CO": "哥伦比亚", "PE": "秘鲁", "ZA": "南非",
		"EG": "埃及", "KZ": "哈萨克斯坦", "PK": "巴基斯坦", "BD": "孟加拉国", "NZ": "新西兰",
		"SA": "沙特阿拉伯", "QA": "卡塔尔", "KH": "柬埔寨", "CY": "塞浦路斯", "RS": "塞尔维亚",
		"HR": "克罗地亚", "SK": "斯洛伐克", "SI": "斯洛文尼亚",
		"VN": "越南", "ID": "印尼", "TR": "土耳其", "BR": "巴西", "AR": "阿根廷",
		"AE": "阿联酋", "CH": "瑞士", "SE": "瑞典", "IT": "意大利", "ES": "西班牙",
		"hk": "中国香港", "uk": "英国", "": "", "ZZ": "ZZ",
	}
	for code, want := range cases {
		if got := localizeCountry(code); got != want {
			t.Errorf("localizeCountry(%q) = %q, want %q", code, got, want)
		}
	}
}

func TestFormatGeoLocationAndInboundOrg(t *testing.T) {
	cases := []struct {
		geo  model.GeoInfo
		want string
	}{
		{model.GeoInfo{Country: "CN", Region: "安徽省", City: "安庆桐城"}, "中国·安庆桐城"},
		{model.GeoInfo{Country: "CN", Region: "上海市", City: "上海市"}, "中国·上海"},
		{model.GeoInfo{Country: "CN", Region: "上海", City: "上海"}, "中国·上海"},
		{model.GeoInfo{Country: "CN", Region: "安徽省", City: ""}, "中国·安徽"},
		{model.GeoInfo{Country: "CN", Region: "北京市", City: "北京海淀区"}, "中国·北京海淀区"},
		{model.GeoInfo{Country: "CN", Region: "北京市", City: "市辖区"}, "中国·北京"},
		{model.GeoInfo{Country: "CN"}, "中国"},
		{model.GeoInfo{Country: "HK", City: "Hong Kong"}, "中国香港"},
		{model.GeoInfo{Country: "HK", City: "香港"}, "中国香港"},
		{model.GeoInfo{Country: "TW", City: "Taiwan"}, "中国台湾"},
		{model.GeoInfo{Country: "TW", City: "台北"}, "中国台湾·台北"},
		{model.GeoInfo{Country: "MO", City: "Macau"}, "中国澳门"},
		{model.GeoInfo{Country: "SG", City: "Singapore"}, "新加坡"},
		{model.GeoInfo{Country: "JP", City: "东京"}, "日本·东京"},
		{model.GeoInfo{Country: "US", City: "洛杉矶"}, "美国·洛杉矶"},
		{model.GeoInfo{Country: "DE", City: ""}, "德国"},
		{model.GeoInfo{}, ""},
	}
	for _, tc := range cases {
		if got := formatGeoLocation(tc.geo); got != tc.want {
			t.Errorf("formatGeoLocation(%+v) = %q, want %q", tc.geo, got, tc.want)
		}
	}

	if got := formatInboundOrg(model.GeoInfo{
		Country: "CN",
		Org:     "China Mobile Communications Group Co., Ltd",
		ISP:     "中国移动",
	}); got != "中国移动" {
		t.Errorf("CN 入口应优先使用简洁中文 ISP, got %q", got)
	}
	if got := formatInboundOrg(model.GeoInfo{
		Country: "CN",
		Org:     "China Telecom",
		ISP:     "电信",
	}); got != "中国电信" {
		t.Errorf("CN 入口简写 ISP 应规范为 中国电信, got %q", got)
	}
	if got := formatInboundOrg(model.GeoInfo{
		Country: "US",
		Org:     "Cloudflare, Inc.",
		ISP:     "Cloudflare",
	}); got != "Cloudflare, Inc." {
		t.Errorf("海外入口应保留 Org, got %q", got)
	}
}

func TestTopologyTablesLocationMergeAndImage(t *testing.T) {
	topo := &model.Topology{
		Inbounds: []model.InboundEntry{
			{
				Index:    1,
				Geo:      model.GeoInfo{Country: "CN", Region: "安徽省", City: "安庆桐城", ASN: "AS9808", Org: "China Mobile Communications Group Co., Ltd", ISP: "中国移动", Stack: "4"},
				Cluster:  1,
				OutCount: 2,
			},
			{
				Index:    2,
				Geo:      model.GeoInfo{Country: "CN", Region: "安徽省", City: "安庆桐城", ASN: "AS9808", Org: "CMCC", ISP: "移动", Stack: "4"},
				Cluster:  2,
				OutCount: 1,
			},
		},
		Outbounds: []model.OutboundEntry{
			{InboundIndex: 1, Geo: model.GeoInfo{Country: "US", City: "洛杉矶", ASN: "AS1", Org: "OrgA", Stack: "4"}, NodeName: "🇺🇸 US LAX 1"},
			{InboundIndex: 1, Geo: model.GeoInfo{Country: "US", City: "圣何塞", ASN: "AS1", Org: "OrgA", Stack: "4"}, NodeName: "🇺🇸 US SJC"},
			{InboundIndex: 1, Geo: model.GeoInfo{Country: "US", City: "洛杉矶", ASN: "AS1", Org: "OrgA", Stack: "4"}, NodeName: "🇺🇸 US LAX 2"},
		},
	}

	inTable, outTable := buildTopologyTables(topo, DefaultTheme())
	if len(inTable.Rows) != 2 {
		t.Fatalf("inTable rows = %d, want 2", len(inTable.Rows))
	}
	if inTable.Rows[0][1].Text != "中国·安庆桐城" || inTable.Rows[0][1].Span != 2 || !inTable.Rows[1][1].Skip {
		t.Errorf("inTable 地区合并异常: row0=%+v row1=%+v", inTable.Rows[0][1], inTable.Rows[1][1])
	}
	if inTable.Rows[0][3].Text != "中国移动" || inTable.Rows[0][3].Span != 2 || !inTable.Rows[1][3].Skip {
		t.Errorf("inTable CN ISP 展示与合并异常: row0=%+v row1=%+v", inTable.Rows[0][3], inTable.Rows[1][3])
	}

	if len(outTable.Rows) != 3 {
		t.Fatalf("outTable rows = %d, want 3", len(outTable.Rows))
	}
	// 排序后 ("圣" U+5723 < "洛" U+6D1B) 美国·圣何塞排在首位 (Span=1)，两条美国·洛杉矶紧邻并合并 (Span=2)
	if outTable.Rows[0][2].Text != "美国·圣何塞" || outTable.Rows[0][2].Span != 1 {
		t.Errorf("outTable 异城市不应合并: row0=%+v", outTable.Rows[0][2])
	}
	if outTable.Rows[1][2].Text != "美国·洛杉矶" || outTable.Rows[1][2].Span != 2 || !outTable.Rows[2][2].Skip {
		t.Errorf("outTable 同城市合并异常: row1=%+v row2=%+v", outTable.Rows[1][2], outTable.Rows[2][2])
	}
	if outTable.Rows[0][3].Span != 1 || outTable.Rows[1][3].Span != 2 || !outTable.Rows[2][3].Skip {
		t.Errorf("outTable 自治编号应跟随地区边界合并: row0=%+v row1=%+v row2=%+v", outTable.Rows[0][3], outTable.Rows[1][3], outTable.Rows[2][3])
	}

	outPath := t.TempDir() + "/topo.png"
	results := []*model.NodeResult{
		{Node: model.Node{Name: "US1"}, Speed: &model.SpeedResult{Avg: 1024}},
	}
	if err := SaveTopologyImage(results, topo, Meta{Version: "0.1.0", StartTime: time.Now()}, outPath); err != nil {
		t.Fatalf("SaveTopologyImage 失败: %v", err)
	}
	if info, err := os.Stat(outPath); err != nil || info.Size() == 0 {
		t.Fatalf("拓扑图文件生成异常: err=%v info=%v", err, info)
	}
}

func TestFooterTimeDynamicTimezone(t *testing.T) {
	origLocal := time.Local
	t.Cleanup(func() { time.Local = origLocal })

	time.Local = time.FixedZone("EST", -5*3600)
	ts := time.Date(2026, 11, 23, 15, 4, 5, 0, time.UTC)
	got := formatFooterTimeText(ts)
	if !strings.Contains(got, "2026-11-23T10:04:05 (EST)") {
		t.Fatalf("期望动态本地时区缩写 (EST), 实际 %q", got)
	}
	if strings.Contains(got, "(CST)") {
		t.Fatalf("非 CST 时区不应硬编码 (CST), 实际 %q", got)
	}
}
