package render

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"nekospeed/internal/model"
)

// SaveTopologyImage 渲染线路分析图（*_topology.png）：上表入口、下表出口，
// 自动按入口与落地区域归并并纵向合并连续相同单元格。
func SaveTopologyImage(results []*model.NodeResult, topo *model.Topology, meta Meta, path string) error {
	if topo == nil {
		topo = &model.Topology{}
	}
	if meta.Theme.Name == "" {
		meta.Theme = DefaultTheme()
	}
	inTable, outTable := buildTopologyTables(topo, meta.Theme)

	failed := 0
	for _, r := range results {
		if !r.OK() {
			failed++
		}
	}
	footer1 := fmt.Sprintf("概要=%d->%d，失败=%d", len(results), len(topo.Outbounds), failed)
	img, err := renderTables(meta, LocalISPTitle("NekoSpeed | topo - ", "线路分析", meta.LocalISP, meta.Bandwidth, meta.Single), footer1, []*Table{inTable, outTable})
	if err != nil {
		return err
	}
	return savePNG(img, path)
}

func buildTopologyTables(topo *model.Topology, theme Theme) (*Table, *Table) {
	if topo == nil {
		topo = &model.Topology{}
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
		loc := formatGeoLocation(e.Geo)
		org := formatInboundOrg(e.Geo)
		inTable.Rows = append(inTable.Rows, []Cell{
			{Text: fmt.Sprintf("%d", e.Index), FG: theme.IndexFG, Align: AlignCenter},
			spanInCell(topo.Inbounds, i, func(x model.InboundEntry) string { return formatGeoLocation(x.Geo) }, loc),
			spanInCell(topo.Inbounds, i, func(x model.InboundEntry) string { return formatGeoLocation(x.Geo) + "|" + x.Geo.ASN }, e.Geo.ASN),
			spanInCell(topo.Inbounds, i, func(x model.InboundEntry) string {
				return formatGeoLocation(x.Geo) + "|" + x.Geo.ASN + "|" + formatInboundOrg(x.Geo)
			}, org),
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
	// 复制出口切片并按 (InboundIndex, 地区展示名, ASN, 组织) 稳定排序，使同入口同落地的节点视觉归并，且不改变原切片顺序
	outbounds := make([]model.OutboundEntry, len(topo.Outbounds))
	copy(outbounds, topo.Outbounds)
	sort.SliceStable(outbounds, func(i, j int) bool {
		a, b := outbounds[i], outbounds[j]
		if a.InboundIndex != b.InboundIndex {
			return a.InboundIndex < b.InboundIndex
		}
		locA, locB := formatGeoLocation(a.Geo), formatGeoLocation(b.Geo)
		if locA != locB {
			return locA < locB
		}
		if a.Geo.ASN != b.Geo.ASN {
			return a.Geo.ASN < b.Geo.ASN
		}
		return formatOutboundOrg(a.Geo) < formatOutboundOrg(b.Geo)
	})

	for i, e := range outbounds {
		flagCode, cleanName := extractNodeFlag(e.NodeName)
		if cleanName == "" {
			cleanName = e.NodeName
		}
		loc := formatGeoLocation(e.Geo)
		org := formatOutboundOrg(e.Geo)
		outTable.Rows = append(outTable.Rows, []Cell{
			{Text: fmt.Sprintf("%d", i+1), FG: theme.IndexFG, Align: AlignCenter},
			spanOutCell(outbounds, i, func(x model.OutboundEntry) string {
				return fmt.Sprintf("%d", x.InboundIndex)
			}, fmt.Sprintf("%d", e.InboundIndex)),
			spanOutCell(outbounds, i, func(x model.OutboundEntry) string {
				return fmt.Sprintf("%d|%s", x.InboundIndex, formatGeoLocation(x.Geo))
			}, loc),
			spanOutCell(outbounds, i, func(x model.OutboundEntry) string {
				return fmt.Sprintf("%d|%s|%s", x.InboundIndex, formatGeoLocation(x.Geo), x.Geo.ASN)
			}, e.Geo.ASN),
			spanOutCell(outbounds, i, func(x model.OutboundEntry) string {
				return fmt.Sprintf("%d|%s|%s|%s", x.InboundIndex, formatGeoLocation(x.Geo), x.Geo.ASN, formatOutboundOrg(x.Geo))
			}, org),
			{Text: stackText(e.Geo.Stack), Badge: stackText(e.Geo.Stack) != ""},
			{Text: clusterText(e.Cluster), Align: AlignCenter},
			{Text: cleanName, Flag: flagCode, Align: AlignLeft},
		})
	}
	return inTable, outTable
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

var countryNames = map[string]string{
	// 东亚与港澳台
	"CN": "中国", "HK": "中国香港", "TW": "中国台湾", "MO": "中国澳门",
	"JP": "日本", "KR": "韩国", "KP": "朝鲜", "MN": "蒙古",
	// 东南亚与南亚
	"SG": "新加坡", "MY": "马来西亚", "TH": "泰国", "VN": "越南",
	"PH": "菲律宾", "ID": "印尼", "KH": "柬埔寨", "LA": "老挝",
	"MM": "缅甸", "BN": "文莱", "IN": "印度", "PK": "巴基斯坦",
	"BD": "孟加拉国", "NP": "尼泊尔", "LK": "斯里兰卡",
	// 中亚与西亚/中东
	"KZ": "哈萨克斯坦", "UZ": "乌兹别克斯坦", "KG": "吉尔吉斯斯坦",
	"AE": "阿联酋", "SA": "沙特阿拉伯", "QA": "卡塔尔", "IL": "以色列",
	"TR": "土耳其", "BH": "巴林", "KW": "科威特", "OM": "阿曼",
	"JO": "约旦", "LB": "黎巴嫩", "IQ": "伊拉克", "IR": "伊朗",
	"GE": "格鲁吉亚", "AM": "亚美尼亚", "AZ": "阿塞拜疆", "CY": "塞浦路斯",
	// 欧洲
	"GB": "英国", "UK": "英国", "DE": "德国", "FR": "法国",
	"NL": "荷兰", "RU": "俄罗斯", "PL": "波兰", "FI": "芬兰",
	"UA": "乌克兰", "RO": "罗马尼亚", "CZ": "捷克", "AT": "奥地利",
	"IE": "爱尔兰", "NO": "挪威", "DK": "丹麦", "BE": "比利时",
	"PT": "葡萄牙", "GR": "希腊", "HU": "匈牙利", "BG": "保加利亚",
	"LU": "卢森堡", "IS": "冰岛", "EE": "爱沙尼亚", "LV": "拉脱维亚",
	"LT": "立陶宛", "CH": "瑞士", "SE": "瑞典", "IT": "意大利",
	"ES": "西班牙", "RS": "塞尔维亚", "HR": "克罗地亚", "SK": "斯洛伐克",
	"SI": "斯洛文尼亚", "MD": "摩尔多瓦", "BY": "白俄罗斯", "AL": "阿尔巴尼亚",
	"MK": "北马其顿", "BA": "波黑", "ME": "黑山", "MT": "马耳他",
	"MC": "摩纳哥", "LI": "列支敦士登", "AD": "安道尔", "GI": "直布罗陀",
	"IM": "马恩岛", "JE": "泽西岛", "GG": "根西岛",
	// 美洲
	"US": "美国", "CA": "加拿大", "MX": "墨西哥", "BR": "巴西",
	"AR": "阿根廷", "CL": "智利", "CO": "哥伦比亚", "PE": "秘鲁",
	"PA": "巴拿马", "CR": "哥斯达黎加", "EC": "厄瓜多尔", "UY": "乌拉圭",
	"PY": "巴拉圭", "BO": "玻利维亚", "VE": "委内瑞拉", "DO": "多米尼加",
	"PR": "波多黎各",
	// 非洲与大洋洲
	"NG": "尼日利亚", "ZA": "南非", "EG": "埃及", "MA": "摩洛哥",
	"TN": "突尼斯", "DZ": "阿尔及利亚", "KE": "肯尼亚", "TZ": "坦桑尼亚",
	"GH": "加纳", "ET": "埃塞俄比亚", "MU": "毛里求斯", "SC": "塞舌尔",
	"AO": "安哥拉", "AU": "澳大利亚", "NZ": "新西兰", "GU": "关岛",
	"FJ": "斐济",
}

// localizeCountry 将 ISO-2 国家/地区代码转换为中文展示名称。
func localizeCountry(code string) string {
	norm := strings.ToUpper(strings.TrimSpace(code))
	if norm == "" {
		return ""
	}
	if name, ok := countryNames[norm]; ok {
		return name
	}
	return norm
}

func cleanGeoField(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "未知" || strings.EqualFold(s, "Unknown") || s == "-" {
		return ""
	}
	return s
}

func compactCNAdmin(s string) string {
	s = cleanGeoField(s)
	if s == "" || s == "市辖区" || s == "县" || strings.Contains(s, "直辖县") {
		return ""
	}
	for _, suf := range []string{"壮族自治区", "回族自治区", "维吾尔自治区", "特别行政区", "自治区", "省", "市"} {
		s = strings.TrimSuffix(s, suf)
	}
	return strings.TrimSpace(s)
}

func isRedundantCity(code, countryName, city string) bool {
	if city == "" {
		return true
	}
	if strings.EqualFold(city, countryName) {
		return true
	}
	switch code {
	case "HK":
		return city == "香港" || city == "中国香港" || city == "香港特别行政区" ||
			strings.EqualFold(city, "Hong Kong") || strings.EqualFold(city, "HongKong") || strings.EqualFold(city, "HK")
	case "TW":
		return city == "台湾" || city == "臺灣" || city == "中国台湾" ||
			strings.EqualFold(city, "Taiwan") || strings.EqualFold(city, "TW")
	case "MO":
		return city == "澳门" || city == "澳門" || city == "中国澳门" ||
			strings.EqualFold(city, "Macau") || strings.EqualFold(city, "Macao") || strings.EqualFold(city, "MO")
	case "SG":
		return city == "新加坡" || strings.EqualFold(city, "Singapore") || strings.EqualFold(city, "SG")
	}
	return false
}

// formatGeoLocation 将 GeoInfo 格式化为拓扑图“地区”列展示文本。
func formatGeoLocation(g model.GeoInfo) string {
	code := strings.ToUpper(strings.TrimSpace(g.Country))
	country := localizeCountry(code)
	city := cleanGeoField(g.City)
	region := cleanGeoField(g.Region)

	if code == "CN" {
		prov := compactCNAdmin(region)
		cityCompact := compactCNAdmin(city)
		loc := cityCompact
		if loc == "" || loc == prov {
			loc = prov
		}
		if loc == "" || loc == "中国" {
			return "中国"
		}
		return "中国·" + loc
	}

	if country == "" {
		if city != "" {
			return city
		}
		return region
	}
	if isRedundantCity(code, country, city) {
		return country
	}
	return country + "·" + city
}

func conciseChineseISP(s string) string {
	s = cleanGeoField(s)
	if s == "" {
		return ""
	}
	switch s {
	case "移动", "中国移动":
		return "中国移动"
	case "电信", "中国电信":
		return "中国电信"
	case "联通", "中国联通":
		return "中国联通"
	case "广电", "中国广电":
		return "中国广电"
	case "教育网", "中国教育网":
		return "中国教育网"
	case "铁通", "中国铁通":
		return "中国铁通"
	}
	hasHan := false
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			hasHan = true
			break
		}
	}
	if hasHan && utf8.RuneCountInString(s) <= 12 {
		return s
	}
	return ""
}

func formatInboundOrg(g model.GeoInfo) string {
	code := strings.ToUpper(strings.TrimSpace(g.Country))
	org := cleanGeoField(g.Org)
	isp := cleanGeoField(g.ISP)
	if code == "CN" {
		if concise := conciseChineseISP(isp); concise != "" {
			return concise
		}
		if concise := conciseChineseISP(org); concise != "" {
			return concise
		}
	}
	if org != "" {
		return org
	}
	return isp
}

func formatOutboundOrg(g model.GeoInfo) string {
	if org := cleanGeoField(g.Org); org != "" {
		return org
	}
	return cleanGeoField(g.ISP)
}
