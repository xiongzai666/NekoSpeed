package render

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"math/rand"
	"time"

	"nekospeed/internal/model"
)

// Pattern 背景花纹类型。
type Pattern string

const (
	PatternNone     Pattern = "none"
	PatternDiagonal Pattern = "diagonal"
	PatternDots     Pattern = "dots"
	PatternCross    Pattern = "cross"
)

// Theme 定义一套图片配色方案。
type Theme struct {
	Name string

	ImageBG    color.RGBA // 整图背景
	HeaderBG   color.RGBA // 表头背景
	RowLine    color.RGBA // 行分隔线
	Text       color.RGBA // 正文与标题文字
	FooterText color.RGBA // 页脚文字
	IndexFG    color.RGBA // 序号列文字

	FailBG  color.RGBA // 失败/超时单元格
	ZeroBG  color.RGBA // 速度为 0 单元格
	BadgeBG color.RGBA // 栈徽章（4/6/46）
	Spark   color.RGBA // 每秒速度柱状图
	RateBG  color.RGBA // 底部解锁率汇总行底色

	Watermark color.RGBA // 水印文字颜色（带 Alpha）

	Pattern      Pattern    // 背景花纹类型
	PatternColor color.RGBA // 花纹颜色（建议低透明度）

	LatencyStops []gradStop // 延迟底色渐变
	SpeedStops   []gradStop // 速度底色渐变
	UnlockColors map[model.UnlockStatus]color.RGBA
}

// DefaultTheme 返回经典 MiaoKo / FFQ 浅色主题（白底、浅灰表头、青蓝延迟渐变、粉红速度渐变、柔和果绿解锁）。
func DefaultTheme() Theme {
	return Theme{
		Name:         "default",
		ImageBG:      rgb(0xFF, 0xFF, 0xFF),
		HeaderBG:     rgb(0xEA, 0xEA, 0xEA),
		RowLine:      rgb(0xDC, 0xDC, 0xDC),
		Text:         rgb(0x1A, 0x1A, 0x1A),
		FooterText:   rgb(0x1A, 0x1A, 0x1A),
		IndexFG:      rgb(0x1A, 0x1A, 0x1A),
		FailBG:       rgb(0x88, 0x88, 0x88),
		ZeroBG:       rgb(0xFB, 0xED, 0xF1),
		BadgeBG:      rgb(0x2B, 0x82, 0xC9),
		Spark:        rgb(0xEE, 0x72, 0x99),
		RateBG:       rgb(0xD5, 0xEB, 0xB8),
		Watermark:    color.RGBA{0x0D, 0x0D, 0x0D, 0x1A},
		Pattern:      PatternNone,
		LatencyStops: fixedLatencyStops,
		SpeedStops:   fixedSpeedStops,
		UnlockColors: defaultUnlockColors(),
	}
}

func defaultUnlockColors() map[model.UnlockStatus]color.RGBA {
	return map[model.UnlockStatus]color.RGBA{
		model.StatusUnlock:    rgb(0xBE, 0xE8, 0x84),
		model.StatusPending:   rgb(0xF8, 0xDF, 0x8C),
		model.StatusSentToCN:  rgb(0xF8, 0xDF, 0x8C),
		model.StatusFailed:    rgb(0xEA, 0x6A, 0x6A),
		model.StatusReqFailed: rgb(0xDF, 0x5A, 0x5A),
		model.StatusNA:        rgb(0x8E, 0x94, 0x9E),
	}
}

// hsl 把 HSL(0~360, 0~100, 0~100) 转为 RGB。
func hsl(h, s, l float64) color.RGBA {
	h = math.Mod(h, 360.0)
	if h < 0 {
		h += 360.0
	}
	s /= 100.0
	l /= 100.0
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60.0, 2.0)-1))
	m := l - c/2.0
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return rgb(uint8((r+m)*255), uint8((g+m)*255), uint8((b+m)*255))
}

// hsla 同 hsl，并按预乘 Alpha 通道缩放 R/G/B。
func hsla(h, s, l, a float64) color.RGBA {
	c := hsl(h, s, l)
	return color.RGBA{
		R: uint8(float64(c.R) * a),
		G: uint8(float64(c.G) * a),
		B: uint8(float64(c.B) * a),
		A: uint8(a * 255),
	}
}

// fixedLatencyStops 复刻 FFQ.LA / MiaoKo 青蓝延迟渐变（低延迟冰青 -> 中延迟天青 -> 高延迟深青灰）。
var fixedLatencyStops = []gradStop{
	{0.00, rgb(0xCE, 0xF0, 0xEC)},
	{0.12, rgb(0xA4, 0xE2, 0xE4)},
	{0.30, rgb(0x76, 0xC7, 0xD4)},
	{0.65, rgb(0x56, 0x92, 0x9E)},
	{1.00, rgb(0x46, 0x69, 0x6F)},
}

// fixedSpeedStops 复刻 FFQ.LA / MiaoKo 樱粉-玫红速度渐变。
var fixedSpeedStops = []gradStop{
	{0.00, rgb(0xFA, 0xE6, 0xEC)},
	{0.25, rgb(0xF6, 0xBD, 0xCF)},
	{0.55, rgb(0xF1, 0x8E, 0xAE)},
	{0.85, rgb(0xEC, 0x66, 0x92)},
	{1.00, rgb(0xE6, 0x49, 0x80)},
}

// blueSpeedStops 复刻品云测速冰青-海蓝速度渐变。
var blueSpeedStops = []gradStop{
	{0.00, rgb(0xE6, 0xF9, 0xF6)},
	{0.25, rgb(0x88, 0xF0, 0xE4)},
	{0.55, rgb(0x28, 0xDF, 0xDC)},
	{0.85, rgb(0x25, 0x96, 0xE0)},
	{1.00, rgb(0x1C, 0x7E, 0xD6)},
}

// darkLatencyStops 专为深色主题设计的高对比度深青蓝延迟渐变（搭配亮白文字清晰易读）。
var darkLatencyStops = []gradStop{
	{0.00, rgb(0x14, 0x6B, 0x62)},
	{0.12, rgb(0x18, 0x5E, 0x78)},
	{0.30, rgb(0x1F, 0x4E, 0x78)},
	{0.65, rgb(0x2C, 0x3E, 0x64)},
	{1.00, rgb(0x36, 0x36, 0x4F)},
}

// darkSpeedStops 专为深色主题设计的高对比度深玫红-宝石红速度渐变（搭配亮白文字清晰易读）。
var darkSpeedStops = []gradStop{
	{0.00, rgb(0x4A, 0x25, 0x3B)},
	{0.25, rgb(0x6E, 0x28, 0x4E)},
	{0.55, rgb(0x91, 0x2B, 0x5E)},
	{0.85, rgb(0xB0, 0x2E, 0x69)},
	{1.00, rgb(0xC9, 0x32, 0x72)},
}

func darkUnlockColors() map[model.UnlockStatus]color.RGBA {
	return map[model.UnlockStatus]color.RGBA{
		model.StatusUnlock:    rgb(0x1B, 0x6E, 0x44),
		model.StatusPending:   rgb(0x8A, 0x5A, 0x14),
		model.StatusSentToCN:  rgb(0x8A, 0x5A, 0x14),
		model.StatusFailed:    rgb(0x8C, 0x2A, 0x32),
		model.StatusReqFailed: rgb(0x7A, 0x22, 0x2A),
		model.StatusNA:        rgb(0x33, 0x3B, 0x48),
	}
}

// makeLightTheme 生成干净无底纹的柔和浅色主题。
func makeLightTheme(name string, hue float64, _ Pattern) Theme {
	useBlueSpeed := hue >= 150 && hue <= 250
	speedStops := fixedSpeedStops
	sparkCol := rgb(0xEE, 0x72, 0x99)
	zeroBG := rgb(0xFB, 0xED, 0xF1)
	if useBlueSpeed {
		speedStops = blueSpeedStops
		sparkCol = rgb(0x1C, 0x88, 0xE5)
		zeroBG = rgb(0xF4, 0xF7, 0xFA)
	}
	t := Theme{
		Name:         name,
		ImageBG:      rgb(0xFF, 0xFF, 0xFF),
		HeaderBG:     rgb(0xEA, 0xEA, 0xEA),
		RowLine:      rgb(0xDC, 0xDC, 0xDC),
		Text:         rgb(0x1A, 0x1A, 0x1A),
		FooterText:   rgb(0x1A, 0x1A, 0x1A),
		IndexFG:      rgb(0x1A, 0x1A, 0x1A),
		FailBG:       rgb(0x88, 0x88, 0x88),
		ZeroBG:       zeroBG,
		BadgeBG:      rgb(0x2B, 0x82, 0xC9),
		Spark:        sparkCol,
		RateBG:       rgb(0xD5, 0xEB, 0xB8),
		Watermark:    color.RGBA{0x0D, 0x0D, 0x0D, 0x1A},
		Pattern:      PatternNone,
		LatencyStops: fixedLatencyStops,
		SpeedStops:   speedStops,
		UnlockColors: defaultUnlockColors(),
	}
	return t
}

// makeDarkTheme 生成深色主题，带指定花纹与深色专属高对比度渐变。
func makeDarkTheme(name string, hue float64, pattern Pattern) Theme {
	t := Theme{
		Name:         name,
		ImageBG:      hsl(hue, 14, 13),
		HeaderBG:     hsl(hue, 20, 21),
		RowLine:      hsl(hue, 14, 26),
		Text:         rgb(0xF1, 0xF5, 0xF9),
		FooterText:   hsl(hue, 12, 78),
		IndexFG:      hsl(0, 82, 72),
		FailBG:       rgb(0x8C, 0x2A, 0x32),
		ZeroBG:       hsl(hue, 12, 24),
		BadgeBG:      hsl(hue, 65, 52),
		Spark:        hsl(hue, 75, 62),
		RateBG:       rgb(0x1E, 0x56, 0x3A),
		Watermark:    hsla(hue, 10, 50, 0.14),
		Pattern:      pattern,
		PatternColor: hsla(hue, 15, 32, 0.12),
		LatencyStops: darkLatencyStops,
		SpeedStops:   darkSpeedStops,
		UnlockColors: darkUnlockColors(),
	}
	return t
}

// curatedTheme 用精确 RGB 组装一套主题。
func curatedTheme(name string, imageBG, headerBG, rowLine, text, footerText, indexFG, failBG, zeroBG, badgeBG, spark, watermark color.RGBA, pattern Pattern, patternColor color.RGBA) Theme {
	t := Theme{
		Name:       name,
		ImageBG:    imageBG,
		HeaderBG:   headerBG,
		RowLine:    rowLine,
		Text:       text,
		FooterText: footerText,
		IndexFG:    indexFG,
		FailBG:     failBG,
		ZeroBG:     zeroBG,
		BadgeBG:    badgeBG,
		Spark:      spark,
		Watermark:  watermark,
		Pattern:    pattern,
		PatternColor: patternColor,
		LatencyStops: fixedLatencyStops,
		SpeedStops:   fixedSpeedStops,
		UnlockColors: defaultUnlockColors(),
	}
	return t
}

// themeRegistry 预定义主题列表：16 浅色色相主题 + 4 深色精选 + default。
var themeRegistry = func() []Theme {
	var list []Theme
	lightNames := []string{
		"rose_light", "peach_light", "amber_light", "yellow_light",
		"lime_light", "green_light", "mint_light", "teal_light",
		"cyan_light", "sky_light", "azure_light", "blue_light",
		"indigo_light", "violet_light", "purple_light", "pink_light",
	}
	lightHues := []float64{350, 25, 40, 55, 85, 135, 155, 165, 185, 200, 210, 220, 245, 270, 290, 335}
	lightPatterns := []Pattern{
		PatternDiagonal, PatternDots, PatternDiagonal, PatternDots,
		PatternDiagonal, PatternCross, PatternDots, PatternDiagonal,
		PatternCross, PatternDots, PatternDiagonal, PatternCross,
		PatternDots, PatternDiagonal, PatternCross, PatternDots,
	}
	for i, hue := range lightHues {
		list = append(list, makeLightTheme(lightNames[i], hue, lightPatterns[i]))
	}

	// 深色精选（数量少，作为可选）
	list = append(list, makeDarkTheme("nord_dark", 210, PatternDots))
	list = append(list, makeDarkTheme("tokyo_dark", 220, PatternDiagonal))
	list = append(list, makeDarkTheme("forest_dark", 120, PatternCross))
	list = append(list, makeDarkTheme("sunset_dark", 25, PatternDiagonal))

	list = append(list, DefaultTheme())
	return list
}()

// RandomTheme 返回干净的经典浅色主题（在粉红经典与海蓝经典之间选择，不随机深色背景）。
func RandomTheme() Theme {
	if rand.New(rand.NewSource(time.Now().UnixNano())).Intn(2) == 0 {
		return DefaultTheme()
	}
	return ThemeByName("sky_light")
}

// ThemeByName 按名称查找主题，找不到返回 DefaultTheme。
func ThemeByName(name string) Theme {
	for _, t := range themeRegistry {
		if t.Name == name {
			return t
		}
	}
	return DefaultTheme()
}

// ThemeNames 返回所有主题名称，供 CLI/Web UI 展示。
func ThemeNames() []string {
	names := make([]string, len(themeRegistry))
	for i, t := range themeRegistry {
		names[i] = t.Name
	}
	return names
}

// latencyColor 返回延迟值对应的单元格底色。
func (t Theme) latencyColor(ms int) color.RGBA {
	g := float64(ms) / 2000.0
	if g < 0 {
		g = 0
	}
	if g > 1 {
		g = 1
	}
	return gradient(t.LatencyStops, g)
}

// speedColor 返回速度（字节/秒）对应的单元格底色（以 80MB/s 为满饱和基准，幂函数平滑过渡）。
func (t Theme) speedColor(v float64) color.RGBA {
	if v <= 0 {
		return t.SpeedStops[0].c
	}
	const fullSpeed = 80 * 1024 * 1024 // 80 MB/s
	ratio := v / fullSpeed
	if ratio > 1 {
		ratio = 1
	}
	g := math.Pow(ratio, 0.55)
	return gradient(t.SpeedStops, g)
}

// statusColor 返回解锁状态对应的单元格底色。
func (t Theme) statusColor(s model.UnlockStatus) color.RGBA {
	if c, ok := t.UnlockColors[s]; ok {
		return c
	}
	return t.UnlockColors[model.StatusNA]
}

// textOn 按底色亮度自适应选择文字颜色：浅色底用深灰黑字，深色底用高对比亮白字。
func (t Theme) textOn(bg color.RGBA) color.RGBA {
	lum := 0.299*float64(bg.R) + 0.587*float64(bg.G) + 0.114*float64(bg.B)
	if lum > 140 {
		return rgb(0x1A, 0x1A, 0x1A)
	}
	if t.ImageBG.R > 128 && lum > 70 {
		return rgb(0x1A, 0x1A, 0x1A)
	}
	return color.RGBA{0xF8, 0xFA, 0xFC, 0xFF}
}

// drawPattern 在图片上绘制背景花纹。花纹会先画在整图上，之后再画表格。
func drawPattern(img *image.RGBA, pattern Pattern, c color.RGBA) {
	if pattern == PatternNone || c.A == 0 {
		return
	}
	tile := createPatternTile(pattern, c)
	if tile == nil {
		return
	}
	b := img.Bounds()
	tw, th := tile.Bounds().Dx(), tile.Bounds().Dy()
	for y := b.Min.Y; y < b.Max.Y; y += th {
		for x := b.Min.X; x < b.Max.X; x += tw {
			r := image.Rect(x, y, x+tw, y+th).Intersect(b)
			draw.Draw(img, r, tile, r.Min.Sub(image.Pt(x, y)), draw.Over)
		}
	}
}

// createPatternTile 创建花纹瓦片。
func createPatternTile(pattern Pattern, c color.RGBA) *image.RGBA {
	const size = 32
	tile := image.NewRGBA(image.Rect(0, 0, size, size))
	switch pattern {
	case PatternDiagonal:
		// 对角线，间隔 16px
		for x := -size; x < size*2; x += 16 {
			for i := 0; i < size; i++ {
				xx, yy := x+i, i
				if xx >= 0 && xx < size && yy >= 0 && yy < size {
					tile.SetRGBA(xx, yy, c)
					// 加粗 1px 线
					if xx+1 < size {
						tile.SetRGBA(xx+1, yy, c)
					}
				}
			}
		}
	case PatternDots:
		// 圆点，中心 3x3
		center := size / 2
		r := image.Rect(center-1, center-1, center+2, center+2)
		draw.Draw(tile, r, image.NewUniform(c), image.Point{}, draw.Src)
	case PatternCross:
		// 十字线，间隔 16px
		for x := -size; x < size*2; x += 16 {
			for i := 0; i < size; i++ {
				xx, yy := x+i, i
				if xx >= 0 && xx < size && yy >= 0 && yy < size {
					tile.SetRGBA(xx, yy, c)
				}
				xx2, yy2 := x+i, size-1-i
				if xx2 >= 0 && xx2 < size && yy2 >= 0 && yy2 < size {
					tile.SetRGBA(xx2, yy2, c)
				}
			}
		}
	default:
		return nil
	}
	return tile
}
