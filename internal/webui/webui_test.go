package webui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nekospeed/internal/geo"
	"nekospeed/internal/model"
	"nekospeed/internal/report"
)

func TestWebUIAuthAndStatus(t *testing.T) {
	t.Chdir(t.TempDir())
	s := &server{hub: newHub(), token: "secret-123"}
	h := s.handler()

	// 无令牌访问 API 应返回 401
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("无令牌 /api/status 状态码 = %d, 期望 401", w.Code)
	}

	// 访问首页带 ?token= 应下发 HttpOnly Cookie
	reqIdx := httptest.NewRequest(http.MethodGet, "/?token=secret-123", nil)
	wIdx := httptest.NewRecorder()
	h.ServeHTTP(wIdx, reqIdx)
	if wIdx.Code != http.StatusOK {
		t.Fatalf("/?token= 状态码 = %d", wIdx.Code)
	}
	cookies := wIdx.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != tokenCookieName {
		t.Fatalf("期望下发 %s cookie, 实际 %+v", tokenCookieName, cookies)
	}

	// 携带 Bearer Header 或 Cookie 应正常返回 200
	reqBearer := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	reqBearer.Header.Set("Authorization", "Bearer secret-123")
	wBearer := httptest.NewRecorder()
	h.ServeHTTP(wBearer, reqBearer)
	if wBearer.Code != http.StatusOK {
		t.Fatalf("Bearer 鉴权状态码 = %d", wBearer.Code)
	}

	reqCookie := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	reqCookie.AddCookie(cookies[0])
	wCookie := httptest.NewRecorder()
	h.ServeHTTP(wCookie, reqCookie)
	if wCookie.Code != http.StatusOK {
		t.Fatalf("Cookie 鉴权状态码 = %d", wCookie.Code)
	}
}

func TestPreviewInlineDedupAndOversizedBody(t *testing.T) {
	t.Chdir(t.TempDir())
	s := &server{hub: newHub()}
	h := s.handler()

	payload := `{"content":"ss://YWVzLTI1Ni1nY206cGFzcw==@1.1.1.1:8388#HK-Test","filename":"my_nodes.txt"}`
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/preview", strings.NewReader(payload))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("preview #%d code = %d, body = %s", i+1, w.Code, w.Body.String())
		}
		var resp struct {
			Source       string `json:"source"`
			SubName      string `json:"subName"`
			Total        int    `json:"total"`
			MatchedCount int    `json:"matchedCount"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if resp.SubName != "my_nodes" || resp.Total != 1 || resp.MatchedCount != 1 {
			t.Fatalf("unexpected preview resp: %+v", resp)
		}
	}

	entries, err := os.ReadDir(filepath.Join("out", "uploads"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("相同内联内容重复预览应去重为 1 个文件, 实际 %d 个", len(entries))
	}

	// 非法正则应返回 400
	badReReq := httptest.NewRequest(http.MethodPost, "/api/preview", strings.NewReader(`{"content":"ss://YWVzLTI1Ni1nY206cGFzcw==@1.1.1.1:8388#A","filter":"[invalid"}`))
	wBad := httptest.NewRecorder()
	h.ServeHTTP(wBad, badReReq)
	if wBad.Code != http.StatusBadRequest {
		t.Fatalf("非法正则期望 400, 实际 %d", wBad.Code)
	}

	// 非法代理协议应返回 400（且使用 LoadContext 不污染全局代理状态）
	badProxyReq := httptest.NewRequest(http.MethodPost, "/api/preview", strings.NewReader(`{"content":"ss://YWVzLTI1Ni1nY206cGFzcw==@1.1.1.1:8388#A","proxy":"ftp://127.0.0.1:21"}`))
	wBadProxy := httptest.NewRecorder()
	h.ServeHTTP(wBadProxy, badProxyReq)
	if wBadProxy.Code != http.StatusBadRequest {
		t.Fatalf("非法代理协议期望 400, 实际 %d", wBadProxy.Code)
	}

	// 超限请求体应返回 413
	huge := append([]byte(`{"content":"`), bytes.Repeat([]byte("a"), maxUploadJSONBytes)...)
	huge = append(huge, []byte(`"}`)...)
	reqHuge := httptest.NewRequest(http.MethodPost, "/api/preview", bytes.NewReader(huge))
	wHuge := httptest.NewRecorder()
	h.ServeHTTP(wHuge, reqHuge)
	if wHuge.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("超限请求体期望 413, 实际 %d", wHuge.Code)
	}
}

func TestHandleReRender(t *testing.T) {
	t.Chdir(t.TempDir())
	runDir := filepath.Join("out", "web", "20261113_230000")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	results := []*model.NodeResult{
		{
			Node:     model.Node{Index: 0, Name: "JP-1", Type: "ss", Server: "1.1.1.1"},
			Outbound: &model.GeoInfo{IP: "203.0.113.5", Country: "JP", Stack: "4"},
			Speed:    &model.SpeedResult{Avg: 12_500_000, Max: 15_000_000, Threads: 4, PerSecond: []float64{12_500_000}},
		},
	}
	topo := geo.BuildTopology(results)
	err := report.SaveJSONWithMeta(results, topo, report.RunMeta{
		Version:   "0.1.0",
		StartTime: time.Now(),
		Elapsed:   time.Second,
		SubName:   "Demo",
		Threads:   4,
		PingCount: 4,
	}, filepath.Join(runDir, "Demo_20261113_230000_result.json"))
	if err != nil {
		t.Fatal(err)
	}

	s := &server{hub: newHub()}
	h := s.handler()
	req := httptest.NewRequest(http.MethodPost, "/api/rerender", strings.NewReader(`{"runId":"20261113_230000","theme":"midnight","sortBy":"speed"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("rerender code = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestHubHistoryCappedAndSafeFilename(t *testing.T) {
	h := newHub()
	for i := 0; i < maxHubHistory+200; i++ {
		h.broadcast("data: x\n\n")
	}
	if len(h.history) > maxHubHistory {
		t.Fatalf("hub.history len = %d, 超过上限 %d", len(h.history), maxHubHistory)
	}
	if !isSafeFilename("abc_123.png") || isSafeFilename("../x") || isSafeFilename(".hidden") || isSafeFilename("a/b") {
		t.Fatal("isSafeFilename 校验逻辑异常")
	}
}

func TestStaticAssetsServed(t *testing.T) {
	t.Chdir(t.TempDir())
	s := &server{hub: newHub(), token: "secret-123"}
	h := s.handler()

	for _, tc := range []struct {
		path string
		want string
	}{
		{"/", "/static/app.css"},
		{"/static/app.css", "--primary"},
		{"/static/app.js", "nekospeed_webui_prefs_v3"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s code = %d, want 200", tc.path, w.Code)
		}
		if !strings.Contains(w.Body.String(), tc.want) {
			t.Fatalf("GET %s body missing %q", tc.path, tc.want)
		}
	}
}

func TestGeoIPStatusAndUpdateConflict(t *testing.T) {
	t.Chdir(t.TempDir())
	s := &server{hub: newHub()}
	h := s.handler()

	reqGeo := httptest.NewRequest(http.MethodGet, "/api/geoip/status", nil)
	wGeo := httptest.NewRecorder()
	h.ServeHTTP(wGeo, reqGeo)
	if wGeo.Code != http.StatusOK {
		t.Fatalf("GET /api/geoip/status code = %d, body = %s", wGeo.Code, wGeo.Body.String())
	}
	var geoResp struct {
		Mode     string `json:"mode"`
		Updating bool   `json:"updating"`
		Files    []any  `json:"files"`
	}
	if err := json.Unmarshal(wGeo.Body.Bytes(), &geoResp); err != nil {
		t.Fatal(err)
	}
	if geoResp.Mode != "online" || geoResp.Updating || len(geoResp.Files) == 0 {
		t.Fatalf("unexpected /api/geoip/status payload: %+v", geoResp)
	}

	reqSt := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	wSt := httptest.NewRecorder()
	h.ServeHTTP(wSt, reqSt)
	if wSt.Code != http.StatusOK || !strings.Contains(wSt.Body.String(), `"geoip"`) {
		t.Fatalf("GET /api/status missing geoip field: code=%d body=%s", wSt.Code, wSt.Body.String())
	}

	s.mu.Lock()
	s.running = true
	s.mu.Unlock()
	reqUpd := httptest.NewRequest(http.MethodPost, "/api/geoip/update", strings.NewReader(`{}`))
	wUpd := httptest.NewRecorder()
	h.ServeHTTP(wUpd, reqUpd)
	if wUpd.Code != http.StatusConflict {
		t.Fatalf("运行中触发 /api/geoip/update 期望 409, 实际 %d", wUpd.Code)
	}
}

func TestOutBaseDir(t *testing.T) {
	t.Chdir(t.TempDir())
	portableDir := filepath.Join(filepath.VolumeName(os.TempDir())+string(filepath.Separator), "Apps", "NekoSpeed")
	portableExe := filepath.Join(portableDir, "nekospeed.exe")

	if got := resolveOutBaseDir(portableExe, false); got != filepath.Join(portableDir, "out") {
		t.Fatalf("无 CWD out 时便携 exe 期望回退至 %q, 实际 %q", filepath.Join(portableDir, "out"), got)
	}
	if got := resolveOutBaseDir(portableExe, true); got != "out" {
		t.Fatalf("测试模式下期望返回 out, 实际 %q", got)
	}
	tempBuildExe := filepath.Join(os.TempDir(), "go-build12345", "b001", "exe", "nekospeed.exe")
	if got := resolveOutBaseDir(tempBuildExe, false); got != "out" {
		t.Fatalf("临时 go-build 可执行文件期望返回 out, 实际 %q", got)
	}

	if err := os.MkdirAll("out", 0o755); err != nil {
		t.Fatal(err)
	}
	if got := resolveOutBaseDir(portableExe, false); got != "out" {
		t.Fatalf("CWD 存在 out 目录时期望返回 out, 实际 %q", got)
	}
}

func TestCSRFProtectionAndContentType(t *testing.T) {
	t.Chdir(t.TempDir())
	s := &server{hub: newHub()} // 即使未设置 token 也必须启用跨站与 Content-Type 防护
	h := s.handler()

	// 1. Sec-Fetch-Site: cross-site 拒绝
	reqCross := httptest.NewRequest(http.MethodPost, "/api/preview", strings.NewReader(`{}`))
	reqCross.Header.Set("Sec-Fetch-Site", "cross-site")
	reqCross.Header.Set("Content-Type", "application/json")
	wCross := httptest.NewRecorder()
	h.ServeHTTP(wCross, reqCross)
	if wCross.Code != http.StatusForbidden {
		t.Fatalf("Sec-Fetch-Site: cross-site 期望 403, 实际 %d", wCross.Code)
	}

	// 2. Origin 主机不匹配或 null 拒绝
	reqBadOrigin := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:18189/api/preview", strings.NewReader(`{}`))
	reqBadOrigin.Host = "127.0.0.1:18189"
	reqBadOrigin.Header.Set("Origin", "https://evil.example")
	reqBadOrigin.Header.Set("Content-Type", "application/json")
	wBadOrigin := httptest.NewRecorder()
	h.ServeHTTP(wBadOrigin, reqBadOrigin)
	if wBadOrigin.Code != http.StatusForbidden {
		t.Fatalf("跨源 Origin 期望 403, 实际 %d", wBadOrigin.Code)
	}

	reqNullOrigin := httptest.NewRequest(http.MethodDelete, "http://127.0.0.1:18189/api/history/20261113_230000", nil)
	reqNullOrigin.Host = "127.0.0.1:18189"
	reqNullOrigin.Header.Set("Origin", "null")
	wNullOrigin := httptest.NewRecorder()
	h.ServeHTTP(wNullOrigin, reqNullOrigin)
	if wNullOrigin.Code != http.StatusForbidden {
		t.Fatalf("Origin: null 期望 403, 实际 %d", wNullOrigin.Code)
	}

	// 3. JSON 接口拒绝 simple-CORS Content-Type (415)
	for _, endpoint := range []string{"/api/geoip/update", "/api/preview", "/api/run", "/api/rerender"} {
		for _, ct := range []string{"text/plain", "text/plain; charset=utf-8", "application/x-www-form-urlencoded", "multipart/form-data; boundary=abc"} {
			reqCT := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{}`))
			reqCT.Header.Set("Content-Type", ct)
			wCT := httptest.NewRecorder()
			h.ServeHTTP(wCT, reqCT)
			if wCT.Code != http.StatusUnsupportedMediaType {
				t.Fatalf("%s with Content-Type %q 期望 415, 实际 %d", endpoint, ct, wCT.Code)
			}
		}
	}

	// 4. 合法同源 application/json 请求放行
	payload := `{"content":"ss://YWVzLTI1Ni1nY206cGFzcw==@1.1.1.1:8388#HK-Test"}`
	reqOK := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:18189/api/preview", strings.NewReader(payload))
	reqOK.Host = "127.0.0.1:18189"
	reqOK.Header.Set("Origin", "http://127.0.0.1:18189")
	reqOK.Header.Set("Sec-Fetch-Site", "same-origin")
	reqOK.Header.Set("Content-Type", "application/json; charset=utf-8")
	wOK := httptest.NewRecorder()
	h.ServeHTTP(wOK, reqOK)
	if wOK.Code != http.StatusOK {
		t.Fatalf("同源 JSON 请求期望 200, 实际 %d (%s)", wOK.Code, wOK.Body.String())
	}
}

func TestHandleReRenderConflictWhileRunning(t *testing.T) {
	t.Chdir(t.TempDir())
	doneDir := filepath.Join("out", "web", "20261113_220000")
	if err := os.MkdirAll(doneDir, 0o755); err != nil {
		t.Fatal(err)
	}
	results := []*model.NodeResult{
		{
			Node:  model.Node{Index: 1, Name: "JP-1", Type: "ss", Server: "1.1.1.1"},
			Speed: &model.SpeedResult{Avg: 10_000_000, Max: 12_000_000, Threads: 4, PerSecond: []float64{10_000_000}},
		},
	}
	if err := report.SaveJSONWithMeta(results, nil, report.RunMeta{
		Version: "0.1.0", StartTime: time.Now(), Elapsed: time.Second, SubName: "DoneRun",
	}, filepath.Join(doneDir, "DoneRun_20261113_220000_result.json")); err != nil {
		t.Fatal(err)
	}

	s := &server{
		hub:     newHub(),
		running: true,
		runID:   "20261113_230000",
		outDir:  filepath.Join("out", "web", "20261113_230000"),
	}
	h := s.handler()

	for _, body := range []string{`{"runId":"20261113_230000"}`, `{}`} {
		req := httptest.NewRequest(http.MethodPost, "/api/rerender", strings.NewReader(body))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("运行中重绘 %s 期望 409, 实际 %d", body, w.Code)
		}
	}

	// 重绘另一条已完成的历史记录允许执行
	reqOther := httptest.NewRequest(http.MethodPost, "/api/rerender", strings.NewReader(`{"runId":"20261113_220000","theme":"default"}`))
	wOther := httptest.NewRecorder()
	h.ServeHTTP(wOther, reqOther)
	if wOther.Code != http.StatusOK {
		t.Fatalf("运行中重绘其他已完成记录期望 200, 实际 %d (%s)", wOther.Code, wOther.Body.String())
	}
}

func TestHistoryCacheAndInvalidation(t *testing.T) {
	t.Chdir(t.TempDir())
	runID := "20261113_230000"
	runDir := filepath.Join("out", "web", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	jsonPath := filepath.Join(runDir, "Demo_20261113_230000_result.json")
	results := []*model.NodeResult{{Node: model.Node{Index: 1, Name: "HK-1", Type: "ss", Server: "1.1.1.1"}, Speed: &model.SpeedResult{Avg: 1000}}}
	if err := report.SaveJSONWithMeta(results, nil, report.RunMeta{
		Version: "0.1.0", StartTime: time.Now(), Elapsed: 2 * time.Second, SubName: "Demo",
	}, jsonPath); err != nil {
		t.Fatal(err)
	}

	s := &server{hub: newHub()}
	h := s.handler()

	// 第一次请求：未命中缓存
	w1 := httptest.NewRecorder()
	h.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/api/history", nil))
	if w1.Code != http.StatusOK {
		t.Fatalf("GET /api/history #1 code = %d", w1.Code)
	}
	if s.histCacheMisses != 1 || s.histCacheHits != 0 {
		t.Fatalf("期望 miss=1 hit=0, 实际 miss=%d hit=%d", s.histCacheMisses, s.histCacheHits)
	}

	// 第二次请求：命中缓存
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/api/history", nil))
	if w2.Code != http.StatusOK {
		t.Fatalf("GET /api/history #2 code = %d", w2.Code)
	}
	if s.histCacheMisses != 1 || s.histCacheHits != 1 {
		t.Fatalf("期望 miss=1 hit=1, 实际 miss=%d hit=%d", s.histCacheMisses, s.histCacheHits)
	}

	// 文件变化后缓存失效重新读取
	if err := report.SaveJSONWithMeta(results, nil, report.RunMeta{
		Version: "0.1.0", StartTime: time.Now(), Elapsed: 3 * time.Second, SubName: "DemoUpdated",
	}, jsonPath); err != nil {
		t.Fatal(err)
	}
	nowPlus := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(jsonPath, nowPlus, nowPlus)

	w3 := httptest.NewRecorder()
	h.ServeHTTP(w3, httptest.NewRequest(http.MethodGet, "/api/history", nil))
	if w3.Code != http.StatusOK || !strings.Contains(w3.Body.String(), "DemoUpdated") {
		t.Fatalf("文件更新后期望读到 DemoUpdated, body=%s", w3.Body.String())
	}
	if s.histCacheMisses != 2 {
		t.Fatalf("文件更新后期望 miss=2, 实际 %d", s.histCacheMisses)
	}

	// 删除记录应清理缓存
	wDel := httptest.NewRecorder()
	h.ServeHTTP(wDel, httptest.NewRequest(http.MethodDelete, "/api/history/"+runID, nil))
	if wDel.Code != http.StatusOK {
		t.Fatalf("DELETE /api/history code = %d", wDel.Code)
	}
	s.histMu.Lock()
	_, exists := s.histCache[runID]
	s.histMu.Unlock()
	if exists {
		t.Fatal("删除历史记录后缓存仍残留")
	}
}
