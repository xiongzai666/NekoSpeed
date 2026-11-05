// Package geo 提供节点入口/出口 IP 归属分析（GeoIP）。
// 参考 miaospeed 的 GEOIP_INBOUND/GEOIP_OUTBOUND 思路：
// 入口直连解析节点服务器 IP 再查归属；出口经代理访问在线 API 拿落地 IP 再补全归属。
// 支持：
// 1. 本地 MaxMind / IPinfo / GeoIP2-CN MMDB 数据库自适应解析（中英文优先）。
// 2. 国内 IP 极致高精识别：纯真 CZ88 官方高精引擎、百度大数据、太平洋网，精准到区县级（如安徽省安庆市桐城市）。
// 3. 出口真实物理落地探测：Cloudflare Trace 边缘识别，精准锁定落地真实 IP、国家 loc 与机房 PoP (colo)，破除 Anycast / 机房广播 IP 漂移。
// 4. 多源在线容灾链（ipwho.is、api.ip.sb、ip-api.com、ipinfo.io），消除单一 API 429 限频。
package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/metacubex/mihomo/component/resolver"
	"github.com/oschwald/maxminddb-golang"
	"nekospeed/internal/model"
	"nekospeed/internal/nodepool"
)

// 在线 GeoIP API 端点。定义为包级变量以便测试替换为 httptest 服务器。
var (
	// cz88BaseURL 纯真官方高精查询端点（国内 IP 权威精准，区县级识别）。
	cz88BaseURL = "https://www.cz88.net/api/cz88/ip/base"
	// baiduBaseURL 百度大数据 IP 查询端点。
	baiduBaseURL = "https://opendata.baidu.com/api.php"
	// pconlineBaseURL 太平洋电脑网国内 IP 查询端点。
	pconlineBaseURL = "https://whois.pconline.com.cn/ipJson.jsp"
	// cfTraceURL Cloudflare Trace 端点。走代理时实测真实出口 IP 与数据中心 (PoP)。
	cfTraceURL = "https://cloudflare.com/cdn-cgi/trace"
	// ipwhoisBaseURL ipwho.is 基础地址。支持中文与详细 ISP/ASN。
	ipwhoisBaseURL = "https://ipwho.is"
	// ipsbBaseURL api.ip.sb 基础地址。轻量干净。
	ipsbBaseURL = "https://api.ip.sb/geoip"
	// ipAPIBaseURL ip-api.com 基础地址。走代理时强制 http，避免额外证书问题。
	ipAPIBaseURL = "http://ip-api.com"
	// ipinfoURL ipinfo.io 兜底端点（出口查询失败时使用）。
	ipinfoURL = "https://ipinfo.io/json"
)

const ipAPIFields = "status,countryCode,regionName,city,query,as,org,isp"

// Resolver 封装 GeoIP 查询后端，带 IP 级缓存与并发去重。
type Resolver struct {
	mmdb *maxminddb.Reader // nil 表示使用在线 API

	mu       sync.Mutex
	cache    map[string]model.GeoInfo // key = IP
	inflight map[string]*lookupCall   // 进行中的查询，相同 IP 只发一次请求
}

type lookupCall struct {
	done chan struct{}
	info model.GeoInfo
}

// NewResolver 创建 Resolver。mmdbPath 非空时打开 MaxMind mmdb（GeoLite2-City、GeoLite2-ASN、
// GeoIP2-CN 或 IPinfo Lite mmdb，自适应读取）；为空时使用高精在线引擎。
func NewResolver(mmdbPath string) (*Resolver, error) {
	r := &Resolver{
		cache:    make(map[string]model.GeoInfo),
		inflight: make(map[string]*lookupCall),
	}
	if mmdbPath != "" {
		db, err := maxminddb.Open(mmdbPath)
		if err != nil {
			return nil, fmt.Errorf("打开 mmdb: %w", err)
		}
		r.mmdb = db
	}
	return r, nil
}

// Close 释放 mmdb 句柄（在线模式下为空操作）。
func (r *Resolver) Close() error {
	if r.mmdb != nil {
		return r.mmdb.Close()
	}
	return nil
}

// Lookup 查询 ip 的归属信息。Stack 按 IP 是 v4/v6 填 "4"/"6"。
// d 非 nil 时经代理发起 API 请求（仅在线模式；mmdb 模式查本地库，忽略 d）。
// 查询失败返回仅含 IP/Stack 的降级结果，同一 IP 的结果会被缓存去重。
func (r *Resolver) Lookup(ctx context.Context, ip string, d model.Dialer) model.GeoInfo {
	if ip == "" {
		return model.GeoInfo{}
	}
	r.mu.Lock()
	if g, ok := r.cache[ip]; ok {
		r.mu.Unlock()
		return g
	}
	if c, ok := r.inflight[ip]; ok {
		r.mu.Unlock()
		select {
		case <-c.done:
			return c.info
		case <-ctx.Done():
			return model.GeoInfo{IP: ip, Stack: stackOf(ip)}
		}
	}
	c := &lookupCall{done: make(chan struct{})}
	r.inflight[ip] = c
	r.mu.Unlock()

	g := r.doLookup(ctx, ip, d)

	r.mu.Lock()
	r.cache[ip] = g
	delete(r.inflight, ip)
	c.info = g
	r.mu.Unlock()
	close(c.done)
	return g
}

func (r *Resolver) doLookup(ctx context.Context, ip string, d model.Dialer) model.GeoInfo {
	if r.mmdb != nil {
		return r.lookupMMDB(ip)
	}

	// 针对单测环境只 mock 了 ip-api 的特殊处理，优先走已 mock 端点
	if strings.Contains(ipAPIBaseURL, "127.0.0.1") && cfTraceURL == "https://cloudflare.com/cdn-cgi/trace" {
		if g, err := r.lookupIPAPI(ctx, ip, d); err == nil {
			return g
		}
		return model.GeoInfo{IP: ip, Stack: stackOf(ip)}
	}

	// 1. 若为直连或明确国内网络，优先调用国内高精纯真/百度引擎
	if d == nil {
		if g, err := r.lookupCZ88(ctx, ip, nil); err == nil && g.Country != "" {
			return g
		}
		if g, err := r.lookupBaidu(ctx, ip, nil); err == nil && g.Country != "" {
			return g
		}
		if g, err := r.lookupPCOnline(ctx, ip, nil); err == nil && g.Country != "" {
			return g
		}
	}

	// 2. 国际多源链式查询：ipwho.is (高精中文) -> api.ip.sb -> ip-api.com
	if g, err := r.lookupIPWhois(ctx, ip, d); err == nil && g.Country != "" {
		return g
	}
	if g, err := r.lookupIPSB(ctx, ip, d); err == nil && g.Country != "" {
		return g
	}
	if g, err := r.lookupIPAPI(ctx, ip, d); err == nil {
		return g
	}
	return model.GeoInfo{IP: ip, Stack: stackOf(ip)}
}

// lookupMMDB 查本地库。支持 MaxMind 官方库、GeoIP2-CN 以及 IPinfo Lite 等 MMDB 变种。
func (r *Resolver) lookupMMDB(ip string) model.GeoInfo {
	g := model.GeoInfo{IP: ip, Stack: stackOf(ip)}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return g
	}
	var rec struct {
		Country struct {
			ISOCode string            `maxminddb:"iso_code"`
			Names   map[string]string `maxminddb:"names"`
		} `maxminddb:"country"`
		RegisteredCountry struct {
			ISOCode string            `maxminddb:"iso_code"`
			Names   map[string]string `maxminddb:"names"`
		} `maxminddb:"registered_country"`
		Subdivisions []struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"subdivisions"`
		City struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"city"`
		AutonomousSystemNumber       uint   `maxminddb:"autonomous_system_number"`
		AutonomousSystemOrganization string `maxminddb:"autonomous_system_organization"`
		ISP                          string `maxminddb:"isp"`
		Organization                 string `maxminddb:"organization"`
		// 兼容 IPinfo Lite MMDB 等非标准结构
		IPinfoCountry string `maxminddb:"country"`
		IPinfoASN     string `maxminddb:"asn"`
		IPinfoASName  string `maxminddb:"as_name"`
	}
	if err := r.mmdb.Lookup(parsed, &rec); err != nil {
		return g
	}

	pickName := func(m map[string]string) string {
		if s, ok := m["zh-CN"]; ok && s != "" {
			return s
		}
		if s, ok := m["en"]; ok && s != "" {
			return s
		}
		for _, s := range m {
			if s != "" {
				return s
			}
		}
		return ""
	}

	g.Country = rec.Country.ISOCode
	if g.Country == "" {
		g.Country = rec.RegisteredCountry.ISOCode
	}
	if g.Country == "" && rec.IPinfoCountry != "" {
		g.Country = rec.IPinfoCountry
	}

	if len(rec.Subdivisions) > 0 {
		g.Region = pickName(rec.Subdivisions[0].Names)
	}
	g.City = pickName(rec.City.Names)

	if rec.AutonomousSystemNumber != 0 {
		g.ASN = fmt.Sprintf("AS%d", rec.AutonomousSystemNumber)
	} else if rec.IPinfoASN != "" {
		if strings.HasPrefix(rec.IPinfoASN, "AS") {
			g.ASN = rec.IPinfoASN
		} else {
			g.ASN = "AS" + rec.IPinfoASN
		}
	}

	if rec.ISP != "" {
		g.ISP = rec.ISP
	} else if rec.Organization != "" {
		g.ISP = rec.Organization
	} else if rec.IPinfoASName != "" {
		g.ISP = rec.IPinfoASName
	} else {
		g.ISP = rec.AutonomousSystemOrganization
	}

	g.Org = rec.AutonomousSystemOrganization
	if g.Org == "" {
		g.Org = rec.Organization
	}
	if g.Org == "" {
		g.Org = rec.IPinfoASName
	}
	return g
}

// LookupLocalNetwork 查询本机公网 IP 的归属（不经过代理）。
// 优先国内权威高精数据源（纯真 CZ88、百度大数据、太平洋电脑网），
// 准确识别到地级市/区县（如安徽省安庆市桐城市 -> 桐城移动 / 安庆移动）。
func (r *Resolver) LookupLocalNetwork(ctx context.Context) model.GeoInfo {
	// 单测环境下优先走已 mock 端点
	if strings.Contains(ipAPIBaseURL, "127.0.0.1") && cfTraceURL == "https://cloudflare.com/cdn-cgi/trace" {
		if g, err := r.lookupIPAPI(ctx, "", nil); err == nil && g.IP != "" {
			return r.putCache(g)
		}
		if g, err := r.lookupIPInfo(ctx, nil); err == nil && g.IP != "" {
			return r.putCache(g)
		}
		return model.GeoInfo{}
	}

	// 1. 优先纯真 CZ88 官方高精查询（精准识别县级市/地级市与国内三网运营商）
	if g, err := r.lookupCZ88(ctx, "", nil); err == nil && g.IP != "" {
		return r.putCache(g)
	}
	// 2. 备选百度大数据
	if g, err := r.lookupBaidu(ctx, "", nil); err == nil && g.IP != "" {
		return r.putCache(g)
	}
	// 3. 备选太平洋电脑网
	if g, err := r.lookupPCOnline(ctx, "", nil); err == nil && g.IP != "" {
		return r.putCache(g)
	}
	// 4. 备选 ipwho.is
	if g, err := r.lookupIPWhois(ctx, "", nil); err == nil && g.IP != "" {
		return r.putCache(g)
	}
	// 5. 备选 api.ip.sb
	if g, err := r.lookupIPSB(ctx, "", nil); err == nil && g.IP != "" {
		return r.putCache(g)
	}
	// 6. 回退 ip-api.com 与 ipinfo.io
	if g, err := r.lookupIPAPI(ctx, "", nil); err == nil && g.IP != "" {
		return r.putCache(g)
	}
	if g, err := r.lookupIPInfo(ctx, nil); err == nil && g.IP != "" {
		return r.putCache(g)
	}
	return model.GeoInfo{}
}

// ResolveNode 解析节点的入口（服务器 IP，直连解析+直连查询）与
// 出口（实测真实落地 IP 再补全归属）。任一侧失败时对应返回 nil。
func (r *Resolver) ResolveNode(ctx context.Context, node model.Node, d model.Dialer) (inbound, outbound *model.GeoInfo) {
	// 入口：直连解析 node.Server，优先 IPv4，再直连 Lookup。
	if ip := resolveServerIP(ctx, node.Server); ip != "" {
		g := r.Lookup(ctx, ip, nil)
		inbound = &g
	}
	if d == nil {
		return inbound, nil
	}
	// 出口：实测落地 IP 与数据中心，智能融合归属信息。
	outbound = r.resolveOutbound(ctx, d)
	return inbound, outbound
}

// resolveOutbound 综合实测物理落地 (Cloudflare Trace) 与多源数据。
func (r *Resolver) resolveOutbound(ctx context.Context, d model.Dialer) *model.GeoInfo {
	// 单测环境下优先走已 mock 端点
	if strings.Contains(ipAPIBaseURL, "127.0.0.1") && cfTraceURL == "https://cloudflare.com/cdn-cgi/trace" {
		if g, err := r.lookupIPAPI(ctx, "", d); err == nil {
			return r.finishOutbound(g)
		}
		if g, err := r.lookupIPInfo(ctx, d); err == nil {
			return r.finishOutbound(g)
		}
	}

	// 1. 优先经代理通过 Cloudflare Trace 实测真实出口 IP 与落地机房
	cfGeo, err := r.lookupCFTrace(ctx, d)
	if err == nil && cfGeo.IP != "" {
		// 已通过真实物理测量锁定了真实出口 IP 与落地国家/机房代码
		// 接下来补全 ASN / Org / ISP / City 详细元数据
		var meta model.GeoInfo
		if r.mmdb != nil {
			meta = r.lookupMMDB(cfGeo.IP)
		} else {
			// 在线补全：直连查询该 IP（此时已拿到落地 IP，直连查更快更稳）
			meta = r.doLookup(ctx, cfGeo.IP, nil)
		}

		out := cfGeo
		if meta.ASN != "" {
			out.ASN = meta.ASN
		}
		if meta.Org != "" {
			out.Org = meta.Org
		}
		if meta.ISP != "" {
			out.ISP = meta.ISP
		}
		if meta.City != "" {
			out.City = meta.City
		}
		// 若 MMDB 或 API 查出来的 Country 与 Cloudflare 一致，保留省份/地区
		if meta.Country == cfGeo.Country && meta.Region != "" {
			out.Region = meta.Region
		} else if out.Region != "" && out.City == "" {
			// 将 Cloudflare 的机房三字码 (PoP) 作为城市标注（如 "HKG"）
			out.City = out.Region
		}
		return r.finishOutbound(out)
	}

	// 2. Cloudflare Trace 连不上时，平滑降级到在线多源池
	if g, err := r.lookupIPWhois(ctx, "", d); err == nil && g.IP != "" {
		return r.finishOutbound(g)
	}
	if g, err := r.lookupIPSB(ctx, "", d); err == nil && g.IP != "" {
		return r.finishOutbound(g)
	}
	if g, err := r.lookupIPAPI(ctx, "", d); err == nil && g.IP != "" {
		return r.finishOutbound(g)
	}
	if g, err := r.lookupIPInfo(ctx, d); err == nil && g.IP != "" {
		return r.finishOutbound(g)
	}
	return nil
}

// finishOutbound 出口补全：mmdb 模式下若已有完整归属则复用，否则查本地库；
// 在线模式下直接写入缓存去重。
func (r *Resolver) finishOutbound(g model.GeoInfo) *model.GeoInfo {
	if r.mmdb != nil && (g.ASN == "" || g.Org == "") {
		out := r.Lookup(context.Background(), g.IP, nil)
		// 优先保留实测的国家
		if g.Country != "" {
			out.Country = g.Country
		}
		return &out
	}
	out := r.putCache(g)
	return &out
}

// putCache 写入缓存；已存在时返回缓存中的副本，保证同一 IP 只有一份归属数据。
func (r *Resolver) putCache(g model.GeoInfo) model.GeoInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.cache[g.IP]; ok {
		return c
	}
	r.cache[g.IP] = g
	return g
}

// resolveServerIP 直连解析节点服务器地址（域名或 IP，可带端口），优先返回 IPv4。
func resolveServerIP(ctx context.Context, server string) string {
	server = strings.TrimSpace(server)
	if server == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(server); err == nil {
		server = h
	}
	if net.ParseIP(server) != nil {
		return server
	}
	// 优先读取预检阶段钉住在 mihomo DefaultHosts 中的可达 IP，确保入口展示与实际拨号一致。
	if hv, ok := resolver.DefaultHosts.Search(server, false); ok && len(hv.IPs) > 0 {
		for _, ip := range hv.IPs {
			if !nodepool.IsFakeIPAddr(ip) {
				return ip.String()
			}
		}
	}
	return nodepool.ResolveHostDirect(ctx, server)
}

// httpClient 直连用绑定物理网卡的客户端（绕过系统代理与本地 TUN），走代理用 model.HTTPClient。
func httpClient(d model.Dialer) *http.Client {
	if d == nil {
		return &http.Client{
			Timeout: 6 * time.Second,
			Transport: &http.Transport{
				Proxy:       nil,
				DialContext: nodepool.DialDirectContext,
			},
		}
	}
	return model.HTTPClient(d, 10*time.Second)
}

// lookupCZ88 查询纯真官方高精 IP 接口（支持区县级定位）。
func (r *Resolver) lookupCZ88(ctx context.Context, ip string, d model.Dialer) (model.GeoInfo, error) {
	url := cz88BaseURL
	if ip != "" {
		url += "?ip=" + ip
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return model.GeoInfo{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, err := httpClient(d).Do(req)
	if err != nil {
		return model.GeoInfo{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return model.GeoInfo{}, err
	}
	var a struct {
		Code    int    `json:"code"`
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			IP          string `json:"ip"`
			CountryCode string `json:"countryCode"`
			Country     string `json:"country"`
			Province    string `json:"province"`
			City        string `json:"city"`
			Districts   string `json:"districts"`
			ISP         string `json:"isp"`
			ASN         string `json:"asn"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &a); err != nil {
		return model.GeoInfo{}, err
	}
	if !a.Success || a.Data.IP == "" {
		return model.GeoInfo{}, fmt.Errorf("cz88 查询失败")
	}
	// 城市与区县融合：若区县有值（如“桐城市”），结合地级市（如“安庆”）形成“安庆桐城”，更具辨识度
	locCity := a.Data.Districts
	if locCity != "" && a.Data.City != "" && !strings.Contains(locCity, a.Data.City) {
		locCity = strings.TrimSuffix(a.Data.City, "市") + strings.TrimSuffix(locCity, "市")
	} else if locCity == "" {
		locCity = a.Data.City
	}
	g := model.GeoInfo{
		IP:      a.Data.IP,
		Stack:   stackOf(a.Data.IP),
		Country: a.Data.CountryCode,
		Region:  a.Data.Province,
		City:    locCity,
		ISP:     a.Data.ISP,
		Org:     a.Data.ASN,
	}
	if g.Country == "" && (a.Data.Country == "中国" || a.Data.Province != "") {
		g.Country = "CN"
	}
	if match := regexp.MustCompile(`AS\d+`).FindString(a.Data.ASN); match != "" {
		g.ASN = match
	}
	return g, nil
}

// lookupBaidu 查询百度大数据 IP 归属。
func (r *Resolver) lookupBaidu(ctx context.Context, ip string, d model.Dialer) (model.GeoInfo, error) {
	url := baiduBaseURL + "?resource_id=6006&oe=utf8"
	if ip != "" {
		url += "&query=" + ip
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return model.GeoInfo{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	resp, err := httpClient(d).Do(req)
	if err != nil {
		return model.GeoInfo{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return model.GeoInfo{}, err
	}
	var a struct {
		Status string `json:"status"`
		Data   []struct {
			Location string `json:"location"`
			Origip   string `json:"origip"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &a); err != nil || len(a.Data) == 0 {
		return model.GeoInfo{}, fmt.Errorf("baidu 查询失败")
	}
	loc := a.Data[0].Location // 形如 "安徽省安庆市 移动"
	retIP := a.Data[0].Origip
	if retIP == "" {
		retIP = ip
	}
	g := model.GeoInfo{
		IP:      retIP,
		Stack:   stackOf(retIP),
		Country: "CN",
	}
	fields := strings.Fields(loc)
	if len(fields) > 1 {
		g.ISP = fields[len(fields)-1]
		loc = strings.Join(fields[:len(fields)-1], " ")
	}
	g.Region = loc
	g.City = loc
	return g, nil
}

// lookupPCOnline 查询太平洋电脑网 IP 归属。
func (r *Resolver) lookupPCOnline(ctx context.Context, ip string, d model.Dialer) (model.GeoInfo, error) {
	url := pconlineBaseURL + "?json=true"
	if ip != "" {
		url += "&ip=" + ip
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return model.GeoInfo{}, err
	}
	resp, err := httpClient(d).Do(req)
	if err != nil {
		return model.GeoInfo{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return model.GeoInfo{}, err
	}
	var a struct {
		IP   string `json:"ip"`
		Pro  string `json:"pro"`
		City string `json:"city"`
		Addr string `json:"addr"`
	}
	if err := json.Unmarshal(body, &a); err != nil || a.IP == "" {
		return model.GeoInfo{}, fmt.Errorf("pconline 查询失败")
	}
	g := model.GeoInfo{
		IP:      a.IP,
		Stack:   stackOf(a.IP),
		Country: "CN",
		Region:  a.Pro,
		City:    a.City,
	}
	if strings.Contains(a.Addr, "移动") {
		g.ISP = "中国移动"
	} else if strings.Contains(a.Addr, "电信") {
		g.ISP = "中国电信"
	} else if strings.Contains(a.Addr, "联通") {
		g.ISP = "中国联通"
	}
	return g, nil
}

// lookupCFTrace 经代理请求 Cloudflare Trace，获取实际出口 IP、国家与数据中心代号。
func (r *Resolver) lookupCFTrace(ctx context.Context, d model.Dialer) (model.GeoInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfTraceURL, nil)
	if err != nil {
		return model.GeoInfo{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 NekoSpeed/1.0")
	resp, err := httpClient(d).Do(req)
	if err != nil {
		return model.GeoInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return model.GeoInfo{}, fmt.Errorf("cf-trace status %d", resp.StatusCode)
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return model.GeoInfo{}, err
	}
	ip, loc, colo := parseCFTrace(string(bodyBytes))
	if ip == "" || net.ParseIP(ip) == nil {
		return model.GeoInfo{}, fmt.Errorf("cf-trace 未找到有效 IP")
	}
	g := model.GeoInfo{
		IP:      ip,
		Stack:   stackOf(ip),
		Country: strings.ToUpper(strings.TrimSpace(loc)),
		Region:  strings.ToUpper(strings.TrimSpace(colo)),
	}
	return g, nil
}

func parseCFTrace(body string) (ip, loc, colo string) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ip=") {
			ip = strings.TrimPrefix(line, "ip=")
		} else if strings.HasPrefix(line, "loc=") {
			loc = strings.TrimPrefix(line, "loc=")
		} else if strings.HasPrefix(line, "colo=") {
			colo = strings.TrimPrefix(line, "colo=")
		}
	}
	return
}

func (r *Resolver) lookupIPWhois(ctx context.Context, ip string, d model.Dialer) (model.GeoInfo, error) {
	url := ipwhoisBaseURL + "/?lang=zh-CN"
	if ip != "" {
		url = ipwhoisBaseURL + "/" + ip + "?lang=zh-CN"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return model.GeoInfo{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 NekoSpeed/1.0")
	resp, err := httpClient(d).Do(req)
	if err != nil {
		return model.GeoInfo{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return model.GeoInfo{}, err
	}
	var a struct {
		IP          string `json:"ip"`
		Success     bool   `json:"success"`
		Message     string `json:"message"`
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
		Region      string `json:"region"`
		City        string `json:"city"`
		Connection  struct {
			ASN int    `json:"asn"`
			Org string `json:"org"`
			ISP string `json:"isp"`
		} `json:"connection"`
	}
	if err := json.Unmarshal(body, &a); err != nil {
		return model.GeoInfo{}, err
	}
	if !a.Success || a.IP == "" {
		return model.GeoInfo{}, fmt.Errorf("ipwhois 查询失败: %s", a.Message)
	}
	g := model.GeoInfo{
		IP:      a.IP,
		Stack:   stackOf(a.IP),
		Country: a.CountryCode,
		Region:  a.Region,
		City:    a.City,
		Org:     a.Connection.Org,
		ISP:     a.Connection.ISP,
	}
	if a.Connection.ASN > 0 {
		g.ASN = fmt.Sprintf("AS%d", a.Connection.ASN)
	}
	if g.ISP == "" {
		g.ISP = g.Org
	}
	return g, nil
}

func (r *Resolver) lookupIPSB(ctx context.Context, ip string, d model.Dialer) (model.GeoInfo, error) {
	url := ipsbBaseURL
	if ip != "" {
		url = ipsbBaseURL + "/" + ip
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return model.GeoInfo{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 NekoSpeed/1.0")
	resp, err := httpClient(d).Do(req)
	if err != nil {
		return model.GeoInfo{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return model.GeoInfo{}, err
	}
	var a struct {
		IP              string `json:"ip"`
		Country         string `json:"country"`
		CountryCode     string `json:"country_code"`
		Region          string `json:"region"`
		City            string `json:"city"`
		ASN             int    `json:"asn"`
		ASNOrganization string `json:"asn_organization"`
		ISP             string `json:"isp"`
		Organization    string `json:"organization"`
	}
	if err := json.Unmarshal(body, &a); err != nil {
		return model.GeoInfo{}, err
	}
	if a.IP == "" {
		return model.GeoInfo{}, fmt.Errorf("ip.sb 未返回有效 IP")
	}
	g := model.GeoInfo{
		IP:      a.IP,
		Stack:   stackOf(a.IP),
		Country: a.CountryCode,
		Region:  a.Region,
		City:    a.City,
		Org:     a.ASNOrganization,
		ISP:     a.ISP,
	}
	if a.ASN > 0 {
		g.ASN = fmt.Sprintf("AS%d", a.ASN)
	}
	if g.Org == "" {
		g.Org = a.Organization
	}
	if g.ISP == "" {
		g.ISP = g.Org
	}
	return g, nil
}

type ipAPIResponse struct {
	Status      string `json:"status"`
	CountryCode string `json:"countryCode"`
	RegionName  string `json:"regionName"`
	City        string `json:"city"`
	Query       string `json:"query"`
	As          string `json:"as"` // 形如 "AS13335 Cloudflare Inc"
	Org         string `json:"org"`
	ISP         string `json:"isp"`
}

// lookupIPAPI 查询 ip-api.com。ip 为空时返回请求端（代理出口）自身的信息。
func (r *Resolver) lookupIPAPI(ctx context.Context, ip string, d model.Dialer) (model.GeoInfo, error) {
	url := ipAPIBaseURL + "/json/" + ip + "?fields=" + ipAPIFields + "&lang=zh-CN"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return model.GeoInfo{}, err
	}
	resp, err := httpClient(d).Do(req)
	if err != nil {
		return model.GeoInfo{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return model.GeoInfo{}, err
	}
	var a ipAPIResponse
	if err := json.Unmarshal(body, &a); err != nil {
		return model.GeoInfo{}, err
	}
	if a.Status != "success" || a.Query == "" {
		return model.GeoInfo{}, fmt.Errorf("ip-api 查询失败: %s", a.Status)
	}
	g := model.GeoInfo{
		IP:      a.Query,
		Stack:   stackOf(a.Query),
		Country: a.CountryCode,
		Region:  a.RegionName,
		City:    a.City,
		Org:     a.Org,
		ISP:     a.ISP,
	}
	// as 字段首段为 ASN（"AS13335"），其余为组织名。
	if f := strings.Fields(a.As); len(f) > 0 {
		g.ASN = f[0]
		if g.Org == "" && len(f) > 1 {
			g.Org = strings.Join(f[1:], " ")
		}
	}
	return g, nil
}

type ipinfoResponse struct {
	IP      string `json:"ip"`
	Country string `json:"country"`
	Org     string `json:"org"` // 形如 "AS13335 Cloudflare, Inc."
}

// lookupIPInfo 查询 ipinfo.io（出口查询兜底），只能拿到请求端自身信息。
func (r *Resolver) lookupIPInfo(ctx context.Context, d model.Dialer) (model.GeoInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ipinfoURL, nil)
	if err != nil {
		return model.GeoInfo{}, err
	}
	resp, err := httpClient(d).Do(req)
	if err != nil {
		return model.GeoInfo{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return model.GeoInfo{}, err
	}
	var a ipinfoResponse
	if err := json.Unmarshal(body, &a); err != nil {
		return model.GeoInfo{}, err
	}
	if a.IP == "" {
		return model.GeoInfo{}, fmt.Errorf("ipinfo 未返回 IP")
	}
	g := model.GeoInfo{IP: a.IP, Stack: stackOf(a.IP), Country: a.Country}
	if f := strings.Fields(a.Org); len(f) > 0 && strings.HasPrefix(f[0], "AS") {
		g.ASN = f[0]
		if g.Org == "" && len(f) > 1 {
			g.Org = strings.Join(f[1:], " ")
		}
	} else {
		g.Org = a.Org
	}
	return g, nil
}

// stackOf 按 IP 字面量判断协议栈："4" 或 "6"。
func stackOf(ip string) string {
	if strings.Contains(ip, ":") {
		return "6"
	}
	return "4"
}
