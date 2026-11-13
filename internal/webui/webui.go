// Package webui 提供内嵌 Web UI：浏览器里填订阅 → 节点预览/挑选 → 启动测速 →
// SSE 实时日志与单节点流式结果 → 免重测重绘与查看结果图。仅使用标准库（net/http + embed）。
package webui

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"nekospeed/internal/model"
	"nekospeed/internal/nodepool"
	"nekospeed/internal/render"
	"nekospeed/internal/runner"
	"nekospeed/internal/subscription"
)

const (
	maxUploadJSONBytes = 16 << 20
	maxUploadFiles     = 30
	maxUploadAge       = 24 * time.Hour
	maxHubHistory      = 4000
	tokenCookieName    = "nekospeed_token"
)

var (
	stampSuffixRe      = regexp.MustCompile(`_\d{8}_\d{6}$`)
	inlineNamePrefixRe = regexp.MustCompile(`^([0-9a-f]{16}_|\d{8}_\d{6})`)
)

//go:embed static/*
var staticFS embed.FS

// Serve 在 addr 上启动 Web UI HTTP 服务并阻塞，直到服务出错。
// 若环境变量 NEKOSPEED_WEB_TOKEN 非空，则自动启用访问令牌保护。
func Serve(addr string) error {
	return ServeWithToken(addr, os.Getenv("NEKOSPEED_WEB_TOKEN"))
}

// ServeWithToken 在 addr 上启动带可选访问令牌保护的 Web UI 服务。
// 启动后自动尝试打开浏览器，失败仅打印 URL。
func ServeWithToken(addr, token string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("监听 %s 失败: %w", addr, err)
	}

	// 启动时预探测一次物理网卡与本地 TUN 环境，供前端顶栏展示。
	nodepool.RefreshNetworkEnv()

	token = strings.TrimSpace(token)
	s := &server{hub: newHub(), token: token}

	bindHost, port, splitErr := net.SplitHostPort(ln.Addr().String())
	if splitErr == nil && token == "" && !isLoopbackBindHost(bindHost) {
		fmt.Fprintf(os.Stderr, "安全警告：Web UI 正在非回环地址 %s 监听且未设置访问令牌，建议通过 -web-token 或 NEKOSPEED_WEB_TOKEN 开启鉴权\n", ln.Addr().String())
	}
	displayHost := bindHost
	if splitErr != nil || displayHost == "" || displayHost == "0.0.0.0" || displayHost == "::" {
		if port == "" {
			port = strings.TrimPrefix(addr, ":")
		}
		displayHost = "127.0.0.1"
	}
	url := "http://" + net.JoinHostPort(displayHost, port)
	if token != "" {
		fmt.Printf("Web UI 已启动：%s （已启用令牌鉴权，支持 Header 或 ?token=...）\n", url)
	} else {
		fmt.Printf("Web UI 已启动：%s\n", url)
	}
	openBrowser(url)

	return http.Serve(ln, s.handler())
}

func isLoopbackBindHost(host string) bool {
	if host == "" || strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /static/", s.handleStatic)
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("POST /api/preview", s.handlePreview)
	mux.HandleFunc("POST /api/run", s.handleRun)
	mux.HandleFunc("POST /api/rerender", s.handleReRender)
	mux.HandleFunc("POST /api/cancel", s.handleCancel)
	mux.HandleFunc("GET /api/history", s.handleHistory)
	mux.HandleFunc("DELETE /api/history/{run}", s.handleDeleteHistory)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.HandleFunc("GET /files/{name}", s.handleFiles)
	mux.HandleFunc("GET /files/{run}/{name}", s.handleHistoryFiles)
	return s.withAuth(mux)
}

func (s *server) withAuth(next http.Handler) http.Handler {
	if s.token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			if q := strings.TrimSpace(r.URL.Query().Get("token")); subtle.ConstantTimeCompare([]byte(q), []byte(s.token)) == 1 {
				http.SetCookie(w, &http.Cookie{
					Name:     tokenCookieName,
					Value:    s.token,
					Path:     "/",
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})
			}
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/files/") {
			if !s.authorized(r) {
				http.Error(w, "未授权：缺少或无效的访问令牌", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) authorized(r *http.Request) bool {
	if s.token == "" {
		return true
	}
	match := func(cand string) bool {
		return subtle.ConstantTimeCompare([]byte(cand), []byte(s.token)) == 1
	}
	if auth := strings.TrimSpace(r.Header.Get("Authorization")); len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
		if match(strings.TrimSpace(auth[7:])) {
			return true
		}
	}
	if hdr := strings.TrimSpace(r.Header.Get("X-NekoSpeed-Token")); hdr != "" && match(hdr) {
		return true
	}
	if q := strings.TrimSpace(r.URL.Query().Get("token")); q != "" && match(q) {
		return true
	}
	if c, err := r.Cookie(tokenCookieName); err == nil && match(c.Value) {
		return true
	}
	return false
}

type server struct {
	hub   *hub
	token string

	mu        sync.Mutex
	running   bool
	cancel    context.CancelFunc
	runID     string
	startedAt time.Time
	outDir    string // 最近一次任务的输出目录
}

// runRequest 是 POST /api/run 的请求体（runner.Options 的子集 + 上传内容支持）。
type runRequest struct {
	Source          string `json:"source"`
	Content         string `json:"content,omitempty"`
	Filename        string `json:"filename,omitempty"`
	Name            string `json:"name"`
	Tests           string `json:"tests"`
	Threads         int    `json:"threads"`
	Duration        int    `json:"duration"`
	Concurrency     int    `json:"concurrency"`
	PingCount       int    `json:"pingCount"`
	Single          bool   `json:"single"`
	Filter          string `json:"filter"`
	Exclude         string `json:"exclude"`
	SkipInfo        *bool  `json:"skipInfo,omitempty"`
	Preflight       *bool  `json:"preflight,omitempty"`
	SortBy          string `json:"sortBy"`
	MaxMB           int    `json:"maxMB"`
	Theme           string `json:"theme"`
	Watermark       string `json:"watermark"`
	Bandwidth       string `json:"bandwidth"`
	SpeedURL        string `json:"speedURL"`
	PingURL         string `json:"pingURL"`
	Proxy           string `json:"proxy"`
	SelectedIndices []int  `json:"selectedIndices,omitempty"`
}

// previewRequest 是 POST /api/preview 的请求体。
type previewRequest struct {
	Source   string `json:"source"`
	Content  string `json:"content,omitempty"`
	Filename string `json:"filename,omitempty"`
	Filter   string `json:"filter"`
	Exclude  string `json:"exclude"`
	SkipInfo *bool  `json:"skipInfo,omitempty"`
	Proxy    string `json:"proxy"`
}

// previewNode 是节点预览列表中的单条记录。
type previewNode struct {
	Index     int    `json:"index"`
	Name      string `json:"name"`
	CleanName string `json:"cleanName"`
	Flag      string `json:"flag"`
	Type      string `json:"type"`
	Server    string `json:"server"`
	IsInfo    bool   `json:"isInfo"`
	Matched   bool   `json:"matched"`
}

// rerenderRequest 是 POST /api/rerender 的请求体。
type rerenderRequest struct {
	RunID     string `json:"runId"`
	SortBy    string `json:"sortBy"`
	Theme     string `json:"theme"`
	Watermark string `json:"watermark"`
	Bandwidth string `json:"bandwidth"`
}

// nodeRecord 与 report.SaveJSON 写出的单节点 JSON 结构保持一致。
type nodeRecord struct {
	Index    int                  `json:"index"`
	Name     string               `json:"name"`
	Type     string               `json:"type"`
	Server   string               `json:"server"`
	Latency  *model.LatencyResult `json:"latency,omitempty"`
	Speed    *model.SpeedResult   `json:"speed,omitempty"`
	NAT      string               `json:"nat,omitempty"`
	Media    []model.UnlockResult `json:"media,omitempty"`
	AI       []model.UnlockResult `json:"ai,omitempty"`
	Inbound  *model.GeoInfo       `json:"inbound,omitempty"`
	Outbound *model.GeoInfo       `json:"outbound,omitempty"`
	Error    string               `json:"error,omitempty"`
}

// nodeEvent 是 SSE node 事件的数据（单节点完成时实时推送）。
type nodeEvent struct {
	Done  int        `json:"done"`
	Total int        `json:"total"`
	Node  nodeRecord `json:"node"`
}

// doneEvent 是 SSE done 事件的数据。
type doneEvent struct {
	OK        bool     `json:"ok"`
	Error     string   `json:"error,omitempty"`
	RunID     string   `json:"runId,omitempty"`
	Total     int      `json:"total"`
	Succeeded int      `json:"succeeded"`
	Elapsed   string   `json:"elapsed"`
	Files     []string `json:"files"` // 文件名（相对 /files/）
}

type historyItem struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	SubName   string   `json:"subName,omitempty"`
	Total     int      `json:"total,omitempty"`
	Succeeded int      `json:"succeeded,omitempty"`
	Elapsed   string   `json:"elapsed,omitempty"`
	Files     []string `json:"files"`
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "前端资源加载失败", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}

func (s *server) handleStatic(w http.ResponseWriter, r *http.Request) {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		http.Error(w, "静态资源不可用", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.StripPrefix("/static/", http.FileServerFS(sub)).ServeHTTP(w, r)
}

func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	running := s.running
	runID := s.runID
	outDir := s.outDir
	var startedAt int64
	if !s.startedAt.IsZero() {
		startedAt = s.startedAt.UnixMilli()
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]any{
		"running":   running,
		"runId":     runID,
		"outDir":    outDir,
		"startedAt": startedAt,
		"themes":    render.ThemeNames(),
		"netEnv":    nodepool.GetNetworkEnv(),
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadJSONBytes)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "请求体过大（上限 16 MiB）", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "请求体不是合法 JSON", http.StatusBadRequest)
		}
		return false
	}
	return true
}

// materializeInlineSource 将前端上传或粘贴的订阅内容按内容哈希去重落盘到 out/uploads/，
// 同时清理超过保留期或数量上限的历史临时上传，返回本地文件路径与推断名称。
func materializeInlineSource(source, content, filename string) (string, string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return strings.TrimSpace(source), "", nil
	}
	uploadDir := filepath.Join("out", "uploads")
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return "", "", fmt.Errorf("创建上传目录失败: %w", err)
	}
	base := strings.TrimSpace(filename)
	if base == "" {
		base = "pasted_sub.yaml"
	}
	base = filepath.Base(base)
	inferred := strings.TrimSuffix(base, filepath.Ext(base))
	sum := sha256.Sum256([]byte(content))
	hashPrefix := hex.EncodeToString(sum[:8])
	safeName := fmt.Sprintf("%s_%s", hashPrefix, sanitizeName(base))
	dst := filepath.Join(uploadDir, safeName)
	now := time.Now()
	if _, err := os.Stat(dst); err == nil {
		_ = os.Chtimes(dst, now, now)
	} else {
		if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
			return "", "", fmt.Errorf("保存上传内容失败: %w", err)
		}
	}
	pruneUploads(uploadDir, dst, now)
	return dst, inferred, nil
}

func pruneUploads(dir, keepPath string, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type item struct {
		path string
		mod  time.Time
	}
	var kept []item
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if p == keepPath {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > maxUploadAge {
			_ = os.Remove(p)
			continue
		}
		kept = append(kept, item{path: p, mod: info.ModTime()})
	}
	if len(kept) >= maxUploadFiles {
		sort.Slice(kept, func(i, j int) bool {
			return kept[i].mod.After(kept[j].mod)
		})
		for i := maxUploadFiles - 1; i < len(kept); i++ {
			_ = os.Remove(kept[i].path)
		}
	}
}

func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' || r > 127 {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	res := b.String()
	if res == "" {
		return "sub.yaml"
	}
	return res
}

func (s *server) handlePreview(w http.ResponseWriter, r *http.Request) {
	var req previewRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	source, inferredName, err := materializeInlineSource(req.Source, req.Content, req.Filename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if source == "" {
		http.Error(w, "请先填写订阅链接或上传/粘贴节点配置", http.StatusBadRequest)
		return
	}
	if err := subscription.SetProxy(req.Proxy); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	nodes, subName, err := subscription.Load(source)
	if err != nil {
		http.Error(w, fmt.Sprintf("解析订阅失败: %v", err), http.StatusBadRequest)
		return
	}
	if subName == "" || inlineNamePrefixRe.MatchString(subName) {
		if inferredName != "" {
			subName = inferredName
		}
	}

	skipInfo := true
	if req.SkipInfo != nil {
		skipInfo = *req.SkipInfo
	}
	var filterRe, excludeRe *regexp.Regexp
	if strings.TrimSpace(req.Filter) != "" {
		filterRe, err = regexp.Compile(req.Filter)
		if err != nil {
			http.Error(w, fmt.Sprintf("包含正则错误: %v", err), http.StatusBadRequest)
			return
		}
	}
	if strings.TrimSpace(req.Exclude) != "" {
		excludeRe, err = regexp.Compile(req.Exclude)
		if err != nil {
			http.Error(w, fmt.Sprintf("排除正则错误: %v", err), http.StatusBadRequest)
			return
		}
	}

	outNodes := make([]previewNode, 0, len(nodes))
	matchedCount := 0
	for _, n := range nodes {
		flag, clean := render.ExtractNodeFlag(n.Name)
		if clean == "" {
			clean = n.Name
		}
		isInfo := runner.IsInfoNodeName(n.Name)
		matched := true
		if skipInfo && isInfo {
			matched = false
		}
		if matched && filterRe != nil && !filterRe.MatchString(n.Name) {
			matched = false
		}
		if matched && excludeRe != nil && excludeRe.MatchString(n.Name) {
			matched = false
		}
		if matched {
			matchedCount++
		}
		outNodes = append(outNodes, previewNode{
			Index:     n.Index,
			Name:      n.Name,
			CleanName: clean,
			Flag:      flag,
			Type:      n.Type,
			Server:    n.Server,
			IsInfo:    isInfo,
			Matched:   matched,
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]any{
		"source":       source,
		"subName":      subName,
		"total":        len(outNodes),
		"matchedCount": matchedCount,
		"nodes":        outNodes,
	})
}

func (s *server) handleReRender(w http.ResponseWriter, r *http.Request) {
	var req rerenderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	var targetDir string
	runID := strings.TrimSpace(req.RunID)
	if runID != "" {
		if !isSafeFilename(runID) {
			http.Error(w, "非法记录标识", http.StatusBadRequest)
			return
		}
		targetDir = filepath.Join("out", "web", runID)
	} else {
		s.mu.Lock()
		targetDir = s.outDir
		runID = s.runID
		s.mu.Unlock()
	}
	if targetDir == "" {
		http.Error(w, "未指定有效的测速记录目录", http.StatusBadRequest)
		return
	}

	files, err := runner.ReRender(targetDir, runner.ReRenderOptions{
		SortBy:    req.SortBy,
		Theme:     req.Theme,
		Watermark: req.Watermark,
		Bandwidth: req.Bandwidth,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("重绘失败: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":        true,
		"runId":     runID,
		"files":     files,
		"updatedAt": time.Now().UnixMilli(),
	})
}

func (s *server) handleRun(w http.ResponseWriter, r *http.Request) {
	var req runRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	source, inferredName, err := materializeInlineSource(req.Source, req.Content, req.Filename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Source = source
	if strings.TrimSpace(req.Name) == "" && inferredName != "" {
		req.Name = inferredName
	}
	if req.Source == "" {
		http.Error(w, "订阅链接/本地路径不能为空", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		http.Error(w, "已有任务在运行中", http.StatusConflict)
		return
	}
	s.running = true
	s.mu.Unlock()

	// 应用默认值（与 CLI flag 默认值一致）
	if req.Tests == "" {
		req.Tests = "all"
	}
	if req.Threads <= 0 {
		req.Threads = 8
	}
	if req.Threads == 1 {
		req.Single = true
	}
	if req.Duration <= 0 {
		req.Duration = 10
	}
	if req.Concurrency <= 0 {
		req.Concurrency = 6
	}
	if req.PingCount <= 0 {
		req.PingCount = 4
	}
	skipInfo := true
	if req.SkipInfo != nil {
		skipInfo = *req.SkipInfo
	}
	preflight := true
	if req.Preflight != nil {
		preflight = *req.Preflight
	}
	pingURL := strings.TrimSpace(req.PingURL)
	if pingURL == "" {
		pingURL = "http://www.gstatic.com/generate_204"
	}

	now := time.Now()
	runID := now.Format("20060102_150405")
	outDir := filepath.Join("out", "web", runID)
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.runID = runID
	s.startedAt = now
	s.outDir = outDir
	s.cancel = cancel
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "started",
		"outDir": outDir,
		"runId":  runID,
		"source": req.Source,
	})

	go func() {
		defer func() {
			cancel()
			s.mu.Lock()
			s.running = false
			s.cancel = nil
			s.mu.Unlock()
		}()

		s.hub.reset()
		s.hub.logf("输出目录：%s", outDir)
		out, err := runner.Run(ctx, runner.Options{
			Source:          req.Source,
			Name:            strings.TrimSpace(req.Name),
			Tests:           req.Tests,
			Threads:         req.Threads,
			Duration:        req.Duration,
			Concurrency:     req.Concurrency,
			PingCount:       req.PingCount,
			Single:          req.Single,
			Filter:          req.Filter,
			Exclude:         req.Exclude,
			SkipInfoNodes:   skipInfo,
			SortBy:          req.SortBy,
			MaxMB:           req.MaxMB,
			Theme:           strings.TrimSpace(req.Theme),
			Watermark:       strings.TrimSpace(req.Watermark),
			Bandwidth:       strings.TrimSpace(req.Bandwidth),
			SpeedURL:        strings.TrimSpace(req.SpeedURL),
			OutDir:          outDir,
			PingURL:         pingURL,
			Preflight:       preflight,
			Proxy:           req.Proxy,
			SelectedIndices: req.SelectedIndices,
			OnNodeDone: func(done, total int, nr *model.NodeResult) {
				s.hub.nodeDone(done, total, nr)
			},
		}, s.hub.logf)

		evt := doneEvent{OK: err == nil, RunID: runID}
		if err != nil {
			evt.Error = err.Error()
			s.hub.logf("任务失败：%v", err)
		} else {
			evt.Total = out.Total
			evt.Succeeded = out.Succeeded
			evt.Elapsed = out.Elapsed.String()
			for _, f := range out.Files {
				evt.Files = append(evt.Files, filepath.Base(f))
			}
		}
		s.hub.done(evt)
	}()
}

func (s *server) handleCancel(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	running := s.running
	cancel := s.cancel
	s.mu.Unlock()
	if !running || cancel == nil {
		http.Error(w, "当前没有运行中的任务", http.StatusBadRequest)
		return
	}
	s.hub.logf("收到停止指令，正在取消当前任务…")
	cancel()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]string{"status": "cancelling"})
}

func (s *server) handleHistory(w http.ResponseWriter, r *http.Request) {
	baseDir := filepath.Join("out", "web")
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode([]historyItem{})
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() > entries[j].Name()
	})
	var items []historyItem
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		runDir := filepath.Join(baseDir, e.Name())
		subEntries, err := os.ReadDir(runDir)
		if err != nil {
			continue
		}
		var files []string
		var jsonFile string
		var subPrefix string
		for _, f := range subEntries {
			if f.IsDir() {
				continue
			}
			fn := f.Name()
			if strings.HasSuffix(fn, ".png") || strings.HasSuffix(fn, "_result.json") {
				files = append(files, fn)
				if jsonFile == "" && strings.HasSuffix(fn, "_result.json") {
					jsonFile = fn
					subPrefix = strings.TrimSuffix(fn, "_result.json")
				}
			}
		}
		if len(files) == 0 {
			continue
		}
		subName := ""
		if m := stampSuffixRe.FindStringIndex(subPrefix); m != nil {
			subName = subPrefix[:m[0]]
		} else if !inlineNamePrefixRe.MatchString(subPrefix) {
			subName = subPrefix
		}
		var total, succeeded int
		var elapsed string
		if jsonFile != "" {
			if raw, err := os.ReadFile(filepath.Join(runDir, jsonFile)); err == nil {
				var summary struct {
					SubName   string `json:"subName"`
					Total     int    `json:"total"`
					Succeeded int    `json:"succeeded"`
					Elapsed   string `json:"elapsed"`
				}
				if json.Unmarshal(raw, &summary) == nil {
					if summary.SubName != "" {
						subName = summary.SubName
					}
					total = summary.Total
					succeeded = summary.Succeeded
					elapsed = summary.Elapsed
				}
			}
		}
		title := e.Name()
		if subName != "" {
			title = e.Name() + " · " + subName
		}
		if total > 0 {
			if elapsed != "" {
				title = fmt.Sprintf("%s (%d/%d · %s)", title, succeeded, total, elapsed)
			} else {
				title = fmt.Sprintf("%s (%d/%d)", title, succeeded, total)
			}
		}
		items = append(items, historyItem{
			ID:        e.Name(),
			Title:     title,
			SubName:   subName,
			Total:     total,
			Succeeded: succeeded,
			Elapsed:   elapsed,
			Files:     files,
		})
		if len(items) >= 50 {
			break
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(items)
}

func (s *server) handleDeleteHistory(w http.ResponseWriter, r *http.Request) {
	run := r.PathValue("run")
	if !isSafeFilename(run) {
		http.Error(w, "非法记录标识", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	active := s.running && s.runID == run
	s.mu.Unlock()
	if active {
		http.Error(w, "不能删除正在运行的记录", http.StatusConflict)
		return
	}
	target := filepath.Join("out", "web", run)
	if err := os.RemoveAll(target); err != nil {
		http.Error(w, fmt.Sprintf("删除失败: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "id": run})
}

func (s *server) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "不支持 SSE", http.StatusInternalServerError)
		return
	}

	ch, replay := s.hub.subscribe()
	defer s.hub.unsubscribe(ch)
	for _, frame := range replay {
		fmt.Fprint(w, frame)
	}
	flusher.Flush()

	for {
		select {
		case frame := <-ch:
			fmt.Fprint(w, frame)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *server) handleFiles(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !isSafeFilename(name) {
		http.Error(w, "非法文件名", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	dir := s.outDir
	s.mu.Unlock()
	if dir == "" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join(dir, name))
}

func (s *server) handleHistoryFiles(w http.ResponseWriter, r *http.Request) {
	run := r.PathValue("run")
	name := r.PathValue("name")
	if !isSafeFilename(run) || !isSafeFilename(name) {
		http.Error(w, "非法路径参数", http.StatusBadRequest)
		return
	}
	http.ServeFile(w, r, filepath.Join("out", "web", run, name))
}

func isSafeFilename(s string) bool {
	return s != "" && s == filepath.Base(s) && !strings.HasPrefix(s, ".") && !strings.ContainsAny(s, `/\`)
}

// hub 管理 SSE 订阅者并缓存历史日志帧，后连接的客户端能补齐之前的日志。
type hub struct {
	mu      sync.Mutex
	history []string
	subs    map[chan string]struct{}
}

func newHub() *hub {
	return &hub{subs: make(map[chan string]struct{})}
}

func (h *hub) reset() {
	h.mu.Lock()
	h.history = nil
	h.mu.Unlock()
}

func (h *hub) logf(format string, args ...any) {
	h.broadcast("data: " + fmt.Sprintf(format, args...) + "\n\n")
}

func (h *hub) nodeDone(done, total int, r *model.NodeResult) {
	if r == nil {
		return
	}
	evt := nodeEvent{
		Done:  done,
		Total: total,
		Node: nodeRecord{
			Index:    r.Node.Index,
			Name:     r.Node.Name,
			Type:     r.Node.Type,
			Server:   r.Node.Server,
			Latency:  r.Latency,
			Speed:    r.Speed,
			NAT:      r.NAT,
			Media:    r.Media,
			AI:       r.AI,
			Inbound:  r.Inbound,
			Outbound: r.Outbound,
			Error:    r.Err,
		},
	}
	data, err := json.Marshal(evt)
	if err != nil {
		return
	}
	h.broadcast("event: node\ndata: " + string(data) + "\n\n")
}

func (h *hub) done(evt doneEvent) {
	data, _ := json.Marshal(evt)
	h.broadcast("event: done\ndata: " + string(data) + "\n\n")
}

func (h *hub) broadcast(frame string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.history) >= maxHubHistory {
		trimmed := make([]string, maxHubHistory/2, maxHubHistory)
		copy(trimmed, h.history[len(h.history)-maxHubHistory/2:])
		h.history = trimmed
	}
	h.history = append(h.history, frame)
	for ch := range h.subs {
		select {
		case ch <- frame:
		default: // 客户端读取慢时丢弃，避免阻塞任务
		}
	}
}

func (h *hub) subscribe() (chan string, []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan string, 256)
	h.subs[ch] = struct{}{}
	return ch, append([]string(nil), h.history...)
}

func (h *hub) unsubscribe(ch chan string) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

// openBrowser 按平台打开默认浏览器，失败仅提示手动访问。
func openBrowser(url string) {
	if os.Getenv("NEKOSPEED_NO_BROWSER") != "" {
		return
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", url)
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		default:
			cmd = exec.Command("xdg-open", url)
		}
		if err := cmd.Run(); err != nil {
			fmt.Printf("请手动在浏览器中打开：%s\n", url)
		}
	}()
}
