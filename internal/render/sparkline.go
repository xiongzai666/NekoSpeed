package render

import (
	"image"
	"image/color"
	"image/draw"
	"math"
)

// drawSparkline 在给定矩形内绘制双层阶梯面积波形图（复刻 FFQ.LA / MiaoKo / 品云 "每秒速度" 样式）：
// 底层为行峰值高度的浅色半透明带，上层为连续无间断的每秒吞吐阶梯柱，高度相对于全场峰值 refMax 线性缩放。
func drawSparkline(img *image.RGBA, r image.Rectangle, vals []float64, refMax float64, c color.RGBA) {
	if len(vals) == 0 {
		return
	}
	inner := image.Rect(r.Min.X+2, r.Min.Y+6, r.Max.X-2, r.Max.Y-2)
	if inner.Dx() < len(vals) || inner.Dy() < 6 {
		return
	}
	rowMax := 0.0
	for _, v := range vals {
		if v > rowMax {
			rowMax = v
		}
	}
	if rowMax <= 0 {
		return
	}
	if refMax < rowMax {
		refMax = rowMax
	}

	// 计算该行整体占满高度的比例（使用柔和幂缩放，既让高速节点饱满，又让低速节点呈现扁平带）
	rowRatio := math.Pow(rowMax/refMax, 0.72)
	if rowRatio > 1 {
		rowRatio = 1
	}
	if rowRatio < 0.08 {
		rowRatio = 0.08
	}

	lightBand := blendWhite(c, 0.35)
	mainFill := blendWhite(c, 0.82)
	base := inner.Max.Y
	totalH := float64(inner.Dy())

	// 1. 浅色背景峰值带（像参考图中柱体上方的浅粉/浅青色衬底）
	bandH := int(math.Round(rowRatio * totalH))
	if bandH > 2 {
		bandRect := image.Rect(inner.Min.X, base-bandH, inner.Max.X, base)
		draw.Draw(img, bandRect, image.NewUniform(lightBand), image.Point{}, draw.Src)
	}

	// 2. 连续阶梯柱状波形（无竖缝，复刻阶梯面积图观感）
	bw := float64(inner.Dx()) / float64(len(vals))
	for i, v := range vals {
		if v <= 0 {
			continue
		}
		ratio := (v / rowMax) * rowRatio
		h := int(math.Round(ratio * totalH))
		if h < 2 {
			h = 2
		}
		x0 := inner.Min.X + int(math.Round(float64(i)*bw))
		x1 := inner.Min.X + int(math.Round(float64(i+1)*bw))
		if x1 <= x0 {
			x1 = x0 + 1
		}
		bar := image.Rect(x0, base-h, x1, base)
		draw.Draw(img, bar, image.NewUniform(mainFill), image.Point{}, draw.Src)
	}
}

func blendWhite(c color.RGBA, alpha float64) color.RGBA {
	r := uint8(math.Round(255*(1-alpha) + float64(c.R)*alpha))
	g := uint8(math.Round(255*(1-alpha) + float64(c.G)*alpha))
	b := uint8(math.Round(255*(1-alpha) + float64(c.B)*alpha))
	return color.RGBA{R: r, G: g, B: b, A: 0xFF}
}
