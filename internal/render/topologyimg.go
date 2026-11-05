package render

import (
	"fmt"
	"sort"

	"nekospeed/internal/model"
)

// SaveTopologyImage 渲染线路分析图（*_topology.png）：上表入口、下表出口，
// 自动按入口与落地区域归并并纵向合并连续相同单元格。
func SaveTopologyImage(results []*model.NodeResult, topo *model.Topology, meta Meta, path string) error {
	if meta.Theme.Name == "" {
		meta.Theme = DefaultTheme()
	}
	inTable := &Table{
		Section: "入口",
		Cols: []Col{
			{Header: "序号"},
			{Header: "地区"},
			{Header: "自治编号"},
			{Header: "组织"},
			{Header: "栈"},
			{Header: "簇"},
			{Header: "出口数量"},
		},
	}
	for i, e := range topo.Inbounds {
		cName := localizeCountry(e.Geo.Country)
		inTable.Rows = append(inTable.Rows, []Cell{
			{Text: fmt.Sprintf("%d", e.Index), FG: meta.Theme.IndexFG, Align: AlignCenter},
			spanInCell(topo.Inbounds, i, func(x model.InboundEntry) string { return localizeCountry(x.Geo.Country) }, cName),
			spanInCell(topo.Inbounds, i, func(x model.InboundEntry) string { return localizeCountry(x.Geo.Country) + "|" + x.Geo.ASN }, e.Geo.ASN),
			spanInCell(topo.Inbounds, i, func(x model.InboundEntry) string { return localizeCountry(x.Geo.Country) + "|" + x.Geo.ASN + "|" + x.Geo.Org }, e.Geo.Org),
			{Text: stackText(e.Geo.Stack), Badge: stackText(e.Geo.Stack) != ""},
			{Text: clusterText(e.Cluster), Align: AlignCenter},
			{Text: fmt.Sprintf("%d", e.OutCount), Align: AlignCenter},
		})
	}

	outTable := &Table{
		Section: "出口",
		Cols: []Col{
			{Header: "序号"},
			{Header: "入口"},
			{Header: "地区"},
			{Header: "自治编号"},
			{Header: "组织"},
			{Header: "栈"},
			{Header: "簇"},
			{Header: "节点名称", Kind: kindName},
		},
	}
	// 复制出口切片并按 (InboundIndex, 地区, ASN, 组织) 稳定排序，使同入口同落地的节点视觉归并，且不改变原切片顺序
	outbounds := make([]model.OutboundEntry, len(topo.Outbounds))
	copy(outbounds, topo.Outbounds)
	sort.SliceStable(outbounds, func(i, j int) bool {
		a, b := outbounds[i], outbounds[j]
		if a.InboundIndex != b.InboundIndex {
			return a.InboundIndex < b.InboundIndex
		}
		if a.Geo.Country != b.Geo.Country {
			return a.Geo.Country < b.Geo.Country
		}
		if a.Geo.ASN != b.Geo.ASN {
			return a.Geo.ASN < b.Geo.ASN
		}
		return a.Geo.Org < b.Geo.Org
	})

	for i, e := range outbounds {
		flagCode, cleanName := extractNodeFlag(e.NodeName)
		if cleanName == "" {
			cleanName = e.NodeName
		}
		cName := localizeCountry(e.Geo.Country)
		outTable.Rows = append(outTable.Rows, []Cell{
			{Text: fmt.Sprintf("%d", i+1), FG: meta.Theme.IndexFG, Align: AlignCenter},
			spanOutCell(outbounds, i, func(x model.OutboundEntry) string {
				return fmt.Sprintf("%d", x.InboundIndex)
			}, fmt.Sprintf("%d", e.InboundIndex)),
			spanOutCell(outbounds, i, func(x model.OutboundEntry) string {
				return fmt.Sprintf("%d|%s", x.InboundIndex, localizeCountry(x.Geo.Country))
			}, cName),
			spanOutCell(outbounds, i, func(x model.OutboundEntry) string {
				return fmt.Sprintf("%d|%s|%s", x.InboundIndex, localizeCountry(x.Geo.Country), x.Geo.ASN)
			}, e.Geo.ASN),
			spanOutCell(outbounds, i, func(x model.OutboundEntry) string {
				return fmt.Sprintf("%d|%s|%s|%s", x.InboundIndex, localizeCountry(x.Geo.Country), x.Geo.ASN, x.Geo.Org)
			}, e.Geo.Org),
			{Text: stackText(e.Geo.Stack), Badge: stackText(e.Geo.Stack) != ""},
			{Text: clusterText(e.Cluster), Align: AlignCenter},
			{Text: cleanName, Flag: flagCode, Align: AlignLeft},
		})
	}

	failed := 0
	for _, r := range results {
		if !r.OK() {
			failed++
		}
	}
	footer1 := fmt.Sprintf("概要=%d->%d，失败=%d", len(results), len(outbounds), failed)
	img, err := renderTables(meta, LocalISPTitle("NekoSpeed | topo - ", "线路分析", meta.LocalISP, meta.Bandwidth, meta.Single), footer1, []*Table{inTable, outTable})
	if err != nil {
		return err
	}
	return savePNG(img, path)
}

// spanInCell 计算入口表连续相同分组键的纵向合并单元格。
func spanInCell(list []model.InboundEntry, i int, keyFn func(model.InboundEntry) string, textOpt ...string) Cell {
	k := keyFn(list[i])
	txt := k
	if len(textOpt) > 0 {
		txt = textOpt[0]
	}
	if txt == "" {
		return Cell{Text: "-", Align: AlignCenter}
	}
	if i > 0 && keyFn(list[i-1]) == k {
		return Cell{Skip: true}
	}
	span := 1
	for j := i + 1; j < len(list) && keyFn(list[j]) == k; j++ {
		span++
	}
	return Cell{Text: txt, Align: AlignCenter, Span: span}
}

// spanOutCell 计算出口表连续相同分组键的纵向合并单元格。
func spanOutCell(list []model.OutboundEntry, i int, keyFn func(model.OutboundEntry) string, txt string) Cell {
	if txt == "" {
		return Cell{Text: "-", Align: AlignCenter}
	}
	k := keyFn(list[i])
	if i > 0 && keyFn(list[i-1]) == k {
		return Cell{Skip: true}
	}
	span := 1
	for j := i + 1; j < len(list) && keyFn(list[j]) == k; j++ {
		span++
	}
	return Cell{Text: txt, Align: AlignCenter, Span: span}
}

// clusterText 规范化簇数量展示（<=0 时默认展示 1）。
func clusterText(c int) string {
	if c <= 0 {
		return "1"
	}
	return fmt.Sprintf("%d", c)
}

// stackText 规范化栈标识（"4"/"6"/"46"）。
func stackText(s string) string {
	switch s {
	case "4", "6", "46":
		return s
	case "64":
		return "46"
	default:
		return s
	}
}

// localizeCountry 将 ISO-2 国家/地区代码转换为中文展示名称。
func localizeCountry(code string) string {
	switch code {
	case "HK":
		return "中国香港"
	case "TW":
		return "中国台湾"
	case "MO":
		return "中国澳门"
	case "CN":
		return "中国"
	case "JP":
		return "日本"
	case "SG":
		return "新加坡"
	case "US":
		return "美国"
	case "KR":
		return "韩国"
	case "GB", "UK":
		return "英国"
	case "DE":
		return "德国"
	case "FR":
		return "法国"
	case "NL":
		return "荷兰"
	case "CA":
		return "加拿大"
	case "AU":
		return "澳大利亚"
	case "RU":
		return "俄罗斯"
	case "IN":
		return "印度"
	case "IL":
		return "以色列"
	case "MY":
		return "马来西亚"
	case "TH":
		return "泰国"
	case "VN":
		return "越南"
	case "PH":
		return "菲律宾"
	case "ID":
		return "印尼"
	case "TR":
		return "土耳其"
	case "BR":
		return "巴西"
	case "AR":
		return "阿根廷"
	case "AE":
		return "阿联酋"
	case "CH":
		return "瑞士"
	case "SE":
		return "瑞典"
	case "IT":
		return "意大利"
	case "ES":
		return "西班牙"
	default:
		return code
	}
}
