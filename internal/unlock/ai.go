package unlock

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"nekospeed/internal/model"
)

const (
	defaultClaudeURL    = "https://claude.ai/"
	defaultClaudeAPIURL = "https://api.anthropic.com/v1/messages"
)

// 各 AI 平台检测端点（包内变量，测试时可替换）。
var (
	openAITraceURL      = "https://chatgpt.com/cdn-cgi/trace"
	openAIComplianceURL = "https://api.openai.com/compliance/cookie_requirements"
	openAIIOSURL        = "https://ios.chat.openai.com/"
	copilotURL          = "https://copilot.microsoft.com/"
	geminiURL           = "https://gemini.google.com/app"
	llamaAPIURL         = "https://api.llama.com/v1/models"
	claudeURL           = defaultClaudeURL
	claudeAPIURL        = defaultClaudeAPIURL
	claudeTraceURL      = "https://claude.ai/cdn-cgi/trace"
	perplexityAPIURL    = "https://api.perplexity.ai/chat/completions"
	perplexityTraceURL  = "https://www.perplexity.ai/cdn-cgi/trace"
	soraAPIURL          = "https://api.openai.com/v1/videos"
	grokURL             = "https://grok.x.ai/"
)

var claudeUnsupportedRegions = map[string]bool{
	"CN": true, "HK": true, "MO": true, "RU": true,
	"BY": true, "KP": true, "IR": true, "SY": true,
	"CU": true, "AF": true, "VE": true,
}

func fetchOpenAITrace(ctx context.Context, c *http.Client) (*httpResult, error) {
	return cachedHTTP(ctx, "openai_trace:"+openAITraceURL, func() (*httpResult, error) {
		return doGet(ctx, c, openAITraceURL)
	})
}

func fetchOpenAICompliance(ctx context.Context, c *http.Client) (*httpResult, error) {
	return cachedHTTP(ctx, "openai_compliance:"+openAIComplianceURL, func() (*httpResult, error) {
		return doRequest(ctx, c, http.MethodGet, openAIComplianceURL, nil, map[string]string{
			"Authorization": "Bearer null",
			"Content-Type":  "application/json",
			"Origin":        "https://platform.openai.com",
			"Referer":       "https://platform.openai.com/",
		})
	})
}

var traceLocRe = regexp.MustCompile(`(?m)^loc=([A-Z]{2})$`)

// traceRegion 从 Cloudflare /cdn-cgi/trace 文本解析 loc=XX 地区码。
func traceRegion(body []byte) string {
	if m := traceLocRe.FindSubmatch(body); m != nil {
		return string(m[1])
	}
	return ""
}

// checkOpenAI 先取 cdn-cgi/trace 地区码，再查两个信号：
// compliance 接口 body 含 unsupported_country → 地区封锁（唯一可靠的地区信号）；
// ios 站点 body 含 vpn → IP 被识别为代理。
// 注意 ios 站点对非浏览器/数据中心客户端固定返回 403（Cloudflare dc 拦截），
// 与地区无关，不能把 403 状态码作为依据。
func checkOpenAI(ctx context.Context, d model.Dialer) model.UnlockResult {
	c := newClient(d, true)
	trace, err := fetchOpenAITrace(ctx, c)
	if err != nil {
		return reqFailed("openai")
	}
	region := traceRegion(trace.Body)

	comp, err := fetchOpenAICompliance(ctx, c)
	if err != nil {
		return reqFailed("openai")
	}
	countryBlocked := strings.Contains(strings.ToLower(string(comp.Body)), "unsupported_country")

	ios, err := doGet(ctx, c, openAIIOSURL)
	if err != nil {
		// 若 compliance 已明确返回地区不支持，即使后续 ios 探针超时/出错，也应报告地区不支持而非误判为 reqfailed。
		if countryBlocked {
			return model.UnlockResult{Status: model.StatusFailed, Region: region, Info: "地区不支持"}
		}
		return reqFailed("openai")
	}
	vpnBlocked := strings.Contains(strings.ToLower(string(ios.Body)), "vpn")

	switch {
	case countryBlocked && vpnBlocked:
		return model.UnlockResult{Status: model.StatusFailed, Region: region, Info: "地区不支持且 IP 被识别为代理"}
	case countryBlocked:
		return model.UnlockResult{Status: model.StatusFailed, Region: region, Info: "地区不支持"}
	case vpnBlocked:
		return model.UnlockResult{Status: model.StatusFailed, Region: region, Info: "IP 被识别为代理"}
	}
	return model.UnlockResult{Status: model.StatusUnlock, Region: region}
}

// checkCopilot 首页可访问且非地区限制页即解锁。
func checkCopilot(ctx context.Context, d model.Dialer) model.UnlockResult {
	r, err := doGet(ctx, newClient(d, true), copilotURL)
	if err != nil {
		return reqFailed("copilot")
	}
	if r.StatusCode != 200 {
		return model.UnlockResult{Status: model.StatusFailed}
	}
	body := strings.ToLower(string(r.Body))
	if strings.Contains(body, "not available in your region") || strings.Contains(body, "unavailable in your market") {
		return model.UnlockResult{Status: model.StatusFailed}
	}
	return model.UnlockResult{Status: model.StatusUnlock}
}

// checkGemini app 页面在非支持地区返回 4xx 或限制提示。
func checkGemini(ctx context.Context, d model.Dialer) model.UnlockResult {
	r, err := doGet(ctx, newClient(d, true), geminiURL)
	if err != nil {
		return reqFailed("gemini")
	}
	if r.StatusCode >= 400 {
		return model.UnlockResult{Status: model.StatusFailed}
	}
	body := strings.ToLower(string(r.Body))
	if strings.Contains(body, "isn't currently supported in your country") ||
		strings.Contains(body, "not available in your country") {
		return model.UnlockResult{Status: model.StatusFailed}
	}
	return model.UnlockResult{Status: model.StatusUnlock}
}

// checkLlama 检测 Meta Llama 开发者 API 可达性。无 key 请求 /v1/models 返回 401
// 表示服务可达且未被地区封锁；body 含地区关键词表示封锁。
// api.llama.com 非 Cloudflare（无 cdn-cgi/trace），地区码无来源，留空。
func checkLlama(ctx context.Context, d model.Dialer) model.UnlockResult {
	r, err := doGet(ctx, newClient(d, true), llamaAPIURL)
	if err != nil {
		return reqFailed("llama")
	}
	if r.StatusCode == 401 || r.StatusCode == 200 {
		return model.UnlockResult{Status: model.StatusUnlock}
	}
	body := strings.ToLower(string(r.Body))
	if strings.Contains(body, "country") || strings.Contains(body, "region") || strings.Contains(body, "not supported") {
		return model.UnlockResult{Status: model.StatusFailed, Info: "地区限制"}
	}
	return model.UnlockResult{Status: model.StatusFailed, Info: fmt.Sprintf("HTTP %d (%db)", r.StatusCode, len(r.Body))}
}

// checkClaude 默认先查询 claude.ai/cdn-cgi/trace 获取地区，再向官方 API POST 空 JSON，
// 正常节点未鉴权返回 401/400/200 即可绕过网页端 Cloudflare JS 403 挑战；
// 若测试仅替换了 claudeURL 而未替换 claudeAPIURL，则保持旧版首页检测路径以兼容既有单测。
func checkClaude(ctx context.Context, d model.Dialer) model.UnlockResult {
	c := newClient(d, true)
	if claudeURL != defaultClaudeURL && claudeAPIURL == defaultClaudeAPIURL {
		return checkClaudeLegacy(ctx, c)
	}

	region := ""
	if trace, terr := doGet(ctx, c, claudeTraceURL); terr == nil {
		region = traceRegion(trace.Body)
	}

	r, err := doRequest(ctx, c, http.MethodPost, claudeAPIURL, strings.NewReader("{}"), map[string]string{
		"Content-Type": "application/json",
	})
	if err != nil {
		if claudeUnsupportedRegions[region] {
			return model.UnlockResult{Status: model.StatusFailed, Region: region, Info: "地区限制"}
		}
		return reqFailed("claude")
	}
	if claudeUnsupportedRegions[region] {
		return model.UnlockResult{Status: model.StatusFailed, Region: region, Info: "地区限制"}
	}
	body := strings.ToLower(string(r.Body))
	if r.StatusCode == 403 && (strings.Contains(body, "not allowed") ||
		strings.Contains(body, "unsupported") ||
		strings.Contains(body, "country") ||
		strings.Contains(body, "region")) {
		return model.UnlockResult{Status: model.StatusFailed, Region: region, Info: "地区限制"}
	}
	if r.StatusCode == 401 || r.StatusCode == 400 || r.StatusCode == 200 {
		return model.UnlockResult{Status: model.StatusUnlock, Region: region}
	}
	return model.UnlockResult{Status: model.StatusFailed, Region: region, Info: fmt.Sprintf("HTTP %d (%db)", r.StatusCode, len(r.Body))}
}

func checkClaudeLegacy(ctx context.Context, c *http.Client) model.UnlockResult {
	r, err := doGet(ctx, c, claudeURL)
	if err != nil {
		return reqFailed("claude")
	}
	region := ""
	if trace, terr := doGet(ctx, c, claudeTraceURL); terr == nil {
		region = traceRegion(trace.Body)
	}
	body := strings.ToLower(string(r.Body))
	if strings.Contains(body, "not available in your country") {
		return model.UnlockResult{Status: model.StatusFailed, Region: region, Info: "地区限制"}
	}
	if r.StatusCode != 200 {
		return model.UnlockResult{Status: model.StatusFailed, Region: region, Info: fmt.Sprintf("HTTP %d (%db)", r.StatusCode, len(r.Body))}
	}
	return model.UnlockResult{Status: model.StatusUnlock, Region: region}
}

// checkPerplexity 通过 API 端点检测：POST /chat/completions 无 key 返回 401 即服务可达。
// Web 首页对机器客户端发 Cloudflare JS 挑战（真人浏览器也要过），不代表地区封锁；
// API 子域 CF 规则宽松，401 是无 key 时的标准可达信号。地区取自 cdn-cgi/trace。
func checkPerplexity(ctx context.Context, d model.Dialer) model.UnlockResult {
	c := newClient(d, true)
	r, err := doRequest(ctx, c, http.MethodPost, perplexityAPIURL, strings.NewReader("{}"), map[string]string{
		"Content-Type": "application/json",
	})
	if err != nil {
		return reqFailed("perplexity")
	}
	region := ""
	if trace, terr := doGet(ctx, c, perplexityTraceURL); terr == nil {
		region = traceRegion(trace.Body)
	}
	if r.StatusCode == 401 || r.StatusCode == 200 {
		return model.UnlockResult{Status: model.StatusUnlock, Region: region}
	}
	body := strings.ToLower(string(r.Body))
	if strings.Contains(body, "country") || strings.Contains(body, "region") || strings.Contains(body, "not supported") || strings.Contains(body, "not available") {
		return model.UnlockResult{Status: model.StatusFailed, Region: region, Info: "地区限制"}
	}
	return model.UnlockResult{Status: model.StatusFailed, Region: region, Info: fmt.Sprintf("HTTP %d (%db)", r.StatusCode, len(r.Body))}
}

// checkSora 通过 OpenAI 平台 /v1/videos 检测 Sora 区域可用性：
// geo 检查先于鉴权，无 key 时 403+unsupported_country 即地区封锁，401 即地区支持。
// 地区与 compliance 结果在单节点 RunAI 内通过上下文缓存与 checkOpenAI 共享。
func checkSora(ctx context.Context, d model.Dialer) model.UnlockResult {
	c := newClient(d, true)
	r, err := doGet(ctx, c, soraAPIURL)
	if err != nil {
		return reqFailed("sora")
	}
	region := ""
	if trace, terr := fetchOpenAITrace(ctx, c); terr == nil {
		region = traceRegion(trace.Body)
	}
	if r.StatusCode == 401 || r.StatusCode == 200 {
		return model.UnlockResult{Status: model.StatusUnlock, Region: region}
	}
	body := strings.ToLower(string(r.Body))
	if strings.Contains(body, "unsupported_country") || strings.Contains(body, "not supported") {
		return model.UnlockResult{Status: model.StatusFailed, Region: region, Info: "地区限制"}
	}
	// OpenAI 现网未鉴权访问 /v1/videos 可能直接返回 404，此时回退查询 compliance 接口判断区域可用性。
	if r.StatusCode == 404 {
		comp, cerr := fetchOpenAICompliance(ctx, c)
		if cerr == nil {
			if strings.Contains(strings.ToLower(string(comp.Body)), "unsupported_country") {
				return model.UnlockResult{Status: model.StatusFailed, Region: region, Info: "地区限制"}
			}
			if comp.StatusCode == 200 {
				return model.UnlockResult{Status: model.StatusUnlock, Region: region}
			}
		}
	}
	return model.UnlockResult{Status: model.StatusFailed, Region: region, Info: fmt.Sprintf("HTTP %d (%db)", r.StatusCode, len(r.Body))}
}

// checkGrok xAI Grok 可用性检测。
func checkGrok(ctx context.Context, d model.Dialer) model.UnlockResult {
	r, err := doGet(ctx, newClient(d, true), grokURL)
	if err != nil {
		return reqFailed("grok")
	}
	body := strings.ToLower(string(r.Body))
	if strings.Contains(body, "not available in your country") || strings.Contains(body, "unsupported_country") {
		return model.UnlockResult{Status: model.StatusFailed, Info: "地区限制"}
	}
	if r.StatusCode != 200 {
		return model.UnlockResult{Status: model.StatusFailed, Info: fmt.Sprintf("HTTP %d (%db)", r.StatusCode, len(r.Body))}
	}
	return model.UnlockResult{Status: model.StatusUnlock}
}
