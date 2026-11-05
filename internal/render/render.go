// Package render 把测速结果渲染成复刻 MiaoKo（喵速）风格的 PNG 表格图。
//
// 通用风格：浅色主题——白底、浅灰表头黑字居中、数据行白底行间细灰线、
// 序号列红色数字居中、节点名称列左对齐、无圆角无阴影，整图叠斜向平铺水印。
package render

import (
	"image"
	"image/png"
	"os"
	"strings"
	"time"
)

// Meta 渲染元信息，由调用方组装。
type Meta struct {
	Version      string        // 版本号，如 "0.1.0"
	Threads      int           // 测速线程数
	Total        int           // 节点总数
	Succeeded    int           // 成功节点数
	StartTime    time.Time     // 测试开始时间
	Watermark    string        // 水印文字
	FontPath     string        // 自定义字体路径（可空，自动探测）
	Single       bool          // 是否单线程测速
	LocalISP     string        // 本地运营商，如 "Shanghai China Telecom"
	Bandwidth    string        // 本地带宽，如 "1Gbps" / "500Mbps"
	SortLabel    string        // 排序说明，如 "订阅原序" / "速度降序" / "延迟升序"
	Theme        Theme         // 配色主题，空值时 renderTables 自动选择默认浅色主题
	SubName      string        // 订阅名称，如 "魔戒.net"
	Elapsed      time.Duration // 测试总耗时
	PingCount    int           // 延迟测试 Ping 次数
	Filter       string        // 过滤器表达式说明
	TrafficBytes int64         // 测速消耗总流量（字节）
}


// LocalISPTitle 根据本地运营商和带宽信息生成标题。
// base 是标题前缀（应包含 " - "），defaultSuffix 是本地运营商与带宽均为空时的默认后缀。
// 若 bandwidth 已包含 "@"（如 "上海电信@1Gbps"）或 localISP 为空但手动指定了 bandwidth（如 "1Gbps"），
// 则直接用其替换标题后缀；否则拼接为 "运营商@带宽"，并在单线程时加上 "单线程" 前缀。
func LocalISPTitle(base, defaultSuffix, localISP, bandwidth string, single bool) string {
	localISP = strings.TrimSpace(localISP)
	bandwidth = strings.TrimSpace(bandwidth)
	if localISP == "" && bandwidth == "" {
		return base + defaultSuffix
	}
	var suffix string
	switch {
	case strings.Contains(bandwidth, "@") || localISP == "":
		suffix = bandwidth
	case bandwidth != "":
		suffix = localISP + "@" + bandwidth
	default:
		suffix = localISP
	}
	if single && !strings.HasPrefix(suffix, "单线程") {
		suffix = "单线程" + suffix
	}
	return base + suffix
}

func savePNG(img *image.RGBA, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
