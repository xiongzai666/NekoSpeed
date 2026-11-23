package unlock

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"

	"nekospeed/internal/model"
)

const (
	defaultAbemaCheckURL    = "https://abema.tv/"
	defaultAbemaAPIURL      = "https://api.abema.io/v1/ip/check?device=android"
	defaultTikTokURL        = "https://www.tiktok.com/"
	defaultTikTokAPIURL     = "https://www.tiktok.com/explore"
	defaultDisneyPlusURL    = "https://www.disneyplus.com/"
	defaultDisneyDevicesURL = "https://disney.api.edge.bamgrid.com/devices"
	defaultDisneyTokenURL   = "https://disney.api.edge.bamgrid.com/token"
	defaultDisneyGraphQLURL = "https://disney.api.edge.bamgrid.com/graph/v1/device/graphql"
	disneyAuthToken         = "ZGlzbmV5JmJyb3dzZXImMS4wLjA.Cu56AgSfBTDag5NiRA81oLHkDZfu5L3CKadnefEAY84"
)

// 各平台检测端点（包内变量，测试时可替换为 httptest 服务器地址）。
var (
	youtubeURL           = "https://www.youtube.com/premium"
	netflixNonOriginalID = "https://www.netflix.com/title/81280792" // 非自制剧（乐高幻影忍者），各区库不同步
	netflixOriginalID    = "https://www.netflix.com/title/80057281" // 自制剧（怪奇物语），有网飞即可看
	disneyPlusURL        = defaultDisneyPlusURL
	disneyDevicesURL     = defaultDisneyDevicesURL
	disneyTokenURL       = defaultDisneyTokenURL
	disneyGraphQLURL     = defaultDisneyGraphQLURL
	bilibiliPlayURL      = "https://api.bilibili.com/pgc/player/web/playurl?avid=18281381&cid=29892777&qn=0&type=&otype=json&ep_id=183799&fourk=1&fnver=0&fnval=16&module=bangumi" // 港澳台限定剧集（2026-07 验证：CN/US 返回 -10403）；旧剧集 ep_id=307247 已被纳入大陆片库会误判
	tiktokURL            = defaultTikTokURL
	tiktokAPIURL         = defaultTikTokAPIURL
	daznStartupURL       = "https://startup.core.indazn.com/misl/v5/Startup"
	abemaCheckURL        = defaultAbemaCheckURL
	abemaAPIURL          = defaultAbemaAPIURL
	bahamutDeviceURL     = "https://ani.gamer.com.tw/ajax/getdeviceid.php"
	bahamutTokenURL      = "https://ani.gamer.com.tw/ajax/token.php?adID=89422&sn=37783"
	bahamutHomeURL       = "https://ani.gamer.com.tw/"
	steamAppURL          = "https://store.steampowered.com/app/761830"
	spotifySignupURL     = "https://www.spotify.com/signup/"
)

var (
	youtubeGLRe  = regexp.MustCompile(`"gl"\s*:\s*"([A-Z]{2})"`)
	youtubeURLRe = regexp.MustCompile(`[?&]gl=([A-Z]{2})`)
	netflixLocRe = regexp.MustCompile(`"requestCountry"\s*:\s*\{[^}]*"id"\s*:\s*"([A-Z]{2})"`)
	tiktokRegRe  = regexp.MustCompile(`"region"\s*:\s*"([a-zA-Z]{2})"`)
	daznRegionRe = regexp.MustCompile(`"(?i:geoLocatedCountry|country|region)[A-Za-z]*"\s*:\s*"([A-Z]{2})"`)
	steamCurrRe     = regexp.MustCompile(`itemprop="priceCurrency"\s+content="([A-Z]{3})"`)
	bahamutGeoRe    = regexp.MustCompile(`data-geo="([^"]+)"`)
	disneyInSupRe   = regexp.MustCompile(`"inSupportedLocation"\s*:\s*(true|false)`)
	disneyCountryRe = regexp.MustCompile(`"countryCode"\s*:\s*"([A-Za-z]{2})"`)
)

// randHex16 生成 16 字节随机 hex（bilibili session 参数用）。
func randHex16() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "0123456789abcdef0123456789abcdef"
	}
	return hex.EncodeToString(b)
}

// checkYouTube 通过 premium 页面提取 gl=XX 地区码；CN 或无地区码视为送中。
func checkYouTube(ctx context.Context, d model.Dialer) model.UnlockResult {
	r, err := doGet(ctx, newClient(d, true), youtubeURL)
	if err != nil {
		return reqFailed("youtube")
	}
	if r.StatusCode != 200 {
		return model.UnlockResult{Status: model.StatusFailed}
	}
	region := ""
	if m := youtubeGLRe.FindSubmatch(r.Body); m != nil {
		region = string(m[1])
	} else if m := youtubeURLRe.FindStringSubmatch(r.FinalURL); m != nil {
		region = m[1]
	}
	if region == "" || region == "CN" {
		return model.UnlockResult{Status: model.StatusSentToCN, Region: "CN"}
	}
	return model.UnlockResult{Status: model.StatusUnlock, Region: region}
}

// checkNetflix 优先请求非自制剧页面（跟随重定向），页面含 "Oh no!" 即本区不可看：
// 非自制可看 → 立即返回全解锁，不再额外请求自制剧页；
// 非自制不可看时再请求自制剧页：仅自制可看 → 待解锁；均不可看 → 失败。
// 注意不能靠状态码判断：Netflix 对地区限制页也返回 200，特征在正文。
func checkNetflix(ctx context.Context, d model.Dialer) model.UnlockResult {
	c := newClient(d, true)
	nonOriginal, err := doGet(ctx, c, netflixNonOriginalID)
	if err != nil {
		return reqFailed("netflix")
	}
	nonAvail := bytes.Contains(nonOriginal.Body, []byte("Oh no!"))
	if !nonAvail {
		region := ""
		if m := netflixLocRe.FindSubmatch(nonOriginal.Body); m != nil {
			region = string(m[1])
		}
		return model.UnlockResult{Status: model.StatusUnlock, Region: region}
	}
	original, err := doGet(ctx, c, netflixOriginalID)
	if err != nil {
		return reqFailed("netflix")
	}
	origAvail := bytes.Contains(original.Body, []byte("Oh no!"))
	if !origAvail {
		return model.UnlockResult{Status: model.StatusPending}
	}
	return model.UnlockResult{Status: model.StatusFailed, Info: fmt.Sprintf("非自制/自制均不可看 (%d/%d)", nonOriginal.StatusCode, original.StatusCode)}
}

// checkDisneyPlus 默认走 Bamgrid 官方设备注册 + token 交换 + GraphQL refreshToken 链路，
// 精准核验 inSupportedLocation 与 countryCode；若测试仅替换了 disneyPlusURL，则兼容旧版首页检测。
func checkDisneyPlus(ctx context.Context, d model.Dialer) model.UnlockResult {
	if disneyPlusURL != defaultDisneyPlusURL &&
		disneyDevicesURL == defaultDisneyDevicesURL &&
		disneyTokenURL == defaultDisneyTokenURL &&
		disneyGraphQLURL == defaultDisneyGraphQLURL {
		return checkDisneyPlusLegacy(ctx, d)
	}

	c := newClient(d, true)
	devHeaders := map[string]string{
		"Authorization": "Bearer " + disneyAuthToken,
		"Content-Type":  "application/json",
	}

	devBody := `{"deviceFamily":"browser","applicationRuntime":"chrome","deviceProfile":"windows","attributes":{}}`
	devRes, err := doRequest(ctx, c, http.MethodPost, disneyDevicesURL, strings.NewReader(devBody), devHeaders)
	if err != nil {
		return reqFailed("disney_plus")
	}
	if devRes.StatusCode == 403 {
		return model.UnlockResult{Status: model.StatusFailed, Info: "地区限制"}
	}
	if devRes.StatusCode == 429 {
		return model.UnlockResult{Status: model.StatusFailed, Info: "HTTP 429"}
	}
	if devRes.StatusCode < 200 || devRes.StatusCode >= 300 {
		return model.UnlockResult{Status: model.StatusFailed, Info: fmt.Sprintf("devices HTTP %d", devRes.StatusCode)}
	}
	var devResp struct {
		Assertion string `json:"assertion"`
	}
	if err := json.Unmarshal(devRes.Body, &devResp); err != nil || devResp.Assertion == "" {
		return model.UnlockResult{Status: model.StatusFailed, Info: "devices 无 assertion"}
	}

	form := url.Values{
		"grant_type":         {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"latitude":           {"0"},
		"longitude":          {"0"},
		"platform":           {"browser"},
		"subject_token":      {devResp.Assertion},
		"subject_token_type": {"urn:bamtech:params:oauth:token-type:device"},
	}
	// /token 与 /graphql 端点要求裸 token（不带 Bearer 前缀）。
	tokRes, err := doRequest(ctx, c, http.MethodPost, disneyTokenURL, strings.NewReader(form.Encode()), map[string]string{
		"Authorization": disneyAuthToken,
		"Content-Type":  "application/x-www-form-urlencoded",
	})
	if err != nil {
		return reqFailed("disney_plus")
	}
	if tokRes.StatusCode == 403 || strings.Contains(strings.ToLower(string(tokRes.Body)), "forbidden-location") {
		return model.UnlockResult{Status: model.StatusFailed, Info: "地区限制"}
	}
	if tokRes.StatusCode == 429 {
		return model.UnlockResult{Status: model.StatusFailed, Info: "HTTP 429"}
	}
	if tokRes.StatusCode < 200 || tokRes.StatusCode >= 300 {
		return model.UnlockResult{Status: model.StatusFailed, Info: fmt.Sprintf("token HTTP %d", tokRes.StatusCode)}
	}
	var tokResp struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(tokRes.Body, &tokResp); err != nil || tokResp.RefreshToken == "" {
		return model.UnlockResult{Status: model.StatusFailed, Info: "token 无 refresh_token"}
	}

	gqlReq := struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}{
		Query: "mutation refreshToken($input:RefreshTokenInput!) { refreshToken(refreshToken:$input) { activeSession { sessionId } } }",
		Variables: map[string]any{
			"input": map[string]string{
				"refreshToken": tokResp.RefreshToken,
			},
		},
	}
	gqlPayload, err := json.Marshal(gqlReq)
	if err != nil {
		return reqFailed("disney_plus")
	}
	gqlRes, err := doRequest(ctx, c, http.MethodPost, disneyGraphQLURL, bytes.NewReader(gqlPayload), map[string]string{
		"Authorization": disneyAuthToken,
		"Content-Type":  "application/json",
	})
	if err != nil {
		return reqFailed("disney_plus")
	}
	if gqlRes.StatusCode == 403 || strings.Contains(strings.ToLower(string(gqlRes.Body)), "forbidden-location") {
		return model.UnlockResult{Status: model.StatusFailed, Info: "地区限制"}
	}
	if gqlRes.StatusCode == 429 {
		return model.UnlockResult{Status: model.StatusFailed, Info: "HTTP 429"}
	}
	if gqlRes.StatusCode < 200 || gqlRes.StatusCode >= 300 {
		return model.UnlockResult{Status: model.StatusFailed, Info: fmt.Sprintf("graphql HTTP %d", gqlRes.StatusCode)}
	}

	type disneySession struct {
		InSupportedLocation *bool `json:"inSupportedLocation"`
		Location            struct {
			CountryCode string `json:"countryCode"`
		} `json:"location"`
	}
	var gqlResp struct {
		Data struct {
			RefreshToken struct {
				ActiveSession disneySession `json:"activeSession"`
			} `json:"refreshToken"`
		} `json:"data"`
		Extensions struct {
			SDK struct {
				Session disneySession `json:"session"`
			} `json:"sdk"`
		} `json:"extensions"`
	}
	if err := json.Unmarshal(gqlRes.Body, &gqlResp); err != nil {
		return model.UnlockResult{Status: model.StatusFailed, Info: "graphql 响应非 JSON"}
	}

	region := strings.ToUpper(strings.TrimSpace(gqlResp.Extensions.SDK.Session.Location.CountryCode))
	if region == "" {
		region = strings.ToUpper(strings.TrimSpace(gqlResp.Data.RefreshToken.ActiveSession.Location.CountryCode))
	}
	if region == "" {
		if m := disneyCountryRe.FindSubmatch(gqlRes.Body); m != nil {
			region = strings.ToUpper(string(m[1]))
		}
	}

	inSup := gqlResp.Extensions.SDK.Session.InSupportedLocation
	if inSup == nil {
		inSup = gqlResp.Data.RefreshToken.ActiveSession.InSupportedLocation
	}
	var supported, hasSupported bool
	if inSup != nil {
		supported = *inSup
		hasSupported = true
	} else if m := disneyInSupRe.FindSubmatch(gqlRes.Body); m != nil {
		supported = string(m[1]) == "true"
		hasSupported = true
	}
	if !hasSupported {
		return model.UnlockResult{Status: model.StatusFailed, Region: region, Info: "无 session 支持信息"}
	}
	if !supported {
		return model.UnlockResult{Status: model.StatusFailed, Region: region, Info: "地区不支持"}
	}
	return model.UnlockResult{Status: model.StatusUnlock, Region: region}
}

func checkDisneyPlusLegacy(ctx context.Context, d model.Dialer) model.UnlockResult {
	r, err := doGet(ctx, newClient(d, false), disneyPlusURL)
	if err != nil {
		return reqFailed("disney_plus")
	}
	loc := strings.ToLower(r.Header.Get("Location"))
	if strings.Contains(loc, "blocked") || strings.Contains(loc, "unavailable") {
		return model.UnlockResult{Status: model.StatusFailed}
	}
	body := strings.ToLower(string(r.Body))
	if strings.Contains(body, "not available in your region") {
		return model.UnlockResult{Status: model.StatusFailed}
	}
	if r.StatusCode >= 400 {
		return model.UnlockResult{Status: model.StatusFailed}
	}
	return model.UnlockResult{Status: model.StatusUnlock}
}

// checkBilibili 请求港澳台限定番剧 playurl（带随机 session 防缓存），code==0 即解锁港澳台地区。
func checkBilibili(ctx context.Context, d model.Dialer) model.UnlockResult {
	u, err := url.Parse(bilibiliPlayURL)
	if err != nil {
		return reqFailed("bilibili")
	}
	q := u.Query()
	q.Set("session", randHex16())
	u.RawQuery = q.Encode()
	r, err := doGet(ctx, newClient(d, true), u.String())
	if err != nil {
		return reqFailed("bilibili")
	}
	if r.StatusCode != 200 {
		return model.UnlockResult{Status: model.StatusFailed}
	}
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(r.Body, &resp); err != nil {
		return model.UnlockResult{Status: model.StatusFailed, Info: "响应非 JSON"}
	}
	if resp.Code == 0 {
		return model.UnlockResult{Status: model.StatusUnlock, Region: "港澳台地区"}
	}
	return model.UnlockResult{Status: model.StatusFailed, Info: fmt.Sprintf("code:%d %s", resp.Code, resp.Message)}
}

func parseTikTokResponse(r *httpResult) (model.UnlockResult, bool) {
	finalLower := strings.ToLower(r.FinalURL)
	bodyLower := strings.ToLower(string(r.Body))
	if strings.Contains(finalLower, "/hk/notfound") || strings.Contains(bodyLower, "tiktok.com/hk/notfound") {
		return model.UnlockResult{Status: model.StatusFailed, Region: "HK", Info: "地区限制"}, true
	}
	if r.StatusCode == 200 {
		if m := tiktokRegRe.FindSubmatch(r.Body); m != nil {
			return model.UnlockResult{Status: model.StatusUnlock, Region: strings.ToUpper(string(m[1]))}, true
		}
	}
	return model.UnlockResult{}, false
}

// checkTikTok 优先请求 /explore 页面提取 region 字段并识别 /hk/notfound 封锁页；
// 若默认 /explore 页面未命中 region，则回退探测首页 https://www.tiktok.com/。
func checkTikTok(ctx context.Context, d model.Dialer) model.UnlockResult {
	c := newClient(d, true)
	r, err := doGet(ctx, c, tiktokAPIURL)
	if err != nil {
		return reqFailed("tiktok")
	}
	if res, ok := parseTikTokResponse(r); ok {
		return res
	}
	if tiktokAPIURL == defaultTikTokAPIURL || tiktokURL != defaultTikTokURL {
		if r2, err2 := doGet(ctx, c, tiktokURL); err2 == nil {
			if res, ok := parseTikTokResponse(r2); ok {
				return res
			}
			r = r2
		}
	}
	if r.StatusCode != 200 {
		return model.UnlockResult{Status: model.StatusFailed, Info: fmt.Sprintf("HTTP %d(%db)", r.StatusCode, len(r.Body))}
	}
	return model.UnlockResult{Status: model.StatusFailed, Info: fmt.Sprintf("页面无 region(%db)", len(r.Body))}
}

// checkDazn 通过启动接口返回的地区字段判断可访问地区。
func checkDazn(ctx context.Context, d model.Dialer) model.UnlockResult {
	payload := strings.NewReader(`{"LandingPageKey":"generic","Languages":"en","Platform":"web","Version":"2"}`)
	r, err := doRequest(ctx, newClient(d, true), "POST", daznStartupURL, payload,
		map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return reqFailed("dazn")
	}
	if r.StatusCode != 200 {
		return model.UnlockResult{Status: model.StatusFailed}
	}
	body := string(r.Body)
	if strings.Contains(body, `"isAllowed":false`) || strings.Contains(body, `"IsAllowed":false`) {
		return model.UnlockResult{Status: model.StatusFailed}
	}
	if m := daznRegionRe.FindStringSubmatch(body); m != nil {
		return model.UnlockResult{Status: model.StatusUnlock, Region: m[1]}
	}
	return model.UnlockResult{Status: model.StatusUnlock}
}

// checkAbema 优先通过 Abema 官方 IP 检测接口识别日本原生解锁与机房/海外拦截；
// 若测试仅替换了 abemaCheckURL，则兼容旧版首页检测行为。
func checkAbema(ctx context.Context, d model.Dialer) model.UnlockResult {
	c := newClient(d, true)
	if abemaCheckURL != defaultAbemaCheckURL && abemaAPIURL == defaultAbemaAPIURL {
		r, err := doGet(ctx, c, abemaCheckURL)
		if err != nil {
			return reqFailed("abema")
		}
		if r.StatusCode == 200 {
			return model.UnlockResult{Status: model.StatusUnlock, Region: "JP"}
		}
		return model.UnlockResult{Status: model.StatusFailed, Info: fmt.Sprintf("HTTP %d", r.StatusCode)}
	}

	r, err := doRequest(ctx, c, http.MethodGet, abemaAPIURL, nil, map[string]string{
		"User-Agent": "Dalvik/2.1.0 (Linux; U; Android 13; Pixel 7)",
	})
	if err != nil {
		return reqFailed("abema")
	}
	var resp struct {
		ISOCountryCode string `json:"isoCountryCode"`
		Message        string `json:"message"`
	}
	_ = json.Unmarshal(r.Body, &resp)

	if cc := strings.ToUpper(strings.TrimSpace(resp.ISOCountryCode)); cc != "" {
		if cc == "JP" {
			return model.UnlockResult{Status: model.StatusUnlock, Region: "JP"}
		}
		return model.UnlockResult{Status: model.StatusFailed, Region: cc, Info: "OverseaOnly"}
	}
	if resp.Message != "" {
		return model.UnlockResult{Status: model.StatusFailed, Info: fmt.Sprintf("HTTP %d (%s)", r.StatusCode, resp.Message)}
	}
	return model.UnlockResult{Status: model.StatusFailed, Info: fmt.Sprintf("HTTP %d", r.StatusCode)}
}

// checkBahamut 动画疯需先取 deviceid（带 cookie）再请求 token 接口，返回 animeSn 即解锁。
// 地区取自首页 data-geo 属性（TW/HK/MO）。
func checkBahamut(ctx context.Context, d model.Dialer) model.UnlockResult {
	c := newClient(d, true)
	c.Jar, _ = cookiejar.New(nil)

	dev, err := doGet(ctx, c, bahamutDeviceURL)
	if err != nil {
		return reqFailed("bahamut")
	}
	var devResp struct {
		DeviceID string `json:"deviceid"`
	}
	if err := json.Unmarshal(dev.Body, &devResp); err != nil || devResp.DeviceID == "" {
		return model.UnlockResult{Status: model.StatusFailed, Info: "无法获取 deviceid"}
	}

	token, err := doGet(ctx, c, bahamutTokenURL+"&device="+devResp.DeviceID)
	if err != nil {
		return reqFailed("bahamut")
	}
	if !bytes.Contains(token.Body, []byte("animeSn")) {
		return model.UnlockResult{Status: model.StatusFailed, Info: fmt.Sprintf("token 无 animeSn (%d)", token.StatusCode)}
	}

	region := "TW"
	if home, err := doGet(ctx, c, bahamutHomeURL); err == nil {
		if m := bahamutGeoRe.FindSubmatch(home.Body); m != nil {
			region = string(m[1])
		}
	}
	return model.UnlockResult{Status: model.StatusUnlock, Region: region}
}

// checkSteam 商店页面 HTML 的 priceCurrency 即账户钱包货币（随 IP 地区）。
func checkSteam(ctx context.Context, d model.Dialer) model.UnlockResult {
	r, err := doGet(ctx, newClient(d, true), steamAppURL)
	if err != nil {
		return reqFailed("steam")
	}
	if r.StatusCode != 200 {
		return model.UnlockResult{Status: model.StatusFailed}
	}
	if m := steamCurrRe.FindSubmatch(r.Body); m != nil {
		return model.UnlockResult{Status: model.StatusUnlock, Info: "货币：" + string(m[1])}
	}
	return model.UnlockResult{Status: model.StatusUnlock}
}

// checkSpotify 注册页含地区限制提示即不可注册。
func checkSpotify(ctx context.Context, d model.Dialer) model.UnlockResult {
	r, err := doGet(ctx, newClient(d, true), spotifySignupURL)
	if err != nil {
		return reqFailed("spotify")
	}
	if r.StatusCode != 200 {
		return model.UnlockResult{Status: model.StatusFailed}
	}
	body := strings.ToLower(string(r.Body))
	if strings.Contains(body, "not available in your country") || strings.Contains(body, "spotify is currently not available") {
		return model.UnlockResult{Status: model.StatusFailed}
	}
	return model.UnlockResult{Status: model.StatusUnlock}
}
