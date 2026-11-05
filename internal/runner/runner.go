// Package runner 封装一次完整测速流程的编排逻辑：
// 加载订阅 → 过滤 → 并发测试 → 聚合拓扑 → 渲染 PNG → 写出 JSON。
// CLI（cmd/nekospeed）与 Web UI（internal/webui）共用本包。
package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nekospeed/internal/geo"
	"nekospeed/internal/model"
	"nekospeed/internal/nodepool"
	"nekospeed/internal/render"
	"nekospeed/internal/report"
	"nekospeed/internal/subscription"
	"nekospeed/internal/tester"
	"nekospeed/internal/unlock"
)

// Version 输出图片与 JSON 中的版本标识。
const Version = "0.1.0"

// Options 一次测速任务的全部参数。
type Options struct {
	Source      string // 订阅链接或本地文件路径
	Tests       string // 测试项，逗号分隔：speed,media,ai,topo,nat 或 all
	Threads     int    // 测速线程数
	Duration    int    // 测速时长秒
	Concurrency int    // 同时测试的节点数
	PingCount   int    // 每节点延迟测试次数
	Single        bool   // 单线程测速（等价于 Threads=1）
	Filter        string // 节点名包含过滤正则（白名单）
	Exclude       string // 节点名排除过滤正则（黑名单）
	SkipInfoNodes bool   // 自动跳过剩余流量/到期时间/官网等提示类伪节点
	SortBy        string // 结果排序方式：""/"none"（订阅原序）、"speed"（速度降序）、"latency"（延迟升序）
	MaxMB         int    // 单节点下载测速最大流量上限（MB，<=0 表示不限制）
	OutDir        string // 输出目录
	MMDB        string // MaxMind GeoIP2 mmdb 路径（空则用在线 API）
	FontPath    string // 自定义中文字体路径
	// Name 手动指定订阅名称，用于输出文件名前缀（覆盖自动提取）。
	Name      string
	// Theme 手动指定渲染主题，为空时随机选择。
	Theme     string
	Watermark string // 图片水印文字
	SpeedURL    string // 自定义测速文件 URL（默认 Google / Cloudflare 大文件）
	PingURL     string // 延迟测试地址
	// Preflight 测试前探测服务器域名全部 IP 的可达性并锁定活 IP（默认开）。
	Preflight bool
	// Bandwidth 手动指定的本地带宽标签（如 "1Gbps"），为空时取测速峰值。
	Bandwidth string
	// Proxy 拉取订阅使用的代理（如 http://127.0.0.1:7890），为空时读取 HTTPS_PROXY 等环境变量。
	Proxy string
	// SelectedIndices 手动勾选要测试的节点原始序号（1-based），为空表示测试全部过滤后节点。
	SelectedIndices []int
	// OnNodeDone 每完成一个节点测试时的实时回调（可选，供 Web UI 流式推送）。
	OnNodeDone func(done, total int, r *model.NodeResult)
}

// Output 一次测速任务的结果摘要。
type Output struct {
	Files     []string      // 生成的文件（含路径）
	Total     int           // 节点总数
	Succeeded int           // 成功节点数
	Elapsed   time.Duration // 测试耗时
}

// Run 执行一次完整测速流程，日志通过 logf 输出（logf 可为 nil）。
func Run(ctx context.Context, o Options, logf func(format string, args ...any)) (*Output, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	if o.Source == "" {
		return nil, fmt.Errorf("必须提供订阅 URL 或本地文件路径")
	}
	if o.Single {
		o.Threads = 1
	} else if o.Threads == 1 {
		o.Single = true
	}
	if err := subscription.SetProxy(o.Proxy); err != nil {
		return nil, err
	}

	netEnv := nodepool.RefreshNetworkEnv()
	if netEnv.PhysicalInterface != "" {
		if netEnv.TUNDetected {
			logf("网络环境：检测到本地 TUN (%s)，已自动绑定物理网卡 %s (%s) 直连，防止代理套代理",
				strings.Join(netEnv.TUNInterfaces, ","), netEnv.PhysicalInterface, netEnv.PhysicalIP)
		} else {
			logf("网络环境：已绑定物理网卡 %s (%s)", netEnv.PhysicalInterface, netEnv.PhysicalIP)
		}
	}

	testSet := parseTests(o.Tests)

	// 1. 加载订阅
	nodes, subName, err := subscription.Load(o.Source)
	if err != nil {
		return nil, fmt.Errorf("加载订阅失败: %w", err)
	}
	if o.Name != "" {
		subName = o.Name
	}
	if len(o.SelectedIndices) > 0 {
		sel := make(map[int]struct{}, len(o.SelectedIndices))
		for _, idx := range o.SelectedIndices {
			sel[idx] = struct{}{}
		}
		var kept []model.Node
		for _, n := range nodes {
			if _, ok := sel[n.Index]; ok {
				kept = append(kept, n)
			}
		}
		nodes = kept
	} else {
		if o.SkipInfoNodes {
			var kept []model.Node
			var skipped []string
			for _, n := range nodes {
				if isInfoNodeName(n.Name) {
					skipped = append(skipped, n.Name)
					continue
				}
				kept = append(kept, n)
			}
			if len(skipped) > 0 {
				logf("已自动跳过 %d 个提示类伪节点：%s", len(skipped), strings.Join(skipped, " / "))
			}
			nodes = kept
		}
		if o.Filter != "" {
			re, err := regexp.Compile(o.Filter)
			if err != nil {
				return nil, fmt.Errorf("编译过滤正则失败: %w", err)
			}
			var kept []model.Node
			for _, n := range nodes {
				if re.MatchString(n.Name) {
					kept = append(kept, n)
				}
			}
			nodes = kept
		}
		if o.Exclude != "" {
			re, err := regexp.Compile(o.Exclude)
			if err != nil {
				return nil, fmt.Errorf("编译排除正则失败: %w", err)
			}
			var kept []model.Node
			for _, n := range nodes {
				if !re.MatchString(n.Name) {
					kept = append(kept, n)
				}
			}
			nodes = kept
		}
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("没有可用节点")
	}
	logf("共 %d 个节点，测试项：%s", len(nodes), strings.Join(testSet, ","))

	// 1.5 服务器 IP 预检：DDNS 轮换 IP 池中可能混有被封锁的死 IP，
	// 探测出当前可达 IP 钉入 mihomo DefaultHosts，避免拨号随机选中死 IP。
	if o.Preflight {
		if s := nodepool.FormatPreflightLog(nodepool.PreflightServers(ctx, nodes)); s != "" {
			logf("%s", s)
		}
	}

	// 2. GeoIP 解析器（topo 需要，speed 图本地运营商标签也需要）
	resolver, err := geo.NewResolver(o.MMDB)
	if err != nil {
		return nil, fmt.Errorf("初始化 GeoIP 失败: %w", err)
	}
	defer resolver.Close()

	// 3. 并发测试：非带宽探测项按 Concurrency 并发执行，下载测速由独立 speedSem（容量 1）串行化，兼顾速度与准确性。
	concurrency := o.Concurrency
	if concurrency <= 0 {
		concurrency = 6
	}
	results := make([]*model.NodeResult, len(nodes))
	sem := make(chan struct{}, concurrency)
	speedSem := make(chan struct{}, 1)
	ucache := newUnlockCache()
	maxBytes := int64(o.MaxMB) * 1024 * 1024
	var wg sync.WaitGroup
	var doneCount int32
	start := time.Now()
	for i, node := range nodes {
		results[i] = &model.NodeResult{Node: node}
		wg.Add(1)
		go func(i int, node model.Node) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i].Err = "任务已取消"
				return
			}
			defer func() { <-sem }()
			testNode(ctx, node, results[i], testSet, testOpts{
				threads:     o.Threads,
				duration:    o.Duration,
				maxBytes:    maxBytes,
				speedURL:    o.SpeedURL,
				pingURL:     o.PingURL,
				pingCount:   o.PingCount,
				resolver:    resolver,
				speedSem:    speedSem,
				unlockCache: ucache,
			})
			done := int(atomic.AddInt32(&doneCount, 1))
			logf("[%d/%d] %s 完成", done, len(nodes), node.Name)
			if o.OnNodeDone != nil {
				o.OnNodeDone(done, len(nodes), results[i])
			}
		}(i, node)
	}
	wg.Wait()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("测试已取消")
	}
	elapsed := time.Since(start).Round(time.Second)

	sortLabel := sortResults(results, o.SortBy)

	// 4. 线路分析聚合
	var topo *model.Topology
	if hasTest(testSet, "topo") {
		topo = geo.BuildTopology(results)
	}

	// 5. 输出
	if err := os.MkdirAll(o.OutDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建输出目录失败: %w", err)
	}
	local := resolver.LookupLocalNetwork(ctx)
	stamp := time.Now().Format("20060102_150405")
	prefix := stamp
	if subName != "" {
		prefix = subName + "_" + stamp
	}
	meta := render.Meta{
		Version:      Version,
		Threads:      o.Threads,
		Total:        len(results),
		Succeeded:    countOK(results),
		StartTime:    start,
		Watermark:    o.Watermark,
		FontPath:     o.FontPath,
		Single:       o.Single,
		LocalISP:     formatLocalISP(local),
		Bandwidth:    maxBandwidth(results, o.Bandwidth),
		SortLabel:    sortLabel,
		Theme:        render.DefaultTheme(),
		SubName:      subName,
		Elapsed:      elapsed,
		PingCount:    o.PingCount,
		Filter:       formatFilterLabel(o.Filter, o.Exclude),
		TrafficBytes: totalTrafficBytes(results),
	}
	if o.Theme != "" {
		meta.Theme = render.ThemeByName(o.Theme)
	}

	out := &Output{
		Total:     len(results),
		Succeeded: countOK(results),
		Elapsed:   elapsed,
	}
	save := func(fn func(string) error, name, what string) error {
		path := filepath.Join(o.OutDir, name)
		if err := fn(path); err != nil {
			return fmt.Errorf("%s失败: %w", what, err)
		}
		out.Files = append(out.Files, path)
		return nil
	}

	if hasTest(testSet, "speed") {
		if err := save(func(p string) error { return render.SaveSpeedImage(results, meta, p) }, prefix+"_speed.png", "渲染测速图"); err != nil {
			return nil, err
		}
	}
	if hasTest(testSet, "media") {
		if err := save(func(p string) error { return render.SaveMediaImage(results, meta, p) }, prefix+"_media.png", "渲染流媒体图"); err != nil {
			return nil, err
		}
	}
	if hasTest(testSet, "ai") {
		if err := save(func(p string) error { return render.SaveAIImage(results, meta, p) }, prefix+"_ai.png", "渲染 AI 图"); err != nil {
			return nil, err
		}
	}
	if hasTest(testSet, "speed") && (hasTest(testSet, "media") || hasTest(testSet, "ai")) {
		if err := save(func(p string) error { return render.SaveFullImage(results, meta, p) }, prefix+"_full.png", "渲染综合图"); err != nil {
			return nil, err
		}
	}
	if topo != nil {
		if err := save(func(p string) error { return render.SaveTopologyImage(results, topo, meta, p) }, prefix+"_topology.png", "渲染线路分析图"); err != nil {
			return nil, err
		}
	}
	if err := save(func(p string) error { return report.SaveJSON(results, topo, meta.Version, start, elapsed, p) }, prefix+"_result.json", "写出 JSON"); err != nil {
		return nil, err
	}

	logf("完成：成功 %d/%d，耗时 %s，输出目录 %s", out.Succeeded, out.Total, elapsed, o.OutDir)
	return out, nil
}

type testOpts struct {
	threads     int
	duration    int
	maxBytes    int64
	speedURL    string
	pingURL     string
	pingCount   int
	resolver    *geo.Resolver
	speedSem    chan struct{}
	unlockCache *unlockCache
}

// testNode 对单个节点执行选中的测试项，结果写入 r。
func testNode(ctx context.Context, node model.Node, r *model.NodeResult, testSet []string, o testOpts) {
	if ctx.Err() != nil {
		r.Err = "任务已取消"
		return
	}
	dialer, err := nodepool.NewDialer(node)
	if err != nil {
		r.Err = err.Error()
		return
	}

	needLatency := hasTest(testSet, "speed") || hasTest(testSet, "media")
	if needLatency {
		r.Latency = tester.Latency(ctx, dialer, o.pingURL, o.pingCount)
		// 延迟测试 100% 丢包说明代理通道不可用，直接熔断后续测速与解锁探测，仅保留入口 IP 解析。
		if r.Latency.PacketLoss >= 1.0 && r.Latency.AvgHTTPS == 0 {
			if o.resolver != nil {
				in, _ := o.resolver.ResolveNode(ctx, node, nil)
				r.Inbound = in
			}
			r.Err = "节点连接超时（丢包率 100%）"
			return
		}
	}

	// 先解析入口/出口 GeoIP：拿到落地出口 IP 后可为同出口节点的 Media/AI 解锁做去重复用。
	if o.resolver != nil && ctx.Err() == nil {
		in, out := o.resolver.ResolveNode(ctx, node, dialer)
		r.Inbound, r.Outbound = in, out
		// 若未测延迟且出口 IP 获取完全失败，说明节点不可连通，熔断后续重型检测。
		if !needLatency && out == nil {
			r.Err = "节点无法获取出口 IP"
			return
		}
	}

	outIP := ""
	if r.Outbound != nil {
		outIP = r.Outbound.IP
	}

	if hasTest(testSet, "nat") && ctx.Err() == nil {
		r.NAT = tester.NAT(ctx, dialer)
	}
	if hasTest(testSet, "media") && ctx.Err() == nil {
		if o.unlockCache != nil && outIP != "" {
			r.Media = o.unlockCache.getMedia(ctx, outIP, dialer)
		} else {
			r.Media = unlock.RunMedia(ctx, dialer)
		}
	}
	if hasTest(testSet, "ai") && ctx.Err() == nil {
		if o.unlockCache != nil && outIP != "" {
			r.AI = o.unlockCache.getAI(ctx, outIP, dialer)
		} else {
			r.AI = unlock.RunAI(ctx, dialer)
		}
	}
	if hasTest(testSet, "speed") && ctx.Err() == nil {
		if o.speedSem != nil {
			select {
			case o.speedSem <- struct{}{}:
				defer func() { <-o.speedSem }()
			case <-ctx.Done():
				return
			}
		}
		r.Speed = tester.SpeedWithLimit(ctx, dialer, o.speedURL, o.threads, o.duration, o.maxBytes)
	}
}

var infoNodeRe = regexp.MustCompile(`(?i)(剩余流量|套餐到期|到期时间|有效期限|官网|新网址|备用网址|最新网址|群组|频道|公告|过滤掉|流量重置|重置剩余|距离下次重置|请更新订阅)`)

// isInfoNodeName 判断节点名是否为机场常见的非代理提示类伪节点。
func isInfoNodeName(name string) bool {
	return infoNodeRe.MatchString(name)
}

// IsInfoNodeName 公开的提示类伪节点判断函数（供 Web UI 预览列表标记使用）。
func IsInfoNodeName(name string) bool {
	return infoNodeRe.MatchString(name)
}

// ReRenderOptions 定义免重测重绘图片的可选覆盖参数。
type ReRenderOptions struct {
	SortBy    string `json:"sortBy"`
	Theme     string `json:"theme"`
	Watermark string `json:"watermark"`
	Bandwidth string `json:"bandwidth"`
}

// ReRender 读取 outDir 下已有的 *_result.json，按新的排序/主题/水印/带宽参数秒级重绘 PNG 图片。
func ReRender(outDir string, o ReRenderOptions) ([]string, error) {
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败: %w", err)
	}
	var jsonFile string
	existingPNGs := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fn := e.Name()
		if strings.HasSuffix(fn, "_result.json") && jsonFile == "" {
			jsonFile = fn
		} else if strings.HasSuffix(fn, ".png") {
			existingPNGs[fn] = true
		}
	}
	if jsonFile == "" {
		return nil, fmt.Errorf("未找到原始测速数据 (*_result.json)")
	}
	prefix := strings.TrimSuffix(jsonFile, "_result.json")
	subName := prefix
	if len(prefix) > 16 && prefix[len(prefix)-16] == '_' && prefix[len(prefix)-7] == '_' {
		subName = prefix[:len(prefix)-16]
	}
	raw, err := os.ReadFile(filepath.Join(outDir, jsonFile))
	if err != nil {
		return nil, err
	}

	var doc struct {
		Version   string    `json:"version"`
		StartTime time.Time `json:"startTime"`
		Elapsed   string    `json:"elapsed"`
		Total     int       `json:"total"`
		Succeeded int       `json:"succeeded"`
		Nodes     []struct {
			Index    int                  `json:"index"`
			Name     string               `json:"name"`
			Type     string               `json:"type"`
			Server   string               `json:"server"`
			Latency  *model.LatencyResult `json:"latency"`
			Speed    *model.SpeedResult   `json:"speed"`
			NAT      string               `json:"nat"`
			Media    []model.UnlockResult `json:"media"`
			AI       []model.UnlockResult `json:"ai"`
			Inbound  *model.GeoInfo       `json:"inbound"`
			Outbound *model.GeoInfo       `json:"outbound"`
			Error    string               `json:"error"`
		} `json:"nodes"`
		Topology *model.Topology `json:"topology"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %w", err)
	}

	var results []*model.NodeResult
	threads := 1
	hasSpeed, hasMedia, hasAI := false, false, false
	for _, n := range doc.Nodes {
		if n.Speed != nil {
			hasSpeed = true
			if n.Speed.Threads > 0 {
				threads = n.Speed.Threads
			}
		}
		if len(n.Media) > 0 {
			hasMedia = true
		}
		if len(n.AI) > 0 {
			hasAI = true
		}
		results = append(results, &model.NodeResult{
			Node: model.Node{
				Index:  n.Index,
				Name:   n.Name,
				Type:   n.Type,
				Server: n.Server,
			},
			Latency:  n.Latency,
			Speed:    n.Speed,
			NAT:      n.NAT,
			Media:    n.Media,
			AI:       n.AI,
			Inbound:  n.Inbound,
			Outbound: n.Outbound,
			Err:      n.Error,
		})
	}

	sortLabel := sortResults(results, o.SortBy)
	dur, _ := time.ParseDuration(doc.Elapsed)
	ver := doc.Version
	if ver == "" {
		ver = Version
	}
	theme := render.DefaultTheme()
	if o.Theme != "" {
		theme = render.ThemeByName(o.Theme)
	}
	meta := render.Meta{
		Version:      ver,
		Threads:      threads,
		Total:        len(results),
		Succeeded:    countOK(results),
		StartTime:    doc.StartTime,
		Watermark:    strings.TrimSpace(o.Watermark),
		Single:       threads == 1,
		Bandwidth:    strings.TrimSpace(o.Bandwidth),
		SortLabel:    sortLabel,
		Theme:        theme,
		SubName:      subName,
		Elapsed:      dur,
		PingCount:    4,
		Filter:       "None",
		TrafficBytes: totalTrafficBytes(results),
	}

	var files []string
	if hasSpeed || existingPNGs[prefix+"_speed.png"] {
		name := prefix + "_speed.png"
		if err := render.SaveSpeedImage(results, meta, filepath.Join(outDir, name)); err != nil {
			return nil, err
		}
		files = append(files, name)
	}
	if hasMedia || existingPNGs[prefix+"_media.png"] {
		name := prefix + "_media.png"
		if err := render.SaveMediaImage(results, meta, filepath.Join(outDir, name)); err != nil {
			return nil, err
		}
		files = append(files, name)
	}
	if hasAI || existingPNGs[prefix+"_ai.png"] {
		name := prefix + "_ai.png"
		if err := render.SaveAIImage(results, meta, filepath.Join(outDir, name)); err != nil {
			return nil, err
		}
		files = append(files, name)
	}
	if (hasSpeed && (hasMedia || hasAI)) || existingPNGs[prefix+"_full.png"] {
		name := prefix + "_full.png"
		if err := render.SaveFullImage(results, meta, filepath.Join(outDir, name)); err != nil {
			return nil, err
		}
		files = append(files, name)
	}
	if doc.Topology != nil || existingPNGs[prefix+"_topology.png"] {
		topo := doc.Topology
		if topo == nil {
			topo = geo.BuildTopology(results)
		}
		name := prefix + "_topology.png"
		if err := render.SaveTopologyImage(results, topo, meta, filepath.Join(outDir, name)); err != nil {
			return nil, err
		}
		files = append(files, name)
	}
	files = append(files, jsonFile)
	return files, nil
}

// sortResults 按 sortBy 对节点结果排序，返回用于图片页脚展示的排序名称。
func sortResults(results []*model.NodeResult, sortBy string) string {
	switch strings.ToLower(strings.TrimSpace(sortBy)) {
	case "speed":
		sort.SliceStable(results, func(i, j int) bool {
			var si, sj float64
			if results[i].Speed != nil {
				si = results[i].Speed.Avg
			}
			if results[j].Speed != nil {
				sj = results[j].Speed.Avg
			}
			if si != sj {
				return si > sj
			}
			return results[i].OK() && !results[j].OK()
		})
		return "速度降序"
	case "latency", "ping":
		sort.SliceStable(results, func(i, j int) bool {
			var li, lj int
			if results[i].Latency != nil {
				li = results[i].Latency.AvgHTTPS
			}
			if results[j].Latency != nil {
				lj = results[j].Latency.AvgHTTPS
			}
			if li > 0 && lj > 0 {
				return li < lj
			}
			if li > 0 || lj > 0 {
				return li > 0
			}
			return results[i].OK() && !results[j].OK()
		})
		return "延迟升序"
	default:
		return "订阅原序"
	}
}

type unlockCall struct {
	done chan struct{}
	res  []model.UnlockResult
	ok   bool
}

// unlockCache 按出口落地 IP 缓存流媒体与 AI 解锁结果（同场测速内去重）。
type unlockCache struct {
	mu            sync.Mutex
	mediaCache    map[string][]model.UnlockResult
	mediaInflight map[string]*unlockCall
	aiCache       map[string][]model.UnlockResult
	aiInflight    map[string]*unlockCall
}

func newUnlockCache() *unlockCache {
	return &unlockCache{
		mediaCache:    make(map[string][]model.UnlockResult),
		mediaInflight: make(map[string]*unlockCall),
		aiCache:       make(map[string][]model.UnlockResult),
		aiInflight:    make(map[string]*unlockCall),
	}
}

func (u *unlockCache) getMedia(ctx context.Context, outIP string, d model.Dialer) []model.UnlockResult {
	return u.getOrRun(ctx, outIP, &u.mediaCache, &u.mediaInflight, func() []model.UnlockResult {
		return unlock.RunMedia(ctx, d)
	})
}

func (u *unlockCache) getAI(ctx context.Context, outIP string, d model.Dialer) []model.UnlockResult {
	return u.getOrRun(ctx, outIP, &u.aiCache, &u.aiInflight, func() []model.UnlockResult {
		return unlock.RunAI(ctx, d)
	})
}

func (u *unlockCache) getOrRun(
	ctx context.Context,
	outIP string,
	cache *map[string][]model.UnlockResult,
	inflight *map[string]*unlockCall,
	fn func() []model.UnlockResult,
) []model.UnlockResult {
	u.mu.Lock()
	if res, ok := (*cache)[outIP]; ok {
		u.mu.Unlock()
		return cloneUnlock(res)
	}
	if call, ok := (*inflight)[outIP]; ok {
		u.mu.Unlock()
		select {
		case <-call.done:
			if call.ok {
				return cloneUnlock(call.res)
			}
			return fn()
		case <-ctx.Done():
			return nil
		}
	}
	call := &unlockCall{done: make(chan struct{})}
	(*inflight)[outIP] = call
	u.mu.Unlock()

	res := fn()
	// 仅当结果中不存在因单节点瞬时抖动导致的请求失败（reqfailed）时才写入共享缓存。
	cacheable := len(res) > 0
	for _, item := range res {
		if item.Status == model.StatusReqFailed {
			cacheable = false
			break
		}
	}

	u.mu.Lock()
	if cacheable {
		(*cache)[outIP] = res
	}
	delete(*inflight, outIP)
	call.res = res
	call.ok = cacheable
	u.mu.Unlock()
	close(call.done)
	return cloneUnlock(res)
}

func cloneUnlock(in []model.UnlockResult) []model.UnlockResult {
	if in == nil {
		return nil
	}
	out := make([]model.UnlockResult, len(in))
	copy(out, in)
	return out
}

func parseTests(s string) []string {
	if s == "all" {
		return []string{"speed", "media", "ai", "topo", "nat"}
	}
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func hasTest(set []string, name string) bool {
	for _, t := range set {
		if t == name {
			return true
		}
	}
	return false
}

func countOK(rs []*model.NodeResult) int {
	n := 0
	for _, r := range rs {
		if r.OK() {
			n++
		}
	}
	return n
}

func totalTrafficBytes(rs []*model.NodeResult) int64 {
	var total int64
	for _, r := range rs {
		if r == nil || r.Speed == nil {
			continue
		}
		for _, v := range r.Speed.PerSecond {
			total += int64(v)
		}
	}
	return total
}

func formatFilterLabel(filter, exclude string) string {
	var parts []string
	if filter != "" {
		parts = append(parts, "包含:"+filter)
	}
	if exclude != "" {
		parts = append(parts, "排除:"+exclude)
	}
	if len(parts) == 0 {
		return "None"
	}
	return strings.Join(parts, " & ")
}

// formatLocalISP 把 GeoInfo 整理为 "城市运营商" 的展示字符串。
// 优先使用城市 + 地区，ISP 名称会简化为中文简称；信息不足时返回空。
func formatLocalISP(g model.GeoInfo) string {
	loc := g.City
	if loc == "" {
		loc = g.Region
	}
	if loc == "" {
		return ""
	}
	isp := g.ISP
	if isp == "" {
		isp = g.Org
	}
	isp = localizeISP(isp)
	if isp == "" {
		return loc
	}
	return loc + isp
}

// localizeISP 把常见中英运营商名统一简化为中文简称。
func localizeISP(isp string) string {
	if strings.Contains(isp, "电信") {
		return "电信"
	}
	if strings.Contains(isp, "联通") {
		return "联通"
	}
	if strings.Contains(isp, "移动") {
		return "移动"
	}
	if strings.Contains(isp, "广电") {
		return "广电"
	}
	lower := strings.ToLower(isp)
	switch {
	case strings.Contains(lower, "telecom") || strings.Contains(lower, "chinanet"):
		return "电信"
	case strings.Contains(lower, "unicom"):
		return "联通"
	case strings.Contains(lower, "mobile") || strings.Contains(lower, "cmcc"):
		return "移动"
	case strings.Contains(lower, "crtc") || strings.Contains(lower, "broadcasting"):
		return "广电"
	}
	return isp
}

// maxBandwidth 取所有节点测速峰值的最大值，格式化为 "1Gbps" / "500Mbps"。
// 手动指定 override 时优先使用 override；没有有效速度时返回空字符串。
func maxBandwidth(results []*model.NodeResult, override string) string {
	if override != "" {
		return override
	}
	var maxBPS float64
	for _, r := range results {
		if r != nil && r.Speed != nil && r.Speed.Max > maxBPS {
			maxBPS = r.Speed.Max
		}
	}
	if maxBPS <= 0 {
		return ""
	}
	mbps := maxBPS * 8 / 1e6
	if mbps >= 1000 {
		gbps := mbps / 1000
		if gbps == float64(int(gbps)) {
			return fmt.Sprintf("%dGbps", int(gbps))
		}
		return fmt.Sprintf("%.1fGbps", gbps)
	}
	if mbps < 1 {
		return "<1Mbps"
	}
	return fmt.Sprintf("%.0fMbps", mbps)
}

