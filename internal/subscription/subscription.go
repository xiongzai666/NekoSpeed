// Package subscription 负责从订阅 URL 或本地文件加载节点列表。
// 支持 Clash YAML 配置与 base64 / 明文分享链接订阅两种格式。
package subscription

import (
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"nekospeed/internal/model"
	"nekospeed/internal/nodepool"
)

// defaultUserAgent 拉取订阅时使用的 UA。Clash 机场面板通常按 UA 下发 clash 配置，
// 但部分面板会按 JA3/TLS fingerprint 或 UA 做风控，所以使用一个常见 Clash.Meta Android UA。
const defaultUserAgent = "ClashMetaForAndroid/2.11.4 Meta/1.18.0 Android/14"

// httpTimeout 订阅拉取单次超时。
const httpTimeout = 30 * time.Second

// fetchRetry 订阅拉取重试次数。
const fetchRetry = 3

// maxBody 订阅内容大小上限（16 MiB），防止异常响应撑爆内存。
const maxBody = 16 << 20

// proxyMu 保护 proxyFn 与 explicitProxy，允许运行时通过 SetProxy 覆盖。
var proxyMu sync.RWMutex

// proxyFn 决定订阅请求使用的代理，默认读取 HTTP_PROXY / HTTPS_PROXY 等环境变量。
var proxyFn = http.ProxyFromEnvironment

// explicitProxy 标记是否通过 SetProxy 显式设置了代理。
var explicitProxy bool

// SetProxy 设置订阅拉取使用的代理（如 http://127.0.0.1:7890 或 socks5://127.0.0.1:1080）。
// 传空字符串恢复为默认的环境变量代理。订阅域名被 DNS 污染 / SNI 重置时通常需要走代理拉取。
func SetProxy(proxy string) error {
	pfn, explicit, err := parseProxyFunc(proxy)
	if err != nil {
		return err
	}
	proxyMu.Lock()
	proxyFn = pfn
	explicitProxy = explicit
	proxyMu.Unlock()
	return nil
}

func parseProxyFunc(proxy string) (func(*http.Request) (*url.URL, error), bool, error) {
	if proxy == "" {
		return http.ProxyFromEnvironment, false, nil
	}
	u, err := url.Parse(proxy)
	if err != nil {
		return nil, false, fmt.Errorf("解析代理地址 %q 失败: %w", proxy, err)
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, false, fmt.Errorf("不支持的代理协议 %q（支持 http/https/socks5）", u.Scheme)
	}
	return http.ProxyURL(u), true, nil
}

type fetchConfig struct {
	proxyFn       func(*http.Request) (*url.URL, error)
	explicitProxy bool
}

// Load 从 source 加载节点。source 为 http(s) URL 时发起 GET 拉取，否则按本地文件读取。
// 拿到字节后自动判别格式：含顶层 proxies: 键的 YAML 按 Clash 配置解析；
// 否则尝试整体 base64 解码后按行解析分享链接；原文本直接以协议 scheme 开头时直接按行解析。
// 同时返回尽可能提取到的订阅名称（用于输出文件名前缀）。
func Load(source string) ([]model.Node, string, error) {
	proxyMu.RLock()
	cfg := fetchConfig{
		proxyFn:       proxyFn,
		explicitProxy: explicitProxy,
	}
	proxyMu.RUnlock()
	return loadWithConfig(context.Background(), source, cfg)
}

// LoadContext 与 Load 相同，但支持通过 ctx 取消请求/退避等待，并为本次调用单独指定 proxy，
// 不修改全局 proxyFn / explicitProxy 状态，适合 Web UI 等并发场景。
func LoadContext(ctx context.Context, source, proxy string) ([]model.Node, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	pfn, explicit, err := parseProxyFunc(proxy)
	if err != nil {
		return nil, "", err
	}
	return loadWithConfig(ctx, source, fetchConfig{
		proxyFn:       pfn,
		explicitProxy: explicit,
	})
}

func loadWithConfig(ctx context.Context, source string, cfg fetchConfig) ([]model.Node, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", fmt.Errorf("读取订阅 %q: %w", source, err)
	}
	var (
		data   []byte
		name   string
		header http.Header
		err    error
	)
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		data, header, err = fetchWithConfig(ctx, source, cfg)
		name = extractName(source, data, header)
	} else {
		data, err = os.ReadFile(source)
		// 文件内容可能只是一行订阅 URL（Android 端为避免 URL 暴露在进程命令行而落盘），
		// 此时按 URL 拉取。
		if err == nil {
			if s := strings.TrimSpace(string(data)); strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
				data, header, err = fetchWithConfig(ctx, s, cfg)
				name = extractName(s, data, header)
			} else {
				name = extractName(source, data, nil)
			}
		}
	}
	if err != nil {
		return nil, "", fmt.Errorf("读取订阅 %q: %w", source, err)
	}
	nodes, err := parse(data, 0)
	if err != nil {
		return nil, "", err
	}
	return nodes, name, nil
}

// fetchWithConfig HTTP GET 拉取订阅内容，返回响应体与响应头。
// 失败时自动重试 fetchRetry 次；第二次起降级到 HTTP/1.1，避免部分机场对 HTTP/2 或 TLS fingerprint 的风控。
// 若在本机 TUN 开启且未显式指定代理时遭遇 403，回退至绑定物理网卡直连拉取以绕过部分机场的海外 IP 拦截。
func fetchWithConfig(ctx context.Context, targetURL string, cfg fetchConfig) ([]byte, http.Header, error) {
	var (
		lastErr        error
		data           []byte
		header         http.Header
		statusCode     int
		directPhysical bool
	)
	for attempt := 0; attempt < fetchRetry; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		forceHTTP11 := attempt > 0
		data, header, statusCode, lastErr = fetchOnce(ctx, targetURL, forceHTTP11, directPhysical, cfg)
		if lastErr == nil {
			return data, header, nil
		}
		if ctx.Err() != nil {
			return nil, nil, lastErr
		}
		if !directPhysical && statusCode == http.StatusForbidden && shouldFallbackDirectPhysicalWithExplicit(targetURL, nodepool.GetNetworkEnv().TUNDetected, cfg.explicitProxy, http.ProxyFromEnvironment) {
			directPhysical = true
		}
		if attempt < fetchRetry-1 {
			timer := time.NewTimer(time.Duration(attempt+1) * 500 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return nil, nil, lastErr
}

func shouldFallbackDirectPhysical(targetURL string, tunDetected bool, envProxyFn func(*http.Request) (*url.URL, error)) bool {
	proxyMu.RLock()
	hasExplicit := explicitProxy
	proxyMu.RUnlock()
	return shouldFallbackDirectPhysicalWithExplicit(targetURL, tunDetected, hasExplicit, envProxyFn)
}

func shouldFallbackDirectPhysicalWithExplicit(targetURL string, tunDetected, hasExplicit bool, envProxyFn func(*http.Request) (*url.URL, error)) bool {
	if !tunDetected || hasExplicit {
		return false
	}
	u, err := url.Parse(targetURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "" || strings.EqualFold(host, "localhost") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return false
	}
	if envProxyFn != nil {
		envProxy, err := envProxyFn(&http.Request{URL: u})
		if err != nil || envProxy != nil {
			return false
		}
	}
	return true
}

func fetchOnce(ctx context.Context, targetURL string, forceHTTP11, directPhysical bool, cfg fetchConfig) ([]byte, http.Header, int, error) {
	transport := &http.Transport{
		Proxy: cfg.proxyFn,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}
	defer transport.CloseIdleConnections()
	if directPhysical {
		transport.Proxy = nil
		transport.DialContext = nodepool.DialDirectContext
	}
	if forceHTTP11 {
		transport.TLSClientConfig.NextProtos = []string{"http/1.1"}
		transport.ForceAttemptHTTP2 = false
	}

	client := &http.Client{
		Timeout:   httpTimeout,
		Transport: transport,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, nil, 0, err
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept", "application/json,text/plain,text/html,*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, resp.StatusCode, fmt.Errorf("订阅请求返回 %s", resp.Status)
	}
	// 请求头手动声明了 Accept-Encoding: gzip, deflate，net/http 不会自动解压，需按 Content-Encoding 自行解压。
	reader := io.Reader(resp.Body)
	switch strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding"))) {
	case "gzip":
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, nil, resp.StatusCode, fmt.Errorf("解压 gzip 订阅响应失败: %w", err)
		}
		defer gr.Close()
		reader = gr
	case "deflate":
		zr, err := zlib.NewReader(resp.Body)
		if err != nil {
			return nil, nil, resp.StatusCode, fmt.Errorf("解压 deflate 订阅响应失败: %w", err)
		}
		defer zr.Close()
		reader = zr
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxBody))
	if err != nil {
		return nil, nil, resp.StatusCode, err
	}
	return data, resp.Header, resp.StatusCode, nil
}

// extractName 尝试从订阅来源、响应头或内容中提取订阅名称。
// 名称会经过 sanitizeName 处理，可直接用于文件名前缀；提取不到时返回空字符串。
func extractName(source string, data []byte, header http.Header) string {
	lower := strings.ToLower(source)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "file://") {
		return sanitizeName(filepath.Base(source))
	}
	u, err := url.Parse(source)
	if err == nil {
		if q := u.Query().Get("name"); q != "" {
			return sanitizeName(q)
		}
		if q := u.Query().Get("title"); q != "" {
			return sanitizeName(q)
		}
		if u.Scheme == "" || u.Scheme == "file" {
			return sanitizeName(filepath.Base(source))
		}
		if header != nil {
			if _, params, err := mime.ParseMediaType(header.Get("Content-Disposition")); err == nil {
				if fn := params["filename"]; fn != "" {
					return sanitizeName(fn)
				}
			}
		}
		if contentName := extractNameFromContent(data); contentName != "" {
			return contentName
		}
		if seg := path.Base(u.Path); seg != "" && seg != "." && seg != "/" {
			return sanitizeName(seg)
		}
	}
	return extractNameFromContent(data)
}

// extractNameFromContent 从订阅内容首行注释中提取名称。
// 例如 "# 一分机场" 或 "# name: 机场名称" 会提取出 "一分机场" / "机场名称"。
func extractNameFromContent(data []byte) string {
	text := string(data)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "#") {
			return ""
		}
		s := strings.TrimSpace(strings.TrimPrefix(line, "#"))
		for _, prefix := range []string{"name:", "名称：", "名称:", "机场：", "机场:"} {
			if strings.HasPrefix(strings.ToLower(s), prefix) {
				s = strings.TrimSpace(s[len(prefix):])
				break
			}
		}
		return sanitizeName(s)
	}
	return ""
}

var (
	invalidNameCharsRe        = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]+`)
	collapseSpaceUnderscoreRe = regexp.MustCompile(`[\s_]+`)
)

// sanitizeName 把名称整理为安全的文件名前缀。
// 保留中文等字符，只替换文件系统非法 ASCII 字符；过长时截断。
func sanitizeName(s string) string {
	if s == "" {
		return ""
	}
	s = invalidNameCharsRe.ReplaceAllString(s, "_")
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ".yaml")
	s = strings.TrimSuffix(s, ".yml")
	s = strings.TrimSuffix(s, ".txt")
	s = strings.TrimSuffix(s, ".json")
	s = collapseSpaceUnderscoreRe.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if len([]rune(s)) > 64 {
		s = string([]rune(s)[:64])
		s = strings.TrimRight(s, "_")
	}
	return s
}

// parse 判别内容格式并解析为节点列表。depth 防止 base64 解码后递归过深。
func parse(data []byte, depth int) ([]model.Node, error) {
	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil, fmt.Errorf("订阅内容为空")
	}

	// 1. 含顶层 proxies: 键的 YAML → Clash 配置
	nodes, ok, err := parseClash(data)
	if err != nil {
		return nil, err
	}
	if ok {
		if len(nodes) == 0 {
			return nil, fmt.Errorf("Clash 配置中没有可用代理节点")
		}
		return nodes, nil
	}

	// 2. 尝试整体 base64 解码（标准 + RawURL 等编码都试），解码后递归判别
	if depth == 0 {
		if decoded, err := base64DecodeAny(text); err == nil {
			// 解码结果必须像文本订阅（含 scheme 或 YAML），否则说明原文本不是 base64
			if looksLikeSubscription(string(decoded)) {
				return parse(decoded, depth+1)
			}
		}
	}

	// 3. 直接按行解析分享链接
	nodes = parseShareLines(splitLines(text))
	if len(nodes) == 0 {
		return nil, fmt.Errorf("未解析到任何节点（无法识别的订阅格式）")
	}
	return nodes, nil
}

// looksLikeSubscription 粗略判断文本是否为订阅内容（分享链接或 YAML 配置）。
func looksLikeSubscription(text string) bool {
	t := strings.TrimSpace(text)
	if strings.Contains(t, "proxies:") {
		return true
	}
	for _, line := range splitLines(t) {
		if isShareLink(line) {
			return true
		}
	}
	return false
}

// splitLines 按行拆分并去除空行与注释。
func splitLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") && !isShareLink(line) {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// schemeOf 提取分享链接的协议前缀（vless://、ss:// 等），用于日志脱敏。
func schemeOf(line string) string {
	if i := strings.Index(line, "://"); i > 0 && i < 16 {
		return line[:i] + "://"
	}
	return "unknown"
}

// parseShareLines 逐行解析分享链接，解析失败的行跳过并 log 到 stderr，不整体失败。
func parseShareLines(lines []string) []model.Node {
	var nodes []model.Node
	failed := 0
	for _, line := range lines {
		if !isShareLink(line) {
			continue
		}
		node, err := ParseShareLink(line)
		if err != nil {
			failed++
			// 隐私：只打印协议类型与错误，不打印原始节点行（可能含明文密码）
			fmt.Fprintf(os.Stderr, "[subscription] 跳过无法解析的节点: %v (%s)\n", err, schemeOf(line))
			continue
		}
		node.Index = len(nodes) + 1
		nodes = append(nodes, node)
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "[subscription] 共跳过 %d 条无法解析的节点\n", failed)
	}
	return nodes
}
