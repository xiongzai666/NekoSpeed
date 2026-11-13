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
