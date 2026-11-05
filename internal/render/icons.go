package render

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
)

const (
	flagW = 22
	flagH = 15
)

// extractNodeFlag 从节点名中提取首个国旗 emoji（Regional Indicator 对）或根据常见地区名推断国家码，
// 并返回去除不可渲染 emoji 后的整洁节点名称。
func extractNodeFlag(name string) (country string, cleanName string) {
	rs := []rune(name)
	var out []rune
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		// Regional Indicator Symbol Letter A (0x1F1E6) .. Z (0x1F1FF)
		if r >= 0x1F1E6 && r <= 0x1F1FF && i+1 < len(rs) && rs[i+1] >= 0x1F1E6 && rs[i+1] <= 0x1F1FF {
			if country == "" {
				c1 := byte('A' + (r - 0x1F1E6))
				c2 := byte('A' + (rs[i+1] - 0x1F1E6))
				country = string([]byte{c1, c2})
			}
			i++ // 跳过第二个区域指示符
			continue
		}
		// 跳过变体选择符与零宽连接符
		if r == 0xFE0F || r == 0xFE0E || r == 0x200D {
			continue
		}
		out = append(out, r)
	}
	cleanName = strings.TrimSpace(string(out))
	if country == "" {
		country = inferCountryFromName(cleanName)
	}
	return country, cleanName
}

// ExtractNodeFlag 提取节点名称中的国旗代码与清理后的纯文本节点名。
func ExtractNodeFlag(name string) (string, string) {
	return extractNodeFlag(name)
}

var countryKeywords = []struct {
	kw   string
	code string
}{
	{"香港", "HK"}, {"HKG", "HK"}, {"HONGKONG", "HK"}, {"HONG KONG", "HK"},
	{"台湾", "TW"}, {"台灣", "TW"}, {"台北", "TW"}, {"新北", "TW"}, {"彰化", "TW"}, {"TPE", "TW"}, {"TAIWAN", "TW"},
	{"日本", "JP"}, {"东京", "JP"}, {"大阪", "JP"}, {"TYO", "JP"}, {"NRT", "JP"}, {"KIX", "JP"}, {"JAPAN", "JP"},
	{"新加坡", "SG"}, {"狮城", "SG"}, {"獅城", "SG"}, {"SIN", "SG"}, {"SINGAPORE", "SG"},
	{"美国", "US"}, {"美國", "US"}, {"洛杉矶", "US"}, {"圣何塞", "US"}, {"西雅图", "US"}, {"凤凰城", "US"}, {"芝加哥", "US"}, {"纽约", "US"}, {"达拉斯", "US"}, {"圣克拉拉", "US"}, {"堪萨斯", "US"}, {"LAX", "US"}, {"SJC", "US"}, {"SEA", "US"},
	{"韩国", "KR"}, {"韓國", "KR"}, {"首尔", "KR"}, {"春川", "KR"}, {"ICN", "KR"}, {"KOREA", "KR"},
	{"英国", "GB"}, {"英國", "GB"}, {"伦敦", "GB"}, {"LHR", "GB"},
	{"德国", "DE"}, {"德國", "DE"}, {"法兰克福", "DE"}, {"FRA", "DE"},
	{"法国", "FR"}, {"法國", "FR"}, {"巴黎", "FR"}, {"CDG", "FR"},
	{"加拿大", "CA"}, {"温哥华", "CA"}, {"多伦多", "CA"}, {"蒙特利尔", "CA"},
	{"印度", "IN"}, {"孟买", "IN"},
	{"澳大利亚", "AU"}, {"澳洲", "AU"}, {"悉尼", "AU"}, {"墨尔本", "AU"},
	{"俄罗斯", "RU"}, {"莫斯科", "RU"}, {"伯力", "RU"},
	{"荷兰", "NL"}, {"阿姆斯特丹", "NL"}, {"AMS", "NL"},
	{"土耳其", "TR"}, {"伊斯坦布尔", "TR"},
	{"马来西亚", "MY"}, {"馬來西亞", "MY"}, {"吉隆坡", "MY"},
	{"泰国", "TH"}, {"泰國", "TH"}, {"曼谷", "TH"},
	{"越南", "VN"}, {"胡志明", "VN"}, {"河内", "VN"},
	{"菲律宾", "PH"}, {"菲律賓", "PH"}, {"马尼拉", "PH"},
	{"印尼", "ID"}, {"印度尼西亚", "ID"}, {"雅加达", "ID"},
	{"澳门", "MO"}, {"澳門", "MO"},
	{"卢森堡", "LU"}, {"盧森堡", "LU"},
	{"意大利", "IT"}, {"米兰", "IT"},
	{"西班牙", "ES"}, {"马德里", "ES"},
	{"乌克兰", "UA"}, {"基辅", "UA"},
	{"巴西", "BR"}, {"圣保罗", "BR"},
	{"阿根廷", "AR"}, {"布宜诺斯艾利斯", "AR"},
	{"阿联酋", "AE"}, {"迪拜", "AE"}, {"DXB", "AE"},
	{"南非", "ZA"}, {"约翰内斯堡", "ZA"},
}

func inferCountryFromName(name string) string {
	upper := strings.ToUpper(name)
	for _, item := range countryKeywords {
		if strings.Contains(upper, item.kw) {
			return item.code
		}
	}
	// 检查前缀二字母国家码，如 "HK 香港" / "JP-01" / "US_LAX"
	for _, code := range []string{"HK", "TW", "JP", "SG", "US", "KR", "GB", "UK", "DE", "FR", "CA", "AU", "NL", "RU", "TR", "MY", "TH", "VN", "PH", "ID", "IN"} {
		if strings.HasPrefix(upper, code+" ") || strings.HasPrefix(upper, code+"-") || strings.HasPrefix(upper, code+"_") || strings.HasPrefix(upper, "["+code+"]") {
			if code == "UK" {
				return "GB"
			}
			return code
		}
	}
	return ""
}

// drawFlagIcon 在 (x, y) 处绘制 22x15 的精致微缩国旗/地区旗帜图标。
func drawFlagIcon(img *image.RGBA, x, y int, code string) bool {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return false
	}
	r := image.Rect(x, y, x+flagW, y+flagH)
	fillRect := func(x0, y0, x1, y1 int, c color.RGBA) {
		draw.Draw(img, image.Rect(x0, y0, x1, y1).Intersect(r), image.NewUniform(c), image.Point{}, draw.Src)
	}
	fillCircle := func(cx, cy, rad float64, c color.RGBA) {
		for py := y; py < y+flagH; py++ {
			for px := x; px < x+flagW; px++ {
				dx := float64(px) + 0.5 - cx
				dy := float64(py) + 0.5 - cy
				if dx*dx+dy*dy <= rad*rad {
					img.SetRGBA(px, py, c)
				}
			}
		}
	}

	white := rgb(0xFF, 0xFF, 0xFF)
	red := rgb(0xDE, 0x29, 0x10)
	crimson := rgb(0xBC, 0x00, 0x2D)
	blue := rgb(0x00, 0x2B, 0x7F)
	navy := rgb(0x0A, 0x31, 0x61)
	gold := rgb(0xFF, 0xCE, 0x00)
	green := rgb(0x00, 0x84, 0x3D)

	switch code {
	case "JP":
		fillRect(x, y, x+flagW, y+flagH, white)
		fillCircle(float64(x)+float64(flagW)/2, float64(y)+float64(flagH)/2, 4.3, crimson)
	case "HK":
		fillRect(x, y, x+flagW, y+flagH, rgb(0xDE, 0x21, 0x19))
		fillCircle(float64(x)+float64(flagW)/2, float64(y)+float64(flagH)/2, 3.8, white)
		fillCircle(float64(x)+float64(flagW)/2, float64(y)+float64(flagH)/2, 1.5, rgb(0xDE, 0x21, 0x19))
	case "TW":
		fillRect(x, y, x+flagW, y+flagH, rgb(0xFE, 0x00, 0x00))
		fillRect(x, y, x+11, y+8, rgb(0x00, 0x00, 0x95))
		fillCircle(float64(x)+5.5, float64(y)+4.0, 2.3, white)
	case "SG":
		fillRect(x, y, x+flagW, y+flagH/2, rgb(0xEF, 0x33, 0x40))
		fillRect(x, y+flagH/2, x+flagW, y+flagH, white)
		fillCircle(float64(x)+5.2, float64(y)+3.8, 2.5, white)
		fillCircle(float64(x)+6.3, float64(y)+3.8, 2.2, rgb(0xEF, 0x33, 0x40))
	case "US":
		fillRect(x, y, x+flagW, y+flagH, white)
		for sy := 0; sy < flagH; sy += 2 {
			fillRect(x, y+sy, x+flagW, y+sy+1, rgb(0xB2, 0x22, 0x34))
		}
		fillRect(x, y, x+10, y+8, navy)
		for py := y + 2; py < y+7; py += 2 {
			for px := x + 2; px < x+9; px += 3 {
				img.SetRGBA(px, py, white)
			}
		}
	case "KR":
		fillRect(x, y, x+flagW, y+flagH, white)
		cx, cy := float64(x)+float64(flagW)/2, float64(y)+float64(flagH)/2
		for py := y; py < y+flagH; py++ {
			for px := x; px < x+flagW; px++ {
				dx, dy := float64(px)+0.5-cx, float64(py)+0.5-cy
				if dx*dx+dy*dy <= 4.0*4.0 {
					if dy < 0 {
						img.SetRGBA(px, py, rgb(0xCD, 0x2E, 0x3A))
					} else {
						img.SetRGBA(px, py, rgb(0x00, 0x47, 0xA0))
					}
				}
			}
		}
	case "DE":
		fillRect(x, y, x+flagW, y+5, rgb(0x1A, 0x1A, 0x1A))
		fillRect(x, y+5, x+flagW, y+10, rgb(0xDD, 0x00, 0x00))
		fillRect(x, y+10, x+flagW, y+flagH, gold)
	case "FR":
		fillRect(x, y, x+7, y+flagH, rgb(0x00, 0x23, 0x95))
		fillRect(x+7, y, x+15, y+flagH, white)
		fillRect(x+15, y, x+flagW, y+flagH, rgb(0xED, 0x29, 0x39))
	case "IT":
		fillRect(x, y, x+7, y+flagH, rgb(0x00, 0x92, 0x46))
		fillRect(x+7, y, x+15, y+flagH, white)
		fillRect(x+15, y, x+flagW, y+flagH, rgb(0xCE, 0x2B, 0x37))
	case "RU":
		fillRect(x, y, x+flagW, y+5, white)
		fillRect(x, y+5, x+flagW, y+10, rgb(0x00, 0x39, 0xA6))
		fillRect(x, y+10, x+flagW, y+flagH, rgb(0xD5, 0x2B, 0x1E))
	case "NL":
		fillRect(x, y, x+flagW, y+5, rgb(0xAE, 0x1C, 0x28))
		fillRect(x, y+5, x+flagW, y+10, white)
		fillRect(x, y+10, x+flagW, y+flagH, rgb(0x21, 0x46, 0x8B))
	case "LU":
		fillRect(x, y, x+flagW, y+5, rgb(0xEA, 0x14, 0x1D))
		fillRect(x, y+5, x+flagW, y+10, white)
		fillRect(x, y+10, x+flagW, y+flagH, rgb(0x51, 0xAD, 0xDA))
	case "UA":
		fillRect(x, y, x+flagW, y+flagH/2, rgb(0x00, 0x57, 0xB7))
		fillRect(x, y+flagH/2, x+flagW, y+flagH, rgb(0xFF, 0xDD, 0x00))
	case "CA":
		fillRect(x, y, x+6, y+flagH, rgb(0xFF, 0x00, 0x00))
		fillRect(x+6, y, x+16, y+flagH, white)
		fillRect(x+16, y, x+flagW, y+flagH, rgb(0xFF, 0x00, 0x00))
		fillCircle(float64(x)+11.0, float64(y)+7.5, 2.8, rgb(0xFF, 0x00, 0x00))
	case "GB", "UK":
		fillRect(x, y, x+flagW, y+flagH, rgb(0x01, 0x21, 0x69))
		fillRect(x, y+5, x+flagW, y+10, white)
		fillRect(x+8, y, x+14, y+flagH, white)
		fillRect(x, y+6, x+flagW, y+9, rgb(0xC8, 0x10, 0x2E))
		fillRect(x+9, y, x+13, y+flagH, rgb(0xC8, 0x10, 0x2E))
	case "IN":
		fillRect(x, y, x+flagW, y+5, rgb(0xFF, 0x99, 0x33))
		fillRect(x, y+5, x+flagW, y+10, white)
		fillRect(x, y+10, x+flagW, y+flagH, rgb(0x13, 0x88, 0x08))
		fillCircle(float64(x)+11.0, float64(y)+7.5, 1.8, blue)
	case "AU":
		fillRect(x, y, x+flagW, y+flagH, rgb(0x00, 0x00, 0x8B))
		fillRect(x, y+3, x+10, y+5, white)
		fillRect(x+4, y, x+6, y+8, white)
		fillCircle(float64(x)+16.0, float64(y)+9.0, 1.5, white)
	case "TR":
		fillRect(x, y, x+flagW, y+flagH, rgb(0xE3, 0x0A, 0x17))
		fillCircle(float64(x)+9.5, float64(y)+7.5, 3.6, white)
		fillCircle(float64(x)+10.8, float64(y)+7.5, 2.9, rgb(0xE3, 0x0A, 0x17))
	case "VN":
		fillRect(x, y, x+flagW, y+flagH, rgb(0xDA, 0x25, 0x1D))
		fillCircle(float64(x)+11.0, float64(y)+7.5, 3.0, rgb(0xFF, 0xFF, 0x00))
	case "TH":
		fillRect(x, y, x+flagW, y+flagH, rgb(0xA5, 0x19, 0x31))
		fillRect(x, y+2, x+flagW, y+13, white)
		fillRect(x, y+5, x+flagW, y+10, rgb(0x2D, 0x2A, 0x4A))
	case "ID":
		fillRect(x, y, x+flagW, y+flagH/2, rgb(0xFF, 0x00, 0x00))
		fillRect(x, y+flagH/2, x+flagW, y+flagH, white)
	case "MY":
		fillRect(x, y, x+flagW, y+flagH, white)
		for sy := 0; sy < flagH; sy += 2 {
			fillRect(x, y+sy, x+flagW, y+sy+1, rgb(0xCC, 0x00, 0x01))
		}
		fillRect(x, y, x+11, y+8, rgb(0x01, 0x00, 0x66))
		fillCircle(float64(x)+5.5, float64(y)+4.0, 2.2, rgb(0xFF, 0xCC, 0x00))
	case "PH":
		fillRect(x, y, x+flagW, y+flagH/2, rgb(0x00, 0x38, 0xA8))
		fillRect(x, y+flagH/2, x+flagW, y+flagH, rgb(0xCE, 0x11, 0x26))
		fillRect(x, y, x+7, y+flagH, white)
		fillCircle(float64(x)+3.5, float64(y)+7.5, 1.8, rgb(0xFC, 0xD1, 0x16))
	case "MO":
		fillRect(x, y, x+flagW, y+flagH, green)
		fillCircle(float64(x)+11.0, float64(y)+7.5, 3.2, white)
	case "BR":
		fillRect(x, y, x+flagW, y+flagH, rgb(0x00, 0x9B, 0x3A))
		fillCircle(float64(x)+11.0, float64(y)+7.5, 4.2, rgb(0xFE, 0xDF, 0x00))
		fillCircle(float64(x)+11.0, float64(y)+7.5, 2.4, rgb(0x00, 0x27, 0x76))
	case "AR":
		fillRect(x, y, x+flagW, y+5, rgb(0x74, 0xAC, 0xDF))
		fillRect(x, y+5, x+flagW, y+10, white)
		fillRect(x, y+10, x+flagW, y+flagH, rgb(0x74, 0xAC, 0xDF))
		fillCircle(float64(x)+11.0, float64(y)+7.5, 1.8, gold)
	case "ES":
		fillRect(x, y, x+flagW, y+4, red)
		fillRect(x, y+4, x+flagW, y+11, gold)
		fillRect(x, y+11, x+flagW, y+flagH, red)
	default:
		fillRect(x, y, x+flagW, y+flagH, rgb(0x3B, 0x82, 0xF6))
		fillCircle(float64(x)+11.0, float64(y)+7.5, 3.5, white)
	}

	// 1px 浅灰外框，保证白色部分在白底表格上边界清晰
	border := rgb(0xD0, 0xD5, 0xDD)
	for px := x; px < x+flagW; px++ {
		img.SetRGBA(px, y, border)
		img.SetRGBA(px, y+flagH-1, border)
	}
	for py := y; py < y+flagH; py++ {
		img.SetRGBA(x, py, border)
		img.SetRGBA(x+flagW-1, py, border)
	}
	return true
}

// drawFooterIcon 在页脚左侧绘制 20x20 彩色小图标（check / helix / clock）。
func drawFooterIcon(img *image.RGBA, x, y int, kind string) int {
	const sz = 20
	cx, cy := float64(x)+sz/2.0, float64(y)+sz/2.0
	switch kind {
	case "check":
		// 绿色圆角方块 + 白色对勾
		bg := rgb(0x63, 0xC7, 0x6A)
		for py := y + 1; py < y+sz-1; py++ {
			for px := x + 1; px < x+sz-1; px++ {
				img.SetRGBA(px, py, bg)
			}
		}
		// 对勾线段
		drawThickLine(img, float64(x+5), float64(y+10), float64(x+9), float64(y+14), 2.0, rgb(0xFF, 0xFF, 0xFF))
		drawThickLine(img, float64(x+9), float64(y+14), float64(x+15), float64(y+6), 2.0, rgb(0xFF, 0xFF, 0xFF))
	case "helix":
		// 蓝粉双螺旋图标（复刻 MiaoKo/FFQ 版本行前导图标）
		drawThickLine(img, float64(x+3), float64(y+16), float64(x+17), float64(y+4), 2.2, rgb(0x5B, 0x9B, 0xF8))
		drawThickLine(img, float64(x+3), float64(y+4), float64(x+17), float64(y+16), 2.2, rgb(0xF0, 0x78, 0x9C))
		drawThickLine(img, float64(x+6), float64(y+6), float64(x+6), float64(y+14), 1.4, rgb(0x94, 0xA3, 0xB8))
		drawThickLine(img, float64(x+14), float64(y+6), float64(x+14), float64(y+14), 1.4, rgb(0x94, 0xA3, 0xB8))
	case "clock":
		// 圆形时钟图标
		stroke := rgb(0x64, 0x74, 0x8B)
		for py := y; py < y+sz; py++ {
			for px := x; px < x+sz; px++ {
				d := math.Hypot(float64(px)+0.5-cx, float64(py)+0.5-cy)
				if d >= 7.2 && d <= 8.8 {
					img.SetRGBA(px, py, stroke)
				}
			}
		}
		drawThickLine(img, cx, cy, cx, cy-5, 1.6, stroke)
		drawThickLine(img, cx, cy, cx+4, cy+2, 1.6, stroke)
	}
	return sz + 6
}

func drawThickLine(img *image.RGBA, x0, y0, x1, y1, radius float64, c color.RGBA) {
	steps := int(math.Hypot(x1-x0, y1-y0)*2) + 1
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		px := x0 + (x1-x0)*t
		py := y0 + (y1-y0)*t
		ir := int(math.Ceil(radius))
		for dy := -ir; dy <= ir; dy++ {
			for dx := -ir; dx <= ir; dx++ {
				if float64(dx*dx+dy*dy) <= radius*radius {
					img.SetRGBA(int(px)+dx, int(py)+dy, c)
				}
			}
		}
	}
}
