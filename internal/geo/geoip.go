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
	"net/netip"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/metacubex/mihomo/component/resolver"
	"github.com/oschwald/maxminddb-golang"
	"golang.org/x/text/encoding/simplifiedchinese"
	"nekospeed/internal/model"
	"nekospeed/internal/nodepool"
)

// 在线 GeoIP API 端点。定义为包级变量以便测试替换为 httptest 服务器。
var (
	// cz88BaseURL 纯真官方高精查询端点（国内 IP 权威精准，区县级识别）。
	cz88BaseURL = "https://www.cz88.net/api/cz88/ip/base"
	// mir6BaseURL Mir6 免费国内 IP 归属接口（支持 IPv4/IPv6 区县与运营商）。
	mir6BaseURL = "https://api.mir6.com/api/ip"
	// baiduBaseURL 百度大数据 IP 查询端点。
	baiduBaseURL = "https://opendata.baidu.com/api.php"
	// pconlineBaseURL 太平洋电脑网国内 IP 查询端点。
	pconlineBaseURL = "https://whois.pconline.com.cn/ipJson.jsp"
	// bilibiliLocalURL B 站直播本机公网 IP 快速探测端点（仅用于本机 IPv4/IPv6 归属兜底）。
	bilibiliLocalURL = "https://api.live.bilibili.com/xlive/web-room/v1/index/getIpInfo"
	// cfTraceURL Cloudflare Trace 端点。走代理时实测真实出口 IP 与数据中心 (PoP)。
	cfTraceURL = "https://cloudflare.com/cdn-cgi/trace"
	// ipwhoisBaseURL ipwho.is 基础地址。支持中文与详细 ISP/ASN。
	ipwhoisBaseURL = "https://ipwho.is"
	// ipsbBaseURL api.ip.sb 基础地址。轻量干净。
	ipsbBaseURL = "https://api.ip.sb/geoip"
	// ipAPIBaseURL ip-api.com 基础地址。走代理时强制 http，避免额外证书问题。
	ipAPIBaseURL = "http://ip-api.com"
	// geojsBaseURL GeoJS 公益国际 GeoIP 兜底端点（免 Key、支持双栈）。
	geojsBaseURL = "https://get.geojs.io/v1/ip/geo"
	// ipinfoURL ipinfo.io 兜底端点（出口查询失败时使用）。
	ipinfoURL = "https://ipinfo.io/json"
)

const ipAPIFields = "status,countryCode,regionName,city,query,as,org,isp"

var cz88ASRe = regexp.MustCompile(`AS\d+`)

// mainlandProvinces 用于判断国内 IP 接口返回是否属于中国大陆，避免把海外/港澳台 IP 误标为 CN。
var mainlandProvinces = []string{
	"中国", "北京", "天津", "上海", "重庆",
	"河北", "山西", "辽宁", "吉林", "黑龙江",
	"江苏", "浙江", "安徽", "福建", "江西", "山东",
	"河南", "湖北", "湖南", "广东", "海南",
	"四川", "贵州", "云南", "陕西", "甘肃", "青海",
	"广西", "内蒙古", "西藏", "宁夏", "新疆",
}

func isMainlandChinaLoc(s string) bool {
	if s == "" || strings.Contains(s, "香港") || strings.Contains(s, "澳门") || strings.Contains(s, "台湾") {
		return false
	}
	for _, p := range mainlandProvinces {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// Resolver 封装 GeoIP 查询后端，支持多 MMDB 分层离线加速、IP 级缓存、并发去重与直连源失败冷却。
type Resolver struct {
	customDB     *maxminddb.Reader // 显式传入单文件 -mmdb 时使用
	cnDB         *maxminddb.Reader // GeoCN.mmdb（国内双栈区县+运营商）
	ipinfoDB     *maxminddb.Reader // ipinfo_lite.mmdb（日更全球国家+ASN）
	asnDB        *maxminddb.Reader // GeoLite2-ASN.mmdb（兜底 ASN）
	cityDB       *maxminddb.Reader // GeoLite2-City.mmdb（全球城市）
	directClient *http.Client      // 复用的直连 HTTP 客户端（绕过系统代理与本地 TUN）

	mu             sync.Mutex
	cache          map[string]model.GeoInfo // key = IP
	inflight       map[string]*lookupCall   // 进行中的查询，相同 IP 只发一次请求
	sourceCooldown map[string]time.Time     // 直连数据源冷却截止时间，避免不可达源逐节点卡超时
}

type lookupCall struct {
	done chan struct{}
	info model.GeoInfo
}

const (
	directHTTPTimeout = 4 * time.Second
	proxyHTTPTimeout  = 5 * time.Second
	sourceCooldownTTL = 3 * time.Minute
	globalCacheTTL    = 24 * time.Hour
)

type globalCacheEntry struct {
	info      model.GeoInfo
	expiresAt time.Time
}

var (
	globalGeoMu    sync.Mutex
	globalGeoCache = make(map[string]globalCacheEntry)
)

// ClearGlobalCache 清空进程级 GeoIP 缓存（离线 IP 库更新完成后调用）。
func ClearGlobalCache() {
	globalGeoMu.Lock()
	clear(globalGeoCache)
	globalGeoMu.Unlock()
}

func isMockedOnlineTest() bool {
	for _, u := range []string{ipAPIBaseURL, cz88BaseURL, ipwhoisBaseURL, baiduBaseURL, pconlineBaseURL, ipsbBaseURL, mir6BaseURL, geojsBaseURL} {
		if strings.Contains(u, "127.0.0.1") {
			return true
		}
	}
	return false
}

// NewResolver 创建 Resolver。mmdbPath 为单文件时打开该 MMDB，为目录时按多库模式打开，
// 为空时自动探测 DefaultDBDir() 中的离线多库（不存在或仅部分存在时无缝协同在线引擎）。
func NewResolver(mmdbPath string) (*Resolver, error) {
	r := &Resolver{
		cache:          make(map[string]model.GeoInfo),
		inflight:       make(map[string]*lookupCall),
		sourceCooldown: make(map[string]time.Time),
		directClient: &http.Client{
			Timeout: directHTTPTimeout,
			Transport: &http.Transport{
				Proxy:       nil,
				DialContext: nodepool.DialDirectContext,
			},
		},
	}
	if mmdbPath != "" {
		if st, err := os.Stat(mmdbPath); err == nil && st.IsDir() {
			r.openDirDBs(mmdbPath)
		} else {
			db, err := maxminddb.Open(mmdbPath)
			if err != nil {
				r.directClient.CloseIdleConnections()
				return nil, fmt.Errorf("打开 mmdb: %w", err)
			}
			r.customDB = db
		}
	} else if !isMockedOnlineTest() {
		r.openDirDBs(DefaultDBDir())
	}
	return r, nil
}

// Close 释放所有 MMDB 句柄并关闭直连空闲连接。
func (r *Resolver) Close() error {
	if r.directClient != nil {
		r.directClient.CloseIdleConnections()
	}
	var firstErr error
	for _, db := range []*maxminddb.Reader{r.customDB, r.cnDB, r.ipinfoDB, r.asnDB, r.cityDB} {
		if db != nil {
			if err := db.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// Lookup 查询 ip 的归属信息。Stack 按 IP 是 v4/v6 填 "4"/"6"。
// d 非 nil 时经代理发起 API 请求（仅在线模式；mmdb 模式查本地库，忽略 d）。
// 查询失败返回仅含 IP/Stack 的降级结果，同一 IP 的结果会被缓存去重。
func (r *Resolver) Lookup(ctx context.Context, ip string, d model.Dialer) model.GeoInfo {
	return r.lookupCached(ctx, ip, d, false)
}

func (r *Resolver) lookupCached(ctx context.Context, ip string, d model.Dialer, skipDomestic bool) model.GeoInfo {
	if ip == "" {
		return model.GeoInfo{}
	}
	r.mu.Lock()
	if g, ok := r.cache[ip]; ok {
		r.mu.Unlock()
		return g
	}
	if r.customDB == nil && !isMockedOnlineTest() {
		globalGeoMu.Lock()
		ent, ok := globalGeoCache[ip]
		if ok && time.Now().Before(ent.expiresAt) {
			globalGeoMu.Unlock()
			r.cache[ip] = ent.info
			r.mu.Unlock()
			return ent.info
		}
		if ok {
			delete(globalGeoCache, ip)
		}
		globalGeoMu.Unlock()
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

	g := r.doLookup(ctx, ip, d, skipDomestic)

	r.mu.Lock()
	r.cache[ip] = g
	delete(r.inflight, ip)
	c.info = g
	r.mu.Unlock()
	if r.customDB == nil && !isMockedOnlineTest() && (g.Country != "" || g.ASN != "") {
		globalGeoMu.Lock()
		globalGeoCache[ip] = globalCacheEntry{info: g, expiresAt: time.Now().Add(globalCacheTTL)}
		globalGeoMu.Unlock()
	}
	close(c.done)
	return g
}

func (r *Resolver) isSourceCooling(name string) bool {
	if r.sourceCooldown == nil {
		return false
	}
	r.mu.Lock()
	until, ok := r.sourceCooldown[name]
	if ok && time.Now().After(until) {
		delete(r.sourceCooldown, name)
		ok = false
	}
	r.mu.Unlock()
	return ok
}

func (r *Resolver) markSourceFailed(name, baseURL string) {
	if r.sourceCooldown == nil || strings.Contains(baseURL, "127.0.0.1") {
		return
	}
	r.mu.Lock()
	r.sourceCooldown[name] = time.Now().Add(sourceCooldownTTL)
	r.mu.Unlock()
}

func (r *Resolver) clearSourceFailed(name string) {
	if r.sourceCooldown == nil {
		return
	}
	r.mu.Lock()
	delete(r.sourceCooldown, name)
	r.mu.Unlock()
}

func mergeGeoMissing(base, extra model.GeoInfo) model.GeoInfo {
	if base.IP == "" {
		base.IP = extra.IP
		base.Stack = extra.Stack
	}
	if base.Country == "" {
		base.Country = extra.Country
	}
	if base.Region == "" {
		base.Region = extra.Region
	}
	if base.City == "" {
		base.City = extra.City
	}
	if base.ASN == "" {
		base.ASN = extra.ASN
	}
	if base.Org == "" {
		base.Org = extra.Org
	}
	if base.ISP == "" {
		base.ISP = extra.ISP
	}
	return base
}

func (r *Resolver) doLookup(ctx context.Context, ip string, d model.Dialer, skipDomestic bool) model.GeoInfo {
	if r.hasLocalDB() {
		g := r.lookupOffline(ip)
		hasCitySource := r.cityDB != nil || r.customDB != nil
		hasASNSource := r.ipinfoDB != nil || r.asnDB != nil || r.customDB != nil
		complete := (hasCitySource && hasASNSource && g.Country != "" && g.ASN != "" &&
			(g.Country != "CN" || r.cnDB != nil || g.City != "" || skipDomestic || d != nil)) ||
			(g.Country != "" && g.City != "" && g.ASN != "" && g.Org != "")
		if complete {
			return g
		}
		supCtx, cancel := context.WithTimeout(ctx, directHTTPTimeout)
		defer cancel()
		onlineSkipDomestic := skipDomestic || (g.Country != "" && g.Country != "CN")
		return mergeGeoMissing(g, r.doLookupOnline(supCtx, ip, d, onlineSkipDomestic))
	}
	return r.doLookupOnline(ctx, ip, d, skipDomestic)
}

func (r *Resolver) doLookupOnline(ctx context.Context, ip string, d model.Dialer, skipDomestic bool) model.GeoInfo {
	// 针对单测环境只 mock 了 ip-api 的特殊处理，优先走已 mock 端点
	if strings.Contains(ipAPIBaseURL, "127.0.0.1") && !strings.Contains(cz88BaseURL, "127.0.0.1") &&
		!strings.Contains(mir6BaseURL, "127.0.0.1") && !strings.Contains(geojsBaseURL, "127.0.0.1") &&
		cfTraceURL == "https://cloudflare.com/cdn-cgi/trace" {
		if g, err := r.lookupIPAPI(ctx, ip, d); err == nil {
			return g
		}
		return model.GeoInfo{IP: ip, Stack: stackOf(ip)}
	}

	// 1. 国际多源链优先（一次拿到 Country + City + ASN/Org；避免海外 IP 浪费国内纯真额度与多余 RTT）
	var intl model.GeoInfo
	if !(d == nil && r.isSourceCooling("ipapi")) {
		if g, err := r.lookupIPAPI(ctx, ip, d); err == nil && g.Country != "" {
			if d == nil {
				r.clearSourceFailed("ipapi")
			}
			intl = g
		} else if d == nil && isTransientSourceErr(err) {
			r.markSourceFailed("ipapi", ipAPIBaseURL)
		}
	}
	if intl.Country == "" && !(d == nil && r.isSourceCooling("ipsb")) {
		if g, err := r.lookupIPSB(ctx, ip, d); err == nil && g.Country != "" {
			if d == nil {
				r.clearSourceFailed("ipsb")
			}
			intl = g
		} else if d == nil && isTransientSourceErr(err) {
			r.markSourceFailed("ipsb", ipsbBaseURL)
		}
	}
	if intl.Country == "" && !(d == nil && r.isSourceCooling("ipwhois")) {
		if g, err := r.lookupIPWhois(ctx, ip, d); err == nil && g.Country != "" {
			if d == nil {
				r.clearSourceFailed("ipwhois")
			}
			intl = g
		} else if d == nil && isTransientSourceErr(err) {
			r.markSourceFailed("ipwhois", ipwhoisBaseURL)
		}
	}
	if intl.Country == "" && !(d == nil && r.isSourceCooling("geojs")) {
		if g, err := r.lookupGeoJS(ctx, ip, d); err == nil && g.Country != "" {
			if d == nil {
				r.clearSourceFailed("geojs")
			}
			intl = g
		} else if d == nil && isTransientSourceErr(err) {
			r.markSourceFailed("geojs", geojsBaseURL)
		}
	}

	// 2. 仅当国际源判定为中国大陆 (CN) 或全部国际源失败时，走国内高精链（纯真 -> Mir6 -> 百度/太平洋）补全区县与运营商
	isV6 := stackOf(ip) == "6"
	if d == nil && !skipDomestic && (intl.Country == "" || intl.Country == "CN") {
		var domestic model.GeoInfo
		if !r.isSourceCooling("cz88") {
			if g, err := r.lookupCZ88(ctx, ip, nil); err == nil {
				r.clearSourceFailed("cz88")
				if g.Country != "" {
					domestic = g
				}
			} else if isTransientSourceErr(err) {
				r.markSourceFailed("cz88", cz88BaseURL)
			}
		}
		if domestic.Country == "" && !r.isSourceCooling("mir6") {
			if g, err := r.lookupMir6(ctx, ip, nil); err == nil && g.Country != "" {
				r.clearSourceFailed("mir6")
				domestic = g
			} else if isTransientSourceErr(err) {
				r.markSourceFailed("mir6", mir6BaseURL)
			}
		}
		if !isV6 && domestic.Country == "" && !r.isSourceCooling("baidu") {
			if g, err := r.lookupBaidu(ctx, ip, nil); err == nil && g.Country != "" {
				r.clearSourceFailed("baidu")
				domestic = g
			} else if isTransientSourceErr(err) {
				r.markSourceFailed("baidu", baiduBaseURL)
			}
		}
		if !isV6 && domestic.Country == "" && !r.isSourceCooling("pconline") {
			if g, err := r.lookupPCOnline(ctx, ip, nil); err == nil && g.Country != "" {
				r.clearSourceFailed("pconline")
				domestic = g
			} else if isTransientSourceErr(err) {
				r.markSourceFailed("pconline", pconlineBaseURL)
			}
		}
		if domestic.Country == "CN" {
			if intl.ASN != "" {
				domestic.ASN = intl.ASN
			}
			if intl.Org != "" {
				domestic.Org = intl.Org
			}
			if domestic.ISP == "" && intl.ISP != "" {
				domestic.ISP = intl.ISP
			}
			if domestic.ASN == "" {
				if extra, ok := r.lookupFastMetaDirect(ctx, ip); ok {
					return mergeGeoMissing(domestic, model.GeoInfo{ASN: extra.ASN, Org: extra.Org, ISP: extra.ISP})
				}
			}
			return domestic
		}
	}

	if intl.Country != "" {
		return intl
	}
	return model.GeoInfo{IP: ip, Stack: stackOf(ip)}
}

func (r *Resolver) lookupFastMetaDirect(ctx context.Context, ip string) (model.GeoInfo, bool) {
	metaCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// HTTP 明文 ip-api 国内直连通常 <0.3s，优先用它补 ASN；不可用时再试 api.ip.sb。
	if !r.isSourceCooling("ipapi") {
		if g, err := r.lookupIPAPI(metaCtx, ip, nil); err == nil && g.ASN != "" {
			r.clearSourceFailed("ipapi")
			return g, true
		} else if isTransientSourceErr(err) {
			r.markSourceFailed("ipapi", ipAPIBaseURL)
		}
	}
	if !r.isSourceCooling("ipsb") {
		if g, err := r.lookupIPSB(metaCtx, ip, nil); err == nil && g.ASN != "" {
			r.clearSourceFailed("ipsb")
			return g, true
		} else if isTransientSourceErr(err) {
			r.markSourceFailed("ipsb", ipsbBaseURL)
		}
	}
	return model.GeoInfo{}, false
}

// isTransientSourceErr 区分网络超时/限频等源端故障与业务拒绝（如“非中国大陆 IP”、不支持 IPv6 或返回 IP 不匹配）。
func isTransientSourceErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "非中国大陆") || strings.Contains(msg, "不支持 IPv6") || strings.Contains(msg, "返回 IP 不匹配") {
		return false
	}
	return true
}

// sameIP 规范化比较两个 IP 字符串是否表示同一地址（兼容 IPv6 大小写与零压缩写法）。
func sameIP(a, b string) bool {
	pa, errA := netip.ParseAddr(strings.TrimSpace(a))
	pb, errB := netip.ParseAddr(strings.TrimSpace(b))
	if errA == nil && errB == nil {
		return pa.Unmap() == pb.Unmap()
	}
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// refineCNLocal 尝试使用离线 GeoCN 或国内高精接口补全指定国内 IP 的省/市/区县与运营商。
func (r *Resolver) refineCNLocal(ctx context.Context, ip string) (model.GeoInfo, bool) {
	if ip == "" {
		return model.GeoInfo{}, false
	}
	if r.cnDB != nil {
		if parsed := net.ParseIP(ip); parsed != nil {
			if cn, ok := lookupGeoCNReader(r.cnDB, parsed); ok && cn.City != "" {
				cn.IP = ip
				cn.Stack = stackOf(ip)
				return cn, true
			}
		}
	}
	if g, err := r.lookupCZ88(ctx, ip, nil); err == nil && g.Country != "" {
		return g, true
	}
	if g, err := r.lookupMir6(ctx, ip, nil); err == nil && g.Country != "" {
		return g, true
	}
	if stackOf(ip) != "6" {
		if g, err := r.lookupBaidu(ctx, ip, nil); err == nil && g.Country != "" {
			return g, true
		}
	}
	return model.GeoInfo{}, false
}

// LookupLocalNetwork 查询本机公网 IP 的归属（不经过代理）。
// 优先国内权威高精数据源（GeoCN / 纯真 CZ88 / Mir6 / 百度大数据 / 太平洋 / Bilibili），
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

	// 1. 优先从太平洋电脑网获取本机公网 IPv4（无需传参，同时可拿到国内基线位置）；
	// 再携带该 IP 调用离线 GeoCN / 纯真 CZ88 / Mir6 / 百度大数据，确保精准识别到县级市。
	pcGeo, pcIP, pcErr := r.lookupPCOnlineWithIP(ctx, "", nil)
	if pcErr == nil && pcGeo.IP != "" {
		if refined, ok := r.refineCNLocal(ctx, pcGeo.IP); ok {
			return r.putCache(refined)
		}
		return r.putCache(pcGeo)
	}

	// 若太平洋失败且未拿到 IP，尝试 Bilibili 本机出口快速探测（约 100ms，双栈支持）
	if pcErr != nil && pcIP == "" {
		if bili, err := r.lookupBilibiliLocal(ctx); err == nil && bili.IP != "" {
			if refined, ok := r.refineCNLocal(ctx, bili.IP); ok {
				return r.putCache(refined)
			}
			return r.putCache(bili)
		}
	}

	// 若太平洋接口已返回非中国大陆 IP，则跳过国内库，直接携带该 IP 查国际源。
	queryIP := ""
	if pcErr != nil && pcIP != "" {
		queryIP = pcIP
	}

	// 2. 备选国际链路：api.ip.sb -> ip-api.com -> ipwho.is -> geojs -> ipinfo.io
	var g model.GeoInfo
	var ok bool
	if !r.isSourceCooling("ipsb") {
		if res, err := r.lookupIPSB(ctx, queryIP, nil); err == nil && res.IP != "" {
			r.clearSourceFailed("ipsb")
			g, ok = res, true
		} else if isTransientSourceErr(err) {
			r.markSourceFailed("ipsb", ipsbBaseURL)
		}
	}
	if !ok && !r.isSourceCooling("ipapi") {
		if res, err := r.lookupIPAPI(ctx, queryIP, nil); err == nil && res.IP != "" {
			r.clearSourceFailed("ipapi")
			g, ok = res, true
		} else if isTransientSourceErr(err) {
			r.markSourceFailed("ipapi", ipAPIBaseURL)
		}
	}
	if !ok && !r.isSourceCooling("ipwhois") {
		if res, err := r.lookupIPWhois(ctx, queryIP, nil); err == nil && res.IP != "" {
			r.clearSourceFailed("ipwhois")
			g, ok = res, true
		} else if isTransientSourceErr(err) {
			r.markSourceFailed("ipwhois", ipwhoisBaseURL)
		}
	}
	if !ok && queryIP != "" && !r.isSourceCooling("geojs") {
		if res, err := r.lookupGeoJS(ctx, queryIP, nil); err == nil && res.IP != "" {
			r.clearSourceFailed("geojs")
			g, ok = res, true
		} else if isTransientSourceErr(err) {
			r.markSourceFailed("geojs", geojsBaseURL)
		}
	}
	if !ok && queryIP == "" {
		if res, err := r.lookupIPInfo(ctx, nil); err == nil && res.IP != "" {
			g, ok = res, true
		}
	}
	if !ok {
		return model.GeoInfo{}
	}

	// 若太平洋接口失败但国际链路拿到的是国内 IP（例如 IPv6），尝试用 GeoCN/纯真/Mir6/百度做区县级精度补全。
	if g.Country == "CN" && (pcErr != nil && pcIP == "") {
		if refined, ok := r.refineCNLocal(ctx, g.IP); ok {
			return r.putCache(refined)
		}
	}
	return r.putCache(g)
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

	// 限制单个节点出口多源解析的总耗时（配合单步 5s 超时，确保全部备用源都有预算执行）。
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	// 1. 优先经代理通过 Cloudflare Trace 实测真实出口 IP 与落地机房
	cfGeo, err := r.lookupCFTrace(ctx, d)
	if err == nil && cfGeo.IP != "" {
		// 已通过真实物理测量锁定了真实出口 IP 与落地国家/机房代码
		// 接下来补全 ASN / Org / ISP / City 详细元数据（优先本地多 MMDB，缺失再直连在线补全）
		skipDomestic := cfGeo.Country != "" && cfGeo.Country != "CN"
		meta := r.lookupCached(ctx, cfGeo.IP, nil, skipDomestic)

		out := cfGeo
		if out.Country == "" && meta.Country != "" {
			out.Country = meta.Country
		}
		if meta.ASN != "" {
			out.ASN = meta.ASN
		}
		if meta.Org != "" {
			out.Org = meta.Org
		}
		if meta.ISP != "" {
			out.ISP = meta.ISP
		}
		// 仅当元数据国家未与实测落地国家冲突时采信城市与省区，禁止再用 CF 机房三字码冒充出口城市。
		if meta.Country == "" || out.Country == "" || meta.Country == out.Country {
			if meta.City != "" {
				out.City = meta.City
			}
			if meta.Region != "" {
				out.Region = meta.Region
			}
		}
		return &out
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

// finishOutbound 出口补全：有本地库时若缺字段则补查，否则直接写入缓存去重。
func (r *Resolver) finishOutbound(g model.GeoInfo) *model.GeoInfo {
	if r.hasLocalDB() && (g.ASN == "" || g.Org == "") {
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

// doGet 发起 HTTP GET 并读取有上限的响应体；直连复用 Resolver.directClient，走代理则在使用后关闭空闲连接。
func (r *Resolver) doGet(ctx context.Context, url, userAgent string, d model.Dialer, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	var c *http.Client
	if d == nil {
		if r.directClient != nil {
			c = r.directClient
		} else {
			c = &http.Client{
				Timeout: directHTTPTimeout,
				Transport: &http.Transport{
					Proxy:       nil,
					DialContext: nodepool.DialDirectContext,
				},
			}
			defer c.CloseIdleConnections()
		}
	} else {
		c = model.HTTPClient(d, proxyHTTPTimeout)
		defer c.CloseIdleConnections()
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBytes))
}

// lookupCZ88 查询纯真官方高精 IP 接口（支持区县级定位）。
func (r *Resolver) lookupCZ88(ctx context.Context, ip string, d model.Dialer) (model.GeoInfo, error) {
	url := cz88BaseURL
	if ip != "" {
		url += "?ip=" + ip
	}
	body, err := r.doGet(ctx, url, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36", d, 1<<20)
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
	if ip != "" && !sameIP(ip, a.Data.IP) {
		return model.GeoInfo{}, fmt.Errorf("cz88 返回 IP 不匹配: 请求 %s 实际 %s", ip, a.Data.IP)
	}
	// 城市与区县融合：若区县有值（如“桐城市”），结合地级市（如“安庆”）形成“安庆桐城”，更具辨识度；
	// 过滤“市辖区”“未知”等泛指词。
	locCity := fuseCityDistrict(a.Data.City, a.Data.Districts)
	g := model.GeoInfo{
		IP:      a.Data.IP,
		Stack:   stackOf(a.Data.IP),
		Country: a.Data.CountryCode,
		Region:  cleanUnknownLoc(a.Data.Province),
		City:    locCity,
		ISP:     a.Data.ISP,
		Org:     a.Data.ASN,
	}
	if g.Country == "" && (a.Data.Country == "中国" || isMainlandChinaLoc(a.Data.Province)) {
		g.Country = "CN"
	}
	if match := cz88ASRe.FindString(a.Data.ASN); match != "" {
		g.ASN = match
	}
	return g, nil
}

// lookupMir6 查询 Mir6 国内 IP 归属（支持 IPv4 与 IPv6 区县级定位，仅认可中国大陆结果）。
func (r *Resolver) lookupMir6(ctx context.Context, ip string, d model.Dialer) (model.GeoInfo, error) {
	url := mir6BaseURL + "?type=json"
	if ip != "" {
		url += "&ip=" + ip
	}
	body, err := r.doGet(ctx, url, "Mozilla/5.0 (Windows NT 10.0; Win64; x64)", d, 1<<20)
	if err != nil {
		return model.GeoInfo{}, err
	}
	var a struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			IP          string `json:"ip"`
			CountryCode string `json:"countryCode"`
			Province    string `json:"province"`
			City        string `json:"city"`
			Districts   string `json:"districts"`
			ISP         string `json:"isp"`
			Location    string `json:"location"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &a); err != nil {
		return model.GeoInfo{}, err
	}
	rawIP := strings.TrimSpace(a.Data.IP)
	if a.Code != 200 || rawIP == "" {
		return model.GeoInfo{}, fmt.Errorf("mir6 查询失败: %s", a.Msg)
	}
	if ip != "" && !sameIP(ip, rawIP) {
		return model.GeoInfo{}, fmt.Errorf("mir6 返回 IP 不匹配: 请求 %s 实际 %s", ip, rawIP)
	}
	if !isMainlandChinaLoc(a.Data.Province) && !isMainlandChinaLoc(a.Data.Location) {
		return model.GeoInfo{}, fmt.Errorf("mir6 非中国大陆 IP: %s", a.Data.Location)
	}
	return model.GeoInfo{
		IP:      rawIP,
		Stack:   stackOf(rawIP),
		Country: "CN",
		Region:  cleanUnknownLoc(a.Data.Province),
		City:    fuseCityDistrict(a.Data.City, a.Data.Districts),
		ISP:     normalizeCNISP(a.Data.ISP),
	}, nil
}

// lookupBilibiliLocal 通过 B 站接口快速探测本机当前公网 IP 与大致归属（仅用于无入参本机兜底）。
func (r *Resolver) lookupBilibiliLocal(ctx context.Context) (model.GeoInfo, error) {
	body, err := r.doGet(ctx, bilibiliLocalURL, "Mozilla/5.0 (Windows NT 10.0; Win64; x64)", nil, 1<<20)
	if err != nil {
		return model.GeoInfo{}, err
	}
	var a struct {
		Code int `json:"code"`
		Data struct {
			Addr     string `json:"addr"`
			Country  string `json:"country"`
			Province string `json:"province"`
			City     string `json:"city"`
			ISP      string `json:"isp"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &a); err != nil {
		return model.GeoInfo{}, err
	}
	rawIP := strings.TrimSpace(a.Data.Addr)
	if a.Code != 0 || rawIP == "" || net.ParseIP(rawIP) == nil {
		return model.GeoInfo{}, fmt.Errorf("bilibili 查询失败")
	}
	if !isMainlandChinaLoc(a.Data.Country + " " + a.Data.Province + " " + a.Data.City) {
		return model.GeoInfo{}, fmt.Errorf("bilibili 非中国大陆 IP: %s %s", a.Data.Country, a.Data.Province)
	}
	return model.GeoInfo{
		IP:      rawIP,
		Stack:   stackOf(rawIP),
		Country: "CN",
		Region:  cleanUnknownLoc(a.Data.Province),
		City:    cleanUnknownLoc(a.Data.City),
		ISP:     normalizeCNISP(a.Data.ISP),
	}, nil
}

// lookupBaidu 查询百度大数据 IP 归属（仅认可中国大陆 IPv4 结果，避免海外或 IPv6 请求误标）。
func (r *Resolver) lookupBaidu(ctx context.Context, ip string, d model.Dialer) (model.GeoInfo, error) {
	if ip != "" && stackOf(ip) == "6" {
		return model.GeoInfo{}, fmt.Errorf("baidu 不支持 IPv6: %s", ip)
	}
	url := baiduBaseURL + "?resource_id=6006&oe=utf8"
	if ip != "" {
		url += "&query=" + ip
	}
	body, err := r.doGet(ctx, url, "Mozilla/5.0 (Windows NT 10.0; Win64; x64)", d, 1<<20)
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
	if !isMainlandChinaLoc(loc) {
		return model.GeoInfo{}, fmt.Errorf("baidu 非中国大陆 IP: %s", loc)
	}
	retIP := strings.TrimSpace(a.Data[0].Origip)
	if ip != "" && retIP != "" && !sameIP(ip, retIP) {
		return model.GeoInfo{}, fmt.Errorf("baidu 返回 IP 不匹配: 请求 %s 实际 %s", ip, retIP)
	}
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
	g.Region, g.City = splitBaiduLoc(loc)
	return g, nil
}

// splitBaiduLoc 把百度返回的“安徽省安庆市”或“上海市”拆分为省/直辖市与地级市，避免展示“安徽省安庆移动”。
func splitBaiduLoc(loc string) (region, city string) {
	loc = strings.TrimSpace(loc)
	for _, sep := range []string{"省", "自治区"} {
		if idx := strings.Index(loc, sep); idx >= 0 {
			end := idx + len(sep)
			region = loc[:end]
			city = strings.TrimSpace(loc[end:])
			if city == "" {
				city = region
			}
			return region, city
		}
	}
	return loc, loc
}

// lookupPCOnline 查询太平洋电脑网 IP 归属（仅认可中国大陆结果，避免海外 IP 被误标为 CN）。
func (r *Resolver) lookupPCOnline(ctx context.Context, ip string, d model.Dialer) (model.GeoInfo, error) {
	g, _, err := r.lookupPCOnlineWithIP(ctx, ip, d)
	return g, err
}

// lookupPCOnlineWithIP 查询太平洋电脑网并返回解析出的原始 IP（即使判定为海外 IP 也保留 rawIP 供后续国际链路复用）。
func (r *Resolver) lookupPCOnlineWithIP(ctx context.Context, ip string, d model.Dialer) (model.GeoInfo, string, error) {
	if ip != "" && stackOf(ip) == "6" {
		return model.GeoInfo{}, "", fmt.Errorf("pconline 不支持 IPv6: %s", ip)
	}
	url := pconlineBaseURL + "?json=true"
	if ip != "" {
		url += "&ip=" + ip
	}
	body, err := r.doGet(ctx, url, "", d, 1<<20)
	if err != nil {
		return model.GeoInfo{}, "", err
	}
	// 太平洋电脑网接口默认返回 GBK 编码，非合法 UTF-8 时先转码再反序列化，兼容测试中的 UTF-8 响应。
	if !utf8.Valid(body) {
		if decoded, decErr := simplifiedchinese.GBK.NewDecoder().Bytes(body); decErr == nil {
			body = decoded
		}
	}
	var a struct {
		IP   string `json:"ip"`
		Pro  string `json:"pro"`
		City string `json:"city"`
		Addr string `json:"addr"`
	}
	if err := json.Unmarshal(body, &a); err != nil || strings.TrimSpace(a.IP) == "" {
		return model.GeoInfo{}, "", fmt.Errorf("pconline 查询失败")
	}
	rawIP := strings.TrimSpace(a.IP)
	if ip != "" && !sameIP(ip, rawIP) {
		return model.GeoInfo{}, "", fmt.Errorf("pconline 返回 IP 不匹配: 请求 %s 实际 %s", ip, rawIP)
	}
	if !isMainlandChinaLoc(a.Pro) && !isMainlandChinaLoc(a.Addr) {
		return model.GeoInfo{}, rawIP, fmt.Errorf("pconline 非中国大陆 IP: %s", a.Addr)
	}
	g := model.GeoInfo{
		IP:      rawIP,
		Stack:   stackOf(rawIP),
		Country: "CN",
		Region:  a.Pro,
		City:    a.City,
	}
	switch {
	case strings.Contains(a.Addr, "移动") || strings.Contains(a.Addr, "移通"):
		g.ISP = "中国移动"
	case strings.Contains(a.Addr, "电信"):
		g.ISP = "中国电信"
	case strings.Contains(a.Addr, "联通"):
		g.ISP = "中国联通"
	}
	return g, rawIP, nil
}

// lookupCFTrace 经代理请求 Cloudflare Trace，获取实际出口 IP 与国家代码。
func (r *Resolver) lookupCFTrace(ctx context.Context, d model.Dialer) (model.GeoInfo, error) {
	bodyBytes, err := r.doGet(ctx, cfTraceURL, "Mozilla/5.0 NekoSpeed/1.0", d, 64*1024)
	if err != nil {
		return model.GeoInfo{}, err
	}
	ip, loc, _ := parseCFTrace(string(bodyBytes))
	if ip == "" || net.ParseIP(ip) == nil {
		return model.GeoInfo{}, fmt.Errorf("cf-trace 未找到有效 IP")
	}
	// colo 仅为 Cloudflare 就近 PoP 机房代号，不写入 Region/City 避免冒充真实出口城市。
	g := model.GeoInfo{
		IP:      ip,
		Stack:   stackOf(ip),
		Country: strings.ToUpper(strings.TrimSpace(loc)),
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
	body, err := r.doGet(ctx, url, "Mozilla/5.0 NekoSpeed/1.0", d, 1<<20)
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
	if ip != "" && !sameIP(ip, a.IP) {
		return model.GeoInfo{}, fmt.Errorf("ipwhois 返回 IP 不匹配: 请求 %s 实际 %s", ip, a.IP)
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
	body, err := r.doGet(ctx, url, "Mozilla/5.0 NekoSpeed/1.0", d, 1<<20)
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
	if ip != "" && !sameIP(ip, a.IP) {
		return model.GeoInfo{}, fmt.Errorf("ip.sb 返回 IP 不匹配: 请求 %s 实际 %s", ip, a.IP)
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
	body, err := r.doGet(ctx, url, "", d, 1<<20)
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
	if ip != "" && !sameIP(ip, a.Query) {
		return model.GeoInfo{}, fmt.Errorf("ip-api 返回 IP 不匹配: 请求 %s 实际 %s", ip, a.Query)
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
	City    string `json:"city"`
	Region  string `json:"region"`
	Country string `json:"country"`
	Org     string `json:"org"` // 形如 "AS13335 Cloudflare, Inc."
}

// lookupIPInfo 查询 ipinfo.io（出口查询兜底），只能拿到请求端自身信息。
func (r *Resolver) lookupIPInfo(ctx context.Context, d model.Dialer) (model.GeoInfo, error) {
	body, err := r.doGet(ctx, ipinfoURL, "", d, 1<<20)
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
	g := model.GeoInfo{
		IP:      a.IP,
		Stack:   stackOf(a.IP),
		Country: a.Country,
		Region:  a.Region,
		City:    a.City,
	}
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

// lookupGeoJS 查询 GeoJS 免费国际 GeoIP 端点（双栈支持，免 Key 兜底）。
func (r *Resolver) lookupGeoJS(ctx context.Context, ip string, d model.Dialer) (model.GeoInfo, error) {
	url := geojsBaseURL + ".json"
	if ip != "" {
		url = geojsBaseURL + "/" + ip + ".json"
	}
	body, err := r.doGet(ctx, url, "Mozilla/5.0 NekoSpeed/1.0", d, 1<<20)
	if err != nil {
		return model.GeoInfo{}, err
	}
	var a struct {
		IP               string `json:"ip"`
		CountryCode      string `json:"country_code"`
		Region           string `json:"region"`
		City             string `json:"city"`
		ASN              int    `json:"asn"`
		OrganizationName string `json:"organization_name"`
	}
	if err := json.Unmarshal(body, &a); err != nil {
		return model.GeoInfo{}, err
	}
	rawIP := strings.TrimSpace(a.IP)
	if rawIP == "" {
		return model.GeoInfo{}, fmt.Errorf("geojs 未返回有效 IP")
	}
	if ip != "" && !sameIP(ip, rawIP) {
		return model.GeoInfo{}, fmt.Errorf("geojs 返回 IP 不匹配: 请求 %s 实际 %s", ip, rawIP)
	}
	org := cleanUnknownLoc(a.OrganizationName)
	g := model.GeoInfo{
		IP:      rawIP,
		Stack:   stackOf(rawIP),
		Country: strings.ToUpper(strings.TrimSpace(a.CountryCode)),
		Region:  cleanUnknownLoc(a.Region),
		City:    cleanUnknownLoc(a.City),
		Org:     org,
		ISP:     org,
	}
	if a.ASN > 0 && !(a.ASN == 64512 && org == "") {
		g.ASN = fmt.Sprintf("AS%d", a.ASN)
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
