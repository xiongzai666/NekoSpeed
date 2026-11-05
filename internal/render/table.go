package render

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"time"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// 版式常量（像素）。
const (
	fontSize    float64 = 26  // 正文字号
	titleSize   float64 = 34  // 图标题字号
	footerSize  float64 = 22  // 页脚字号
	wmSize      float64 = 40  // 水印字号
	cellPadX            = 14  // 单元格水平内边距
	rowHeight           = 50  // 行高（约 字号*1.9）
	margin              = 26  // 整图边距
	tableGap            = 30  // 多表之间的间隔
	maxNameW            = 560 // 节点名称列最大宽（超出截断加 …）
	sparkColW           = 180 // 每秒速度柱状图列宽
	footerLineH         = 36  // 页脚行高
	titleH              = 61  // 图标题块高（titleSize*1.8）
	sectionH            = 46  // 表小标题块高（fontSize*1.8）
	badgeSize           = 31  // 栈徽章边长（fontSize*1.2）
)

// Align 单元格文字对齐方式。
type Align int

const (
	AlignCenter Align = iota
	AlignLeft
)

// colKind 列类型：普通文字 / 节点名称（左对齐+限宽截断）/ 柱状图（定宽）。
type colKind int

const (
	kindText colKind = iota
	kindName
	kindSpark
)

// Col 表格列定义。
type Col struct {
	Header string
	Kind   colKind
	width  int // 计算出的列宽
}

// Cell 单元格。
type Cell struct {
	Text     string
	Flag     string      // 国旗代码（如 "HK"/"JP"/"US"），非空则在文字左侧绘制彩色国旗图标
	BG       color.Color // 整格底色，nil 为白底
	FG       color.Color // 文字色，nil 为自动（随底色反白或黑）
	Align    Align
	Spark    []float64 // 非空则画柱状图而不是文字
	SparkMax float64   // 柱状图参考最高速度（用于统一比例）
	Badge    bool      // Text 为 "4"/"6"/"46"，画蓝色小方块徽章
	Span     int       // 纵向合并：>1 表示向下合并的行数
	Skip     bool      // 被上方合并单元格覆盖
}

// Table 一张表。
type Table struct {
	Section string // 表上方居中的小标题（如 入口/出口），可空
	Cols    []Col
	Rows    [][]Cell
}

// painter 持有整张图共享的绘制状态。
type painter struct {
	img       *image.RGBA
	face      font.Face // 正文
	titleFace font.Face // 图标题
	footFace  font.Face // 页脚
	wmFace    font.Face // 水印
	theme     Theme     // 当前配色主题
}


// renderTables 渲染一张完整图片：图标题 + 若干张表 + 页脚 + 水印。
// footer1 为页脚第一行（各图不同），第二、三行固定。
func renderTables(meta Meta, title, footer1 string, tables []*Table) (*image.RGBA, error) {
	f, err := loadFont(meta.FontPath)
	if err != nil {
		return nil, err
	}
	theme := meta.Theme
	if theme.Name == "" {
		theme = DefaultTheme()
	}
	p := &painter{theme: theme}
	if p.face, err = face(f, fontSize); err != nil {
		return nil, err
	}
	if p.titleFace, err = face(f, titleSize); err != nil {
		return nil, err
	}
	if p.footFace, err = face(f, footerSize); err != nil {
		return nil, err
	}
	if p.wmFace, err = face(f, wmSize); err != nil {
		return nil, err
	}

	// 组装页脚行（带左侧图标类型）
	type footItem struct {
		icon string
		text string
	}
	sortLabel := meta.SortLabel
	if sortLabel == "" {
		sortLabel = "订阅原序"
	}
	trafficStr := "-"
	if meta.TrafficBytes > 0 {
		trafficStr = formatSpeed(float64(meta.TrafficBytes))
	}
	elapsedSec := int(meta.Elapsed.Round(time.Second).Seconds())
	if elapsedSec < 0 {
		elapsedSec = 0
	}
	pingCnt := meta.PingCount
	if pingCnt <= 0 {
		pingCnt = 6
	}
	filterStr := meta.Filter
	if filterStr == "" {
		filterStr = "None"
	}
	threads := meta.Threads
	if threads <= 0 {
		threads = 4
	}
	var footers []footItem
	if footer1 != "" {
		footers = append(footers, footItem{icon: "check", text: footer1})
	}
	footers = append(footers,
		footItem{
			icon: "helix",
			text: fmt.Sprintf("版本=%s 后端=mihomo 消耗流量=%s 线程=↓%d 排序=%s 耗时=%ds Ping=%d次 概要=%d/%d 过滤器=%s",
				meta.Version, trafficStr, threads, sortLabel, elapsedSec, pingCnt, meta.Succeeded, meta.Total, filterStr),
		},
		footItem{
			icon: "clock",
			text: fmt.Sprintf("测试时间=%s (CST) 测试结果仅供参考，以实际情况为准",
				meta.StartTime.Local().Format("2006-01-02T15:04:05")),
		},
	)

	// 计算各表列宽与总宽，并将较窄的表拉伸至与最宽表及页脚等宽（避免页脚被截断）
	maxTW := 760
	for _, item := range footers {
		if fw := p.measureString(p.footFace, item.text) + 44; fw > maxTW {
			maxTW = fw
		}
	}
	for _, t := range tables {
		p.measureTable(t)
		if tw := t.totalWidth(); tw > maxTW {
			maxTW = tw
		}
	}
	for _, t := range tables {
		stretchTable(t, maxTW)
	}
	imgW := maxTW + 2*margin

	// 计算总高
	imgH := margin
	if title != "" {
		imgH += titleH
	}
	for _, t := range tables {
		if t.Section != "" {
			imgH += sectionH
		}
		imgH += rowHeight * (1 + len(t.Rows))
		imgH += tableGap
	}
	imgH += len(footers)*footerLineH + margin

	p.img = image.NewRGBA(image.Rect(0, 0, imgW, imgH))
	draw.Draw(p.img, p.img.Bounds(), image.NewUniform(theme.ImageBG), image.Point{}, draw.Src)
	drawPattern(p.img, theme.Pattern, theme.PatternColor)

	// 图标题
	y := margin
	if title != "" {
		h := titleH
		p.drawText(image.Rect(0, y, imgW, y+h), p.titleFace, title, AlignCenter, theme.Text)
		y += h
	}
	// 各表（水平居中）
	for _, t := range tables {
		x := (imgW - t.totalWidth()) / 2
		y += p.drawTable(t, x, y)
		y += tableGap
	}
	// 页脚（左对齐 + 矢量小图标，复刻原版样式）
	y -= tableGap / 2
	for _, item := range footers {
		iconX := margin + 2
		iconY := y + (footerLineH-20)/2
		drawFooterIcon(p.img, iconX, iconY, item.icon)
		textRect := image.Rect(margin+28-cellPadX, y, imgW-margin, y+footerLineH)
		p.drawText(textRect, p.footFace, item.text, AlignLeft, theme.FooterText)
		y += footerLineH
	}

	// 水印（最上层，斜向平铺）
	drawWatermark(p.img, p.wmFace, sanitize(p.wmFace, meta.Watermark), theme.Watermark)
	return p.img, nil
}

func (t *Table) totalWidth() int {
	w := 0
	for _, c := range t.Cols {
		w += c.width
	}
	return w
}

// stretchTable 将表格列宽平滑扩展至 targetW，使多张表宽度严格对齐。
func stretchTable(t *Table, targetW int) {
	tw := t.totalWidth()
	if tw >= targetW || len(t.Cols) == 0 {
		return
	}
	extra := targetW - tw
	// 若存在"组织"或"节点名称"等弹性宽列，优先分配 55% 剩余宽度
	flexIdx := -1
	for i, c := range t.Cols {
		if c.Header == "组织" || c.Kind == kindName {
			flexIdx = i
			break
		}
	}
	if flexIdx >= 0 && len(t.Cols) > 1 {
		flexAdd := extra * 55 / 100
		t.Cols[flexIdx].width += flexAdd
		extra -= flexAdd
	}
	perCol := extra / len(t.Cols)
	rem := extra % len(t.Cols)
	for i := range t.Cols {
		t.Cols[i].width += perCol
		if i < rem {
			t.Cols[i].width++
		}
	}
}

// measureString 量取（sanitize 后的）文字宽度。
func (p *painter) measureString(fc font.Face, s string) int {
	return font.MeasureString(fc, sanitize(fc, s)).Ceil()
}

// measureTable 自适应计算每列宽度：表头与所有单元格文字宽度 + padding，
// 名称列限宽，柱状图列定宽。
func (p *painter) measureTable(t *Table) {
	for j := range t.Cols {
		c := &t.Cols[j]
		switch c.Kind {
		case kindSpark:
			c.width = sparkColW
			continue
		}
		w := p.measureString(p.face, c.Header) + 2*cellPadX
		for _, row := range t.Rows {
			if j >= len(row) {
				continue
			}
			cell := row[j]
			if cell.Skip || cell.Spark != nil {
				continue
			}
			cw := p.measureString(p.face, cell.Text) + 2*cellPadX
			if cell.Flag != "" {
				cw += flagW + 8
			}
			if cell.Badge && cell.Text == "46" {
				cw += badgeSize + 6 // 两个并排徽章的额外宽度
			}
			if cw > w {
				w = cw
			}
		}
		if c.Kind == kindName && w > maxNameW {
			w = maxNameW
		}
		c.width = w
	}
}

// drawTable 在 (x,y) 处绘制一张表，返回消耗的高度。
func (p *painter) drawTable(t *Table, x, y int) int {
	y0 := y
	tw := t.totalWidth()
	if t.Section != "" {
		h := sectionH
		p.drawText(image.Rect(x, y, x+tw, y+h), p.face, t.Section, AlignCenter, p.theme.Text)
		y += h
	}
	// 表头：浅灰底黑字居中 + 完整网格线
	hdr := image.Rect(x, y, x+tw, y+rowHeight)
	draw.Draw(p.img, hdr, image.NewUniform(p.theme.HeaderBG), image.Point{}, draw.Src)
	p.hline(x, x+tw, y)
	p.vline(x, y, y+rowHeight)
	cx := x
	for j := range t.Cols {
		w := t.Cols[j].width
		r := image.Rect(cx, y, cx+w, y+rowHeight)
		p.drawText(r, p.face, t.Cols[j].Header, AlignCenter, p.theme.Text)
		cx += w
		p.vline(cx-1, y, y+rowHeight)
	}
	y += rowHeight
	// 表头下分隔线
	p.hline(x, x+tw, y-1)

	// 数据行
	for _, row := range t.Rows {
		p.vline(x, y, y+rowHeight)
		cx := x
		for j := range t.Cols {
			if j >= len(row) {
				cx += t.Cols[j].width
				continue
			}
			cell := row[j]
			w := t.Cols[j].width
			cx += w
			if cell.Skip {
				continue
			}
			h := rowHeight
			if cell.Span > 1 {
				h = rowHeight * cell.Span
			}
			r := image.Rect(cx-w, y, cx, y+h)
			p.drawCell(r, cell)
		}
		y += rowHeight
	}
	return y - y0
}

// drawCell 绘制单个单元格：整格底色 + 内容 + 底部与右侧分隔线。
func (p *painter) drawCell(r image.Rectangle, c Cell) {
	if c.BG != nil {
		draw.Draw(p.img, r, image.NewUniform(c.BG), image.Point{}, draw.Src)
	}
	var fg color.Color = p.theme.Text
	if c.FG != nil {
		fg = c.FG
	} else if c.BG != nil {
		fg = p.theme.textOn(toRGBA(c.BG))
	}
	switch {
	case c.Spark != nil:
		drawSparkline(p.img, r, c.Spark, c.SparkMax, p.theme.Spark)
	case c.Badge:
		p.drawBadges(r, c.Text)
	case c.Flag != "":
		fx := r.Min.X + cellPadX
		fy := r.Min.Y + (r.Dy()-flagH)/2
		drawFlagIcon(p.img, fx, fy, c.Flag)
		if c.Text != "" {
			tr := image.Rect(r.Min.X+flagW+8, r.Min.Y, r.Max.X, r.Max.Y)
			p.drawText(tr, p.face, c.Text, c.Align, fg)
		}
	case c.Text != "":
		p.drawText(r, p.face, c.Text, c.Align, fg)
	}
	// 底部与右侧 1px 细灰网格线（合并单元格只在自己的外边界画）
	p.hline(r.Min.X, r.Max.X, r.Max.Y-1)
	p.vline(r.Max.X-1, r.Min.Y, r.Max.Y)
}

// drawBadges 绘制栈徽章：圆角蓝底白字 "4"/"6"，"46" 画两个并排。
func (p *painter) drawBadges(r image.Rectangle, s string) {
	if s == "" {
		return
	}
	size := badgeSize
	gap := 6
	n := len(s)
	total := n*size + (n-1)*gap
	x := r.Min.X + (r.Dx()-total)/2
	y := r.Min.Y + (r.Dy()-size)/2
	for i := 0; i < n; i++ {
		br := image.Rect(x, y, x+size, y+size)
		// 绘制圆角矩形徽章（切掉四角 2px）
		for py := br.Min.Y; py < br.Max.Y; py++ {
			for px := br.Min.X; px < br.Max.X; px++ {
				dx := minInt(px-br.Min.X, br.Max.X-1-px)
				dy := minInt(py-br.Min.Y, br.Max.Y-1-py)
				if dx+dy < 2 {
					continue
				}
				p.img.SetRGBA(px, py, p.theme.BadgeBG)
			}
		}
		p.drawText(br, p.face, string(s[i]), AlignCenter, color.White)
		x += size + gap
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// drawText 在矩形内按对齐方式绘制单行文字（垂直居中），名称列左对齐并限宽截断。
func (p *painter) drawText(r image.Rectangle, fc font.Face, s string, align Align, fg color.Color) {
	s = sanitize(fc, s)
	if s == "" {
		return
	}
	maxW := r.Dx() - 2*cellPadX
	if align == AlignLeft {
		s = truncate(fc, s, maxW)
	}
	w := font.MeasureString(fc, s).Ceil()
	var x int
	switch align {
	case AlignLeft:
		x = r.Min.X + cellPadX
	default:
		x = r.Min.X + (r.Dx()-w)/2
	}
	m := fc.Metrics()
	height := (m.Ascent + m.Descent).Ceil()
	baseY := r.Min.Y + (r.Dy()-height)/2 + m.Ascent.Ceil()
	d := &font.Drawer{
		Dst:  p.img,
		Src:  image.NewUniform(fg),
		Face: fc,
		Dot:  fixed.P(x, baseY),
	}
	d.DrawString(s)
}

// truncate 超出 maxW 时截断并加 …。
func truncate(fc font.Face, s string, maxW int) string {
	if font.MeasureString(fc, s).Ceil() <= maxW {
		return s
	}
	rs := []rune(s)
	for len(rs) > 1 {
		rs = rs[:len(rs)-1]
		if font.MeasureString(fc, string(rs)+"…").Ceil() <= maxW {
			return string(rs) + "…"
		}
	}
	return "…"
}

// hline 画一条 1px 水平分隔线。
func (p *painter) hline(x0, x1, y int) {
	r := image.Rect(x0, y, x1, y+1).Intersect(p.img.Bounds())
	if !r.Empty() {
		draw.Draw(p.img, r, image.NewUniform(p.theme.RowLine), image.Point{}, draw.Src)
	}
}

// vline 画一条 1px 垂直分隔线。
func (p *painter) vline(x, y0, y1 int) {
	r := image.Rect(x, y0, x+1, y1).Intersect(p.img.Bounds())
	if !r.Empty() {
		draw.Draw(p.img, r, image.NewUniform(p.theme.RowLine), image.Point{}, draw.Src)
	}
}

// drawWatermark 整图叠斜向（约 -30°）半透明重复平铺水印。
func drawWatermark(img *image.RGBA, fc font.Face, text string, c color.RGBA) {
	if text == "" {
		return
	}
	m := fc.Metrics()
	w := font.MeasureString(fc, text).Ceil()
	h := (m.Ascent + m.Descent).Ceil()
	pad := 24
	// 把水印文字渲染到小图上
	src := image.NewRGBA(image.Rect(0, 0, w+2*pad, h+2*pad))
	d := &font.Drawer{
		Dst:  src,
		Src:  image.NewUniform(c),
		Face: fc,
		Dot:  fixed.P(pad, pad+m.Ascent.Ceil()),
	}
	d.DrawString(text)

	// 旋转 -25° 进瓦片图（正向仿射变换 s2d：将 src 中心映射到 tile 中心，确保文字完整不裁切）
	ang := -25 * math.Pi / 180
	cos, sin := math.Cos(ang), math.Sin(ang)
	sw, sh := float64(src.Bounds().Dx()), float64(src.Bounds().Dy())
	rotW := int(math.Ceil(sw*math.Abs(cos) + sh*math.Abs(sin)))
	rotH := int(math.Ceil(sw*math.Abs(sin) + sh*math.Abs(cos)))
	tileW, tileH := rotW+180, rotH+160
	tile := image.NewRGBA(image.Rect(0, 0, tileW, tileH))
	cx, cy := sw/2, sh/2
	dx, dy := float64(tileW)/2, float64(tileH)/2
	m6 := [6]float64{
		cos, -sin, dx - cos*cx + sin*cy,
		sin, cos, dy - sin*cx - cos*cy,
	}
	xdraw.ApproxBiLinear.Transform(tile, m6, src, src.Bounds(), xdraw.Over, nil)

	// 平铺
	b := img.Bounds()
	for ty := b.Min.Y - tile.Bounds().Dy(); ty < b.Max.Y; ty += tile.Bounds().Dy() {
		for tx := b.Min.X - tile.Bounds().Dx(); tx < b.Max.X; tx += tile.Bounds().Dx() {
			r := image.Rect(tx, ty, tx+tile.Bounds().Dx(), ty+tile.Bounds().Dy()).Intersect(b)
			draw.Draw(img, r, tile, r.Min.Sub(image.Pt(tx, ty)), draw.Over)
		}
	}
}

func toRGBA(c color.Color) color.RGBA {
	r, g, b, _ := c.RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 0xFF}
}
