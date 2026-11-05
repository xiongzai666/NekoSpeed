package render

import (
	"fmt"
	"math"

	"nekospeed/internal/model"
)

// platformCols 按平台名列表生成列定义（使用标准展示表头名）。
func platformCols(platforms []string) []Col {
	cols := make([]Col, len(platforms))
	for i, p := range platforms {
		cols[i] = Col{Header: platformDisplayName(p)}
	}
	return cols
}

// unlockRateCell 计算某平台在 results 中的完全解锁（StatusUnlock）百分比单元格。
func unlockRateCell(results []*model.NodeResult, platform string, isAI bool, theme Theme) Cell {
	rateBG := theme.RateBG
	if rateBG.A == 0 {
		rateBG = rgb(0xD5, 0xEB, 0xB8)
	}
	if len(results) == 0 {
		return Cell{Text: "0%", BG: rateBG, Align: AlignCenter}
	}
	unlocked := 0
	for _, r := range results {
		if r == nil {
			continue
		}
		var u *model.UnlockResult
		if isAI {
			u = findUnlock(r.AI, platform)
		} else {
			u = findUnlock(r.Media, platform)
		}
		if u != nil && u.Status == model.StatusUnlock {
			unlocked++
		}
	}
	pct := int(math.Round(float64(unlocked) * 100 / float64(len(results))))
	return Cell{Text: fmt.Sprintf("%d%%", pct), BG: rateBG, Align: AlignCenter}
}

// SaveFullImage 渲染综合图（*_full.png）：测速列 + 流媒体列 + AI 列 + 底部解锁率汇总行。
func SaveFullImage(results []*model.NodeResult, meta Meta, path string) error {
	if meta.Theme.Name == "" {
		meta.Theme = DefaultTheme()
	}
	cols := append(baseCols(),
		Col{Header: "TLS RTT"},
		Col{Header: "HTTPS延迟"},
		Col{Header: "平均速度"},
		Col{Header: "最高速度"},
		Col{Header: "每秒速度", Kind: kindSpark},
	)
	cols = append(cols, platformCols(mediaPlatforms)...)
	cols = append(cols, platformCols(aiPlatforms)...)
	maxSpd := maxTableSpeed(results)
	t := &Table{Cols: cols}
	for _, r := range results {
		row := append(baseCells(r, meta.Theme), latencyCells(r, meta.Theme)...)
		row = append(row, speedCells(r, maxSpd, meta.Theme)...)
		for _, p := range mediaPlatforms {
			row = append(row, unlockCell(r.Media, p, meta.Theme))
		}
		for _, p := range aiPlatforms {
			row = append(row, unlockCell(r.AI, p, meta.Theme))
		}
		t.Rows = append(t.Rows, row)
	}
	if len(results) > 0 {
		summary := make([]Cell, 0, len(cols))
		for i := 0; i < 8; i++ {
			summary = append(summary, Cell{Align: AlignCenter})
		}
		for _, p := range mediaPlatforms {
			summary = append(summary, unlockRateCell(results, p, false, meta.Theme))
		}
		for _, p := range aiPlatforms {
			summary = append(summary, unlockRateCell(results, p, true, meta.Theme))
		}
		t.Rows = append(t.Rows, summary)
	}
	img, err := renderTables(meta, LocalISPTitle("NekoSpeed | fftest - ", "综合测试", meta.LocalISP, meta.Bandwidth, meta.Single), footerLatencyNote, []*Table{t})
	if err != nil {
		return err
	}
	return savePNG(img, path)
}

// SaveMediaImage 渲染流媒体图（*_media.png）：延迟列 + 媒体各列 +（若测试了 AI 则附带 ChatGPT）+ 底部解锁率汇总行。
func SaveMediaImage(results []*model.NodeResult, meta Meta, path string) error {
	if meta.Theme.Name == "" {
		meta.Theme = DefaultTheme()
	}
	hasOpenAI := false
	for _, r := range results {
		if findUnlock(r.AI, "openai") != nil {
			hasOpenAI = true
			break
		}
	}
	cols := append(baseCols(),
		Col{Header: "TLS RTT"},
		Col{Header: "HTTPS延迟"},
	)
	cols = append(cols, platformCols(mediaPlatforms)...)
	if hasOpenAI {
		cols = append(cols, Col{Header: platformDisplayName("openai")})
	}
	t := &Table{Cols: cols}
	for _, r := range results {
		row := append(baseCells(r, meta.Theme), latencyCells(r, meta.Theme)...)
		for _, p := range mediaPlatforms {
			row = append(row, unlockCell(r.Media, p, meta.Theme))
		}
		if hasOpenAI {
			row = append(row, unlockCell(r.AI, "openai", meta.Theme))
		}
		t.Rows = append(t.Rows, row)
	}
	if len(results) > 0 {
		summary := make([]Cell, 0, len(cols))
		for i := 0; i < 5; i++ {
			summary = append(summary, Cell{Align: AlignCenter})
		}
		for _, p := range mediaPlatforms {
			summary = append(summary, unlockRateCell(results, p, false, meta.Theme))
		}
		if hasOpenAI {
			summary = append(summary, unlockRateCell(results, "openai", true, meta.Theme))
		}
		t.Rows = append(t.Rows, summary)
	}
	img, err := renderTables(meta, LocalISPTitle("NekoSpeed | test - ", "基础流媒体", meta.LocalISP, meta.Bandwidth, meta.Single), footerLatencyNote, []*Table{t})
	if err != nil {
		return err
	}
	return savePNG(img, path)
}

// SaveAIImage 渲染 AI 图（*_ai.png）：无延迟列，仅 AI 各列 + 底部解锁率汇总行。
func SaveAIImage(results []*model.NodeResult, meta Meta, path string) error {
	if meta.Theme.Name == "" {
		meta.Theme = DefaultTheme()
	}
	cols := append(baseCols(), platformCols(aiPlatforms)...)
	t := &Table{Cols: cols}
	for _, r := range results {
		row := baseCells(r, meta.Theme)
		for _, p := range aiPlatforms {
			row = append(row, unlockCell(r.AI, p, meta.Theme))
		}
		t.Rows = append(t.Rows, row)
	}
	if len(results) > 0 {
		summary := make([]Cell, 0, len(cols))
		for i := 0; i < 3; i++ {
			summary = append(summary, Cell{Align: AlignCenter})
		}
		for _, p := range aiPlatforms {
			summary = append(summary, unlockRateCell(results, p, true, meta.Theme))
		}
		t.Rows = append(t.Rows, summary)
	}
	img, err := renderTables(meta, LocalISPTitle("NekoSpeed | ai - ", "AI可用性检测", meta.LocalISP, meta.Bandwidth, meta.Single), footerLatencyNote, []*Table{t})
	if err != nil {
		return err
	}
	return savePNG(img, path)
}
