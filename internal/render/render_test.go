package render

import (
	"image/png"
	"os"
	"testing"
	"time"
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
