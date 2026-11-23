package subscription

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"nekospeed/internal/model"
)

const maxHostHops = 8

// nonProxyTypes 是 Clash 配置里不代表代理节点的 type 值，解析时跳过。
var nonProxyTypes = map[string]bool{
	"direct": true, "reject": true, "rejectdrop": true,
	"dns": true, "compatible": true, "pass": true,
}

// parseClash 尝试把 data 按 Clash 配置解析。
// ok 为 false 表示内容不是含顶层 proxies: 键的 YAML（应继续尝试其他格式）；
// ok 为 true 时返回解析出的节点（err 非空表示 YAML 合法但 proxies 结构损坏）。
func parseClash(data []byte) (nodes []model.Node, ok bool, err error) {
	var top map[string]any
	if err := yaml.Unmarshal(data, &top); err != nil {
		return nil, false, nil // 不是 YAML，交由其他格式处理
	}
	raw, exists := top["proxies"]
	if !exists {
		return nil, false, nil
	}
	list, isList := raw.([]any)
	if !isList {
		return nil, true, fmt.Errorf("Clash 配置的 proxies 字段不是列表")
	}

	hosts := parseClashHosts(top["hosts"])

	for _, item := range list {
		m, isMap := item.(map[string]any)
		if !isMap {
			continue
		}
		node, keep := clashNode(m, len(nodes)+1, hosts)
		if keep {
			nodes = append(nodes, node)
		}
	}
	return nodes, true, nil
}

// clashNode 把 Clash 单节点 map 转为 model.Node；keep 为 false 表示该条应跳过（DIRECT/REJECT 等）。
func clashNode(m map[string]any, index int, hosts *clashHosts) (model.Node, bool) {
	typ, _ := m["type"].(string)
	typ = strings.ToLower(strings.TrimSpace(typ))
	if typ == "" || nonProxyTypes[typ] {
		return model.Node{}, false
	}
	name, _ := m["name"].(string)
	server, _ := m["server"].(string)
	origServer := strings.TrimSuffix(strings.TrimSpace(server), ".")
	if origServer != "" && hosts != nil {
		if mapped := hosts.Resolve(server); mapped != "" && !strings.EqualFold(mapped, origServer) {
			// 若原 server 是域名且未显式配置 SNI，改写 server 前保留原域名作 TLS SNI，避免误伤依赖默认 SNI 的节点。
			if _, err := netip.ParseAddr(origServer); err != nil {
				preserveDefaultSNI(m, typ, origServer)
			}
			server = mapped
			m["server"] = mapped
		}
	}
	if name == "" {
		name = server
	}
	// Raw 直接复用原 map，字段原样交给 mihomo adapter.ParseProxy
	return model.Node{
		Index:  index,
		Name:   name,
		Type:   displayType(typ),
		Server: server,
		Raw:    m,
	}, true
}

func preserveDefaultSNI(m map[string]any, typ, origHost string) {
	switch typ {
	case "anytls", "trojan", "hysteria", "hysteria2", "tuic":
		if !hasNonEmptyString(m["sni"]) {
			m["sni"] = origHost
		}
	case "vmess", "vless":
		if isTruthy(m["tls"]) && !hasNonEmptyString(m["servername"]) && !hasNonEmptyString(m["sni"]) {
			m["servername"] = origHost
		}
	}
}

func hasNonEmptyString(v any) bool {
	s, ok := v.(string)
	return ok && strings.TrimSpace(s) != ""
}

func isTruthy(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		s := strings.ToLower(strings.TrimSpace(x))
		return s != "" && s != "false" && s != "0" && s != "none"
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	default:
		return false
	}
}

type wildcardHostRule struct {
	suffix string
	isPlus bool
	target string
}

type clashHosts struct {
	exact      map[string]string
	singleStar []wildcardHostRule
	multiWild  []wildcardHostRule
}

func parseClashHosts(raw any) *clashHosts {
	m, ok := raw.(map[string]any)
	if !ok || len(m) == 0 {
		return nil
	}
	h := &clashHosts{
		exact: make(map[string]string),
	}
	for rawKey, rawVal := range m {
		target := parseHostTarget(rawVal)
		if target == "" {
			continue
		}
		key := normalizeHostName(rawKey)
		if key == "" {
			continue
		}
		switch {
		case strings.HasPrefix(key, "*."):
			if suffix := strings.TrimPrefix(key, "*."); suffix != "" {
				h.singleStar = append(h.singleStar, wildcardHostRule{suffix: suffix, target: target})
			}
		case strings.HasPrefix(key, "+."):
			if suffix := strings.TrimPrefix(key, "+."); suffix != "" {
				h.multiWild = append(h.multiWild, wildcardHostRule{suffix: suffix, isPlus: true, target: target})
			}
		case strings.HasPrefix(key, "."):
			if suffix := strings.TrimPrefix(key, "."); suffix != "" {
				h.multiWild = append(h.multiWild, wildcardHostRule{suffix: suffix, isPlus: false, target: target})
			}
		default:
			h.exact[key] = target
		}
	}
	if len(h.exact) == 0 && len(h.singleStar) == 0 && len(h.multiWild) == 0 {
		return nil
	}
	sort.Slice(h.singleStar, func(i, j int) bool {
		if len(h.singleStar[i].suffix) != len(h.singleStar[j].suffix) {
			return len(h.singleStar[i].suffix) > len(h.singleStar[j].suffix)
		}
		return h.singleStar[i].suffix < h.singleStar[j].suffix
	})
	sort.Slice(h.multiWild, func(i, j int) bool {
		if len(h.multiWild[i].suffix) != len(h.multiWild[j].suffix) {
			return len(h.multiWild[i].suffix) > len(h.multiWild[j].suffix)
		}
		if h.multiWild[i].suffix != h.multiWild[j].suffix {
			return h.multiWild[i].suffix < h.multiWild[j].suffix
		}
		return !h.multiWild[i].isPlus && h.multiWild[j].isPlus
	})
	return h
}

func normalizeHostName(s string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
}

func normalizeHostTarget(s string) string {
	s = normalizeHostName(s)
	if s == "" || strings.Contains(s, "://") || strings.ContainsAny(s, " \t\r\n/") || s == "system" {
		return ""
	}
	return s
}

func parseHostTarget(v any) string {
	switch x := v.(type) {
	case string:
		return normalizeHostTarget(x)
	case []any:
		var fallback string
		for _, item := range x {
			s, ok := item.(string)
			if !ok {
				continue
			}
			t := normalizeHostTarget(s)
			if t == "" {
				continue
			}
			if ip, err := netip.ParseAddr(t); err == nil {
				if ip.Is4() {
					return t
				}
				if fallback == "" {
					fallback = t
				}
				continue
			}
			if fallback == "" {
				fallback = t
			}
		}
		return fallback
	case []string:
		items := make([]any, len(x))
		for i, s := range x {
			items[i] = s
		}
		return parseHostTarget(items)
	default:
		return ""
	}
}

func (h *clashHosts) lookupOne(domain string) (string, bool) {
	if target, ok := h.exact[domain]; ok {
		return target, true
	}
	for _, r := range h.singleStar {
		if strings.HasSuffix(domain, "."+r.suffix) {
			label := strings.TrimSuffix(domain, "."+r.suffix)
			if label != "" && !strings.Contains(label, ".") {
				return r.target, true
			}
		}
	}
	for _, r := range h.multiWild {
		if r.isPlus && domain == r.suffix {
			return r.target, true
		}
		if strings.HasSuffix(domain, "."+r.suffix) {
			return r.target, true
		}
	}
	return "", false
}

// Resolve 依据 Clash hosts 规则展开域名映射（支持精确匹配、通配符、链式跳转与防循环）。
func (h *clashHosts) Resolve(host string) string {
	orig := normalizeHostName(host)
	if h == nil || orig == "" {
		return host
	}
	if _, err := netip.ParseAddr(orig); err == nil {
		return host
	}
	cur := orig
	visited := map[string]bool{orig: true}
	matched := false
	for hop := 0; hop <= maxHostHops; hop++ {
		next, ok := h.lookupOne(cur)
		if !ok || next == cur {
			if !matched {
				return host
			}
			return cur
		}
		if hop == maxHostHops || visited[next] {
			return host
		}
		matched = true
		cur = next
		visited[cur] = true
		if _, err := netip.ParseAddr(cur); err == nil {
			return cur
		}
	}
	return host
}

// displayType 把 clash 协议 type 转成展示名。
func displayType(typ string) string {
	if d, ok := typeDisplayNames[typ]; ok {
		return d
	}
	return typ
}
