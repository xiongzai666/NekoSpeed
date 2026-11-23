package unlock

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"nekospeed/internal/model"
)

// directDialer 直连拨号器（不经过代理），供测试使用。
func directDialer() model.Dialer {
	nd := &net.Dialer{Timeout: 5 * time.Second}
	return model.DialFunc(nd.DialContext)
}

// swapURL 临时替换包内 URL 变量，测试结束后恢复。
func swapURL(t *testing.T, p *string, v string) {
	t.Helper()
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

func TestYouTubeRegionUnlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`<html><script>var ytcfg={"gl":"JP","hl":"en"};</script></html>`))
	}))
	defer srv.Close()
	swapURL(t, &youtubeURL, srv.URL)

	res := checkYouTube(context.Background(), directDialer())
	if res.Status != model.StatusUnlock || res.Region != "JP" {
		t.Fatalf("期望解锁(JP)，得到 %+v", res)
	}
}

func TestYouTubeSentToCN(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"地区为CN", `{"gl":"CN"}`},
		{"无地区码", `<html>no region info</html>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			swapURL(t, &youtubeURL, srv.URL)

			res := checkYouTube(context.Background(), directDialer())
			if res.Status != model.StatusSentToCN || res.Region != "CN" {
				t.Fatalf("期望送中(CN)，得到 %+v", res)
			}
		})
	}
}

func TestNetflixFullUnlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`<html>{"requestCountry":{"id":"US","countryName":"United States"}}</html>`))
	}))
	defer srv.Close()
	swapURL(t, &netflixNonOriginalID, srv.URL+"/title/81280792")
	swapURL(t, &netflixOriginalID, srv.URL+"/title/80057281")

	res := checkNetflix(context.Background(), directDialer())
	if res.Status != model.StatusUnlock || res.Region != "US" {
		t.Fatalf("期望解锁(US)，得到 %+v", res)
	}
}

func TestNetflixOriginalOnly(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/title/81280792", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><h1>Oh no!</h1>This title currently isn't available to watch in your country.</html>`))
	})
	mux.HandleFunc("/title/80057281", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>Stranger Things</html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	swapURL(t, &netflixNonOriginalID, srv.URL+"/title/81280792")
	swapURL(t, &netflixOriginalID, srv.URL+"/title/80057281")

	res := checkNetflix(context.Background(), directDialer())
	if res.Status != model.StatusPending {
		t.Fatalf("期望待解锁，得到 %+v", res)
	}
}

func TestNetflixBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><h1>Oh no!</h1></html>`))
	}))
	defer srv.Close()
	swapURL(t, &netflixNonOriginalID, srv.URL+"/title/81280792")
	swapURL(t, &netflixOriginalID, srv.URL+"/title/80057281")

	res := checkNetflix(context.Background(), directDialer())
	if res.Status != model.StatusFailed {
		t.Fatalf("期望失败，得到 %+v", res)
	}
}

func TestBahamutUnlock(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ajax/getdeviceid.php", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Set-Cookie", "BAHARUNE=abc123")
		w.Write([]byte(`{"deviceid":"dev-xyz","msg":"success"}`))
	})
	mux.HandleFunc("/ajax/token.php", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("device") != "dev-xyz" {
			w.Write([]byte(`{"error":"device required"}`))
			return
		}
		w.Write([]byte(`{"animeSn":37783,"token":"x"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body data-geo="TW"></body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	swapURL(t, &bahamutDeviceURL, srv.URL+"/ajax/getdeviceid.php")
	swapURL(t, &bahamutTokenURL, srv.URL+"/ajax/token.php?adID=89422&sn=37783")
	swapURL(t, &bahamutHomeURL, srv.URL+"/")

	res := checkBahamut(context.Background(), directDialer())
	if res.Status != model.StatusUnlock || res.Region != "TW" {
		t.Fatalf("期望解锁(TW)，得到 %+v", res)
	}
}

func TestBahamutRegionBlocked(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ajax/getdeviceid.php", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"deviceid":"dev-xyz"}`))
	})
	mux.HandleFunc("/ajax/token.php", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"error":"region blocked"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	swapURL(t, &bahamutDeviceURL, srv.URL+"/ajax/getdeviceid.php")
	swapURL(t, &bahamutTokenURL, srv.URL+"/ajax/token.php?adID=89422&sn=37783")
	swapURL(t, &bahamutHomeURL, srv.URL+"/")

	res := checkBahamut(context.Background(), directDialer())
	if res.Status != model.StatusFailed {
		t.Fatalf("期望失败，得到 %+v", res)
	}
}

func TestTikTokUnlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><script>{"appContext":{"region":"jp"}}</script></html>`))
	}))
	defer srv.Close()
	swapURL(t, &tiktokAPIURL, srv.URL)

	res := checkTikTok(context.Background(), directDialer())
	if res.Status != model.StatusUnlock || res.Region != "JP" {
		t.Fatalf("期望解锁(JP)，得到 %+v", res)
	}
}

func TestTikTokVerifyPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><div id="tiktok-verify-page"></div></html>`))
	}))
	defer srv.Close()
	swapURL(t, &tiktokAPIURL, srv.URL)

	res := checkTikTok(context.Background(), directDialer())
	if res.Status != model.StatusFailed {
		t.Fatalf("期望失败，得到 %+v", res)
	}
}

// openAIMock 同时 mock trace/compliance/ios 三个端点。
func openAIMock(t *testing.T, traceBody, compBody string, compCode int, iosBody string, iosCode int) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/trace", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(traceBody))
	})
	mux.HandleFunc("/compliance", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(compCode)
		w.Write([]byte(compBody))
	})
	mux.HandleFunc("/ios", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(iosCode)
		w.Write([]byte(iosBody))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	swapURL(t, &openAITraceURL, srv.URL+"/trace")
	swapURL(t, &openAIComplianceURL, srv.URL+"/compliance")
	swapURL(t, &openAIIOSURL, srv.URL+"/ios")
}

func TestOpenAIUnlockDespiteDCBlock(t *testing.T) {
	// 回归：ios 站点对数据中心/非浏览器客户端固定 403（cf_details dc 拦截），
	// 不含地区/VPN 关键词，不得误判为地区封锁。
	openAIMock(t,
		"fl=1f0\nh=chat.openai.com\nloc=US\nts=1.0\n",
		`{"cookie_requirements":{"required":false}}`, 200,
		`{"cf_details":"Request is not allowed. Please try again later.", "type":"dc"}`, 403)

	res := checkOpenAI(context.Background(), directDialer())
	if res.Status != model.StatusUnlock || res.Region != "US" {
		t.Fatalf("DC 403 不应误判，期望解锁(US)，得到 %+v", res)
	}
}

func TestOpenAIUnsupportedCountry(t *testing.T) {
	openAIMock(t,
		"loc=HK\n",
		`{"unsupported_country_name_list":["Hong Kong"]}`, 200,
		"", 403)

	res := checkOpenAI(context.Background(), directDialer())
	if res.Status != model.StatusFailed || res.Region != "HK" {
		t.Fatalf("期望地区封锁(HK)，得到 %+v", res)
	}
}

func TestOpenAIVPNDetected(t *testing.T) {
	openAIMock(t,
		"loc=JP\n",
		`{"cookie_requirements":{"required":false}}`, 200,
		`<html>You seem to be using a VPN or proxy.</html>`, 403)

	res := checkOpenAI(context.Background(), directDialer())
	if res.Status != model.StatusFailed || res.Region != "JP" {
		t.Fatalf("期望代理识别(JP)，得到 %+v", res)
	}
}

func TestOpenAIComplianceErrorMapsToReqFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("loc=US\n"))
	}))
	swapURL(t, &openAITraceURL, srv.URL)
	swapURL(t, &openAIComplianceURL, srv.URL) // trace 请求后关闭，compliance 将连接失败
	swapURL(t, &openAIIOSURL, srv.URL)

	done := make(chan model.UnlockResult, 1)
	go func() { done <- checkOpenAI(context.Background(), directDialer()) }()
	srv.Close()
	res := <-done
	if res.Status != model.StatusReqFailed {
		t.Fatalf("期望请求失败，得到 %+v", res)
	}
}

func TestBilibiliUnlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("session") == "" {
			t.Error("缺少随机 session 参数")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":0,"message":"success","result":{}}`))
	}))
	defer srv.Close()
	swapURL(t, &bilibiliPlayURL, srv.URL)

	res := checkBilibili(context.Background(), directDialer())
	if res.Status != model.StatusUnlock || res.Region != "港澳台地区" {
		t.Fatalf("期望解锁(港澳台地区)，得到 %+v", res)
	}
}

func TestBilibiliFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":-10403,"message":"抱歉您所在地区不可观看"}`))
	}))
	defer srv.Close()
	swapURL(t, &bilibiliPlayURL, srv.URL)

	res := checkBilibili(context.Background(), directDialer())
	if res.Status != model.StatusFailed {
		t.Fatalf("期望失败，得到 %+v", res)
	}
}

func TestRequestErrorMapsToReqFailed(t *testing.T) {
	// 指向一个不可达的本地端口。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // 立即关闭，连接将被拒绝
	swapURL(t, &bilibiliPlayURL, srv.URL)

	res := checkBilibili(context.Background(), directDialer())
	if res.Status != model.StatusReqFailed {
		t.Fatalf("期望请求失败，得到 %+v", res)
	}
}

func TestAbemaUnlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`<html>ABEMA</html>`))
	}))
	defer srv.Close()
	swapURL(t, &abemaCheckURL, srv.URL)
	res := checkAbema(context.Background(), directDialer())
	if res.Status != model.StatusUnlock || res.Region != "JP" {
		t.Fatalf("期望解锁(JP)，得到 %+v", res)
	}
}

func TestAbemaRegionBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`<html>blocked</html>`))
	}))
	defer srv.Close()
	swapURL(t, &abemaCheckURL, srv.URL)
	res := checkAbema(context.Background(), directDialer())
	if res.Status != model.StatusFailed {
		t.Fatalf("期望失败，得到 %+v", res)
	}
}

func TestAbemaHTTP503(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		w.Write([]byte(`<html>Service Unavailable</html>`))
	}))
	defer srv.Close()
	swapURL(t, &abemaCheckURL, srv.URL)
	res := checkAbema(context.Background(), directDialer())
	if res.Status != model.StatusFailed {
		t.Fatalf("期望失败，得到 %+v", res)
	}
}

func TestCheckerPanicRecovered(t *testing.T) {
	c := checkerFunc{"panic_test", func(ctx context.Context, d model.Dialer) model.UnlockResult {
		panic("boom")
	}}
	res := c.Check(context.Background(), directDialer())
	if res.Status != model.StatusReqFailed || res.Platform != "panic_test" {
		t.Fatalf("panic 应被 recover 为 ReqFailed，得到 %+v", res)
	}
}

func TestRunMediaAndAIOrder(t *testing.T) {
	// 不依赖外网：全部请求失败也应按固定顺序返回完整结果。
	media := RunMedia(context.Background(), directDialer())
	if len(media) != len(mediaCheckers) {
		t.Fatalf("媒体结果数量不符：%d != %d", len(media), len(mediaCheckers))
	}
	for i, c := range mediaCheckers {
		if media[i].Platform != c.Platform() {
			t.Fatalf("媒体结果顺序不符：第 %d 项 %s != %s", i, media[i].Platform, c.Platform())
		}
	}

	ai := RunAI(context.Background(), directDialer())
	if len(ai) != len(aiCheckers) {
		t.Fatalf("AI 结果数量不符：%d != %d", len(ai), len(aiCheckers))
	}
	for i, c := range aiCheckers {
		if ai[i].Platform != c.Platform() {
			t.Fatalf("AI 结果顺序不符：第 %d 项 %s != %s", i, ai[i].Platform, c.Platform())
		}
	}
}

// aiCloudflareMock 同时 mock trace（cdn-cgi/trace）和首页。
func aiCloudflareMock(t *testing.T, homeBody string, homeCode int, traceBody string) (traceURL, homeURL string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/trace", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(traceBody))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(homeCode)
		w.Write([]byte(homeBody))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL + "/trace", srv.URL + "/"
}

func TestClaudeUnlock(t *testing.T) {
	trace, home := aiCloudflareMock(t, `<html>Claude</html>`, 200, "loc=US\n")
	swapURL(t, &claudeTraceURL, trace)
	swapURL(t, &claudeURL, home)
	res := checkClaude(context.Background(), directDialer())
	if res.Status != model.StatusUnlock || res.Region != "US" {
		t.Fatalf("期望解锁(US)，得到 %+v", res)
	}
}

func TestClaudeRegionBlocked(t *testing.T) {
	trace, home := aiCloudflareMock(t, `<html>not available in your country</html>`, 200, "loc=CN\n")
	swapURL(t, &claudeTraceURL, trace)
	swapURL(t, &claudeURL, home)
	res := checkClaude(context.Background(), directDialer())
	if res.Status != model.StatusFailed {
		t.Fatalf("期望失败，得到 %+v", res)
	}
}

func TestClaudeCF403NotRegionBlock(t *testing.T) {
	// Cloudflare 403（cf_details dc 拦截）不含"not available"，应返回 failed 而非 unlock
	trace, home := aiCloudflareMock(t, `{"cf_details":"Request is not allowed."}`, 403, "loc=US\n")
	swapURL(t, &claudeTraceURL, trace)
	swapURL(t, &claudeURL, home)
	res := checkClaude(context.Background(), directDialer())
	if res.Status != model.StatusFailed {
		t.Fatalf("期望失败，得到 %+v", res)
	}
}

// apiTraceMock 同时 mock API 端点（任意路径）与 /trace 端点。
func apiTraceMock(t *testing.T, apiPath string, apiCode int, apiBody, traceBody string) (apiURL, traceURL string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(apiPath, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(apiCode)
		w.Write([]byte(apiBody))
	})
	mux.HandleFunc("/trace", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(traceBody))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL + apiPath, srv.URL + "/trace"
}

func TestLlamaUnlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"title":"Authentication Error","status":401}`))
	}))
	defer srv.Close()
	swapURL(t, &llamaAPIURL, srv.URL)
	res := checkLlama(context.Background(), directDialer())
	if res.Status != model.StatusUnlock {
		t.Fatalf("期望解锁，得到 %+v", res)
	}
}

func TestLlamaRegionBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"error":"not supported in your region"}`))
	}))
	defer srv.Close()
	swapURL(t, &llamaAPIURL, srv.URL)
	res := checkLlama(context.Background(), directDialer())
	if res.Status != model.StatusFailed || res.Info != "地区限制" {
		t.Fatalf("期望失败(地区限制)，得到 %+v", res)
	}
}

func TestLlamaOtherError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	swapURL(t, &llamaAPIURL, srv.URL)
	res := checkLlama(context.Background(), directDialer())
	if res.Status != model.StatusFailed || res.Info == "地区限制" {
		t.Fatalf("期望失败(非地区限制)，得到 %+v", res)
	}
}

func TestPerplexityAPIUnlock(t *testing.T) {
	api, trace := apiTraceMock(t, "/chat/completions", 401, `{"error":{"code":401,"type":"invalid_api_key"}}`, "loc=US\n")
	swapURL(t, &perplexityAPIURL, api)
	swapURL(t, &perplexityTraceURL, trace)
	res := checkPerplexity(context.Background(), directDialer())
	if res.Status != model.StatusUnlock || res.Region != "US" {
		t.Fatalf("期望解锁(US)，得到 %+v", res)
	}
}

func TestPerplexityAPIRegionBlocked(t *testing.T) {
	api, trace := apiTraceMock(t, "/chat/completions", 403, `{"error":"not available in your country"}`, "loc=CN\n")
	swapURL(t, &perplexityAPIURL, api)
	swapURL(t, &perplexityTraceURL, trace)
	res := checkPerplexity(context.Background(), directDialer())
	if res.Status != model.StatusFailed || res.Region != "CN" {
		t.Fatalf("期望失败(CN)，得到 %+v", res)
	}
}

func TestPerplexityAPIOtherError(t *testing.T) {
	api, trace := apiTraceMock(t, "/chat/completions", 500, ``, "loc=US\n")
	swapURL(t, &perplexityAPIURL, api)
	swapURL(t, &perplexityTraceURL, trace)
	res := checkPerplexity(context.Background(), directDialer())
	if res.Status != model.StatusFailed || res.Info == "地区限制" {
		t.Fatalf("期望失败(非地区限制)，得到 %+v", res)
	}
}

func TestSoraAPIUnlock(t *testing.T) {
	api, trace := apiTraceMock(t, "/v1/videos", 401, `{"error":{"code":"invalid_api_key"}}`, "loc=US\n")
	swapURL(t, &soraAPIURL, api)
	swapURL(t, &openAITraceURL, trace)
	res := checkSora(context.Background(), directDialer())
	if res.Status != model.StatusUnlock || res.Region != "US" {
		t.Fatalf("期望解锁(US)，得到 %+v", res)
	}
}

func TestSoraAPIRegionBlocked(t *testing.T) {
	api, trace := apiTraceMock(t, "/v1/videos", 403, `{"error":{"code":"unsupported_country_region_territory","message":"Country, region, or territory not supported"}}`, "loc=CN\n")
	swapURL(t, &soraAPIURL, api)
	swapURL(t, &openAITraceURL, trace)
	res := checkSora(context.Background(), directDialer())
	if res.Status != model.StatusFailed || res.Region != "CN" {
		t.Fatalf("期望失败(CN)，得到 %+v", res)
	}
}

func TestGrokUnlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`<html>Grok</html>`))
	}))
	defer srv.Close()
	swapURL(t, &grokURL, srv.URL)
	res := checkGrok(context.Background(), directDialer())
	if res.Status != model.StatusUnlock {
		t.Fatalf("期望解锁，得到 %+v", res)
	}
}

func TestGrokRegionBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`<html>not available in your country</html>`))
	}))
	defer srv.Close()
	swapURL(t, &grokURL, srv.URL)
	res := checkGrok(context.Background(), directDialer())
	if res.Status != model.StatusFailed {
		t.Fatalf("期望失败，得到 %+v", res)
	}
}

func TestGrokHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	}))
	defer srv.Close()
	swapURL(t, &grokURL, srv.URL)
	res := checkGrok(context.Background(), directDialer())
	if res.Status != model.StatusFailed {
		t.Fatalf("期望失败，得到 %+v", res)
	}
}

func TestAbemaAPIJapanUnlockAndBlocks(t *testing.T) {
	t.Run("JP_Unlock", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Write([]byte(`{"isoCountryCode":"JP"}`))
		}))
		defer srv.Close()
		swapURL(t, &abemaAPIURL, srv.URL)
		res := checkAbema(context.Background(), directDialer())
		if res.Status != model.StatusUnlock || res.Region != "JP" {
			t.Fatalf("期望解锁(JP)，得到 %+v", res)
		}
	})

	t.Run("OverseaOnly", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Write([]byte(`{"isoCountryCode":"US"}`))
		}))
		defer srv.Close()
		swapURL(t, &abemaAPIURL, srv.URL)
		res := checkAbema(context.Background(), directDialer())
		if res.Status != model.StatusFailed || res.Region != "US" {
			t.Fatalf("期望非日本地区返回失败(US)，得到 %+v", res)
		}
	})

	t.Run("AnonymousIP_403", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(403)
			w.Write([]byte(`{"message":"anonymous_ip"}`))
		}))
		defer srv.Close()
		swapURL(t, &abemaAPIURL, srv.URL)
		res := checkAbema(context.Background(), directDialer())
		if res.Status != model.StatusFailed || res.Info != "HTTP 403 (anonymous_ip)" {
			t.Fatalf("期望返回 HTTP 403 (anonymous_ip) 失败，得到 %+v", res)
		}
	})
}

func TestOpenAICountryBlockedSurvivesIOSFailure(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/trace", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("loc=CN\n"))
	})
	mux.HandleFunc("/compliance", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"error":{"code":"unsupported_country"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	swapURL(t, &openAITraceURL, srv.URL+"/trace")
	swapURL(t, &openAIComplianceURL, srv.URL+"/compliance")
	swapURL(t, &openAIIOSURL, "http://127.0.0.1:1/unreachable")

	res := checkOpenAI(context.Background(), directDialer())
	if res.Status != model.StatusFailed || res.Region != "CN" || res.Info != "地区不支持" {
		t.Fatalf("期望即使 ios 探针出错仍返回地区不支持，得到 %+v", res)
	}
}

func TestDesktopUAModernChrome(t *testing.T) {
	if !strings.Contains(desktopUA, "Chrome/131.0.0.0") {
		t.Fatalf("期望 desktopUA 为现代 Chrome 版本，得到 %q", desktopUA)
	}
}

func TestNetflixFullUnlockShortCircuitsOriginal(t *testing.T) {
	var origHits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/title/81280792", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`<html>{"requestCountry":{"id":"JP"}}</html>`))
	})
	mux.HandleFunc("/title/80057281", func(w http.ResponseWriter, r *http.Request) {
		origHits.Add(1)
		w.WriteHeader(200)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	swapURL(t, &netflixNonOriginalID, srv.URL+"/title/81280792")
	swapURL(t, &netflixOriginalID, srv.URL+"/title/80057281")

	res := checkNetflix(context.Background(), directDialer())
	if res.Status != model.StatusUnlock || res.Region != "JP" {
		t.Fatalf("期望解锁(JP)，得到 %+v", res)
	}
	if origHits.Load() != 0 {
		t.Fatalf("非自制剧已解锁时不应再请求自制剧页，实际请求次数=%d", origHits.Load())
	}
}

func TestRunAIDedupOpenAIAndSora(t *testing.T) {
	var traceHits, compHits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/openai-trace", func(w http.ResponseWriter, r *http.Request) {
		traceHits.Add(1)
		w.Write([]byte("loc=US\n"))
	})
	mux.HandleFunc("/trace", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("loc=US\n"))
	})
	mux.HandleFunc("/compliance", func(w http.ResponseWriter, r *http.Request) {
		compHits.Add(1)
		w.WriteHeader(200)
		w.Write([]byte(`{"cookie_requirements":{"required":false}}`))
	})
	mux.HandleFunc("/ios", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"cf_details":"dc","type":"dc"}`))
	})
	mux.HandleFunc("/v1/videos", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	})
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{}`))
	})
	mux.HandleFunc("/401", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	swapURL(t, &openAITraceURL, srv.URL+"/openai-trace")
	swapURL(t, &openAIComplianceURL, srv.URL+"/compliance")
	swapURL(t, &openAIIOSURL, srv.URL+"/ios")
	swapURL(t, &soraAPIURL, srv.URL+"/v1/videos")
	swapURL(t, &copilotURL, srv.URL+"/ok")
	swapURL(t, &geminiURL, srv.URL+"/ok")
	swapURL(t, &llamaAPIURL, srv.URL+"/401")
	swapURL(t, &claudeTraceURL, srv.URL+"/trace")
	swapURL(t, &claudeAPIURL, srv.URL+"/401")
	swapURL(t, &perplexityAPIURL, srv.URL+"/401")
	swapURL(t, &perplexityTraceURL, srv.URL+"/trace")
	swapURL(t, &grokURL, srv.URL+"/ok")

	findPlatform := func(res []model.UnlockResult, name string) model.UnlockResult {
		for _, r := range res {
			if r.Platform == name {
				return r
			}
		}
		t.Fatalf("未找到平台 %s", name)
		return model.UnlockResult{}
	}

	first := RunAI(context.Background(), directDialer())
	if r := findPlatform(first, "openai"); r.Status != model.StatusUnlock || r.Region != "US" {
		t.Fatalf("首次 RunAI openai 期望解锁(US)，得到 %+v", r)
	}
	if r := findPlatform(first, "sora"); r.Status != model.StatusUnlock || r.Region != "US" {
		t.Fatalf("首次 RunAI sora 期望解锁(US)，得到 %+v", r)
	}
	if traceHits.Load() != 1 || compHits.Load() != 1 {
		t.Fatalf("单节点 RunAI 内 openAITrace/openAICompliance 应各仅请求 1 次，实际 trace=%d compliance=%d",
			traceHits.Load(), compHits.Load())
	}

	// 第二次 RunAI 代表另一节点，必须独立重新请求，不跨节点串缓存。
	second := RunAI(context.Background(), directDialer())
	if r := findPlatform(second, "sora"); r.Status != model.StatusUnlock || r.Region != "US" {
		t.Fatalf("第二次 RunAI sora 期望解锁(US)，得到 %+v", r)
	}
	if traceHits.Load() != 2 || compHits.Load() != 2 {
		t.Fatalf("跨节点第二次 RunAI 应各自请求一次，实际 trace=%d compliance=%d",
			traceHits.Load(), compHits.Load())
	}
}

func TestClaudeAPIFlow(t *testing.T) {
	setupClaudeAPIMock := func(t *testing.T, traceBody string, apiCode int, apiBody string) {
		t.Helper()
		mux := http.NewServeMux()
		mux.HandleFunc("/trace", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(traceBody))
		})
		mux.HandleFunc("/v1/messages", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("期望 POST 方法，得到 %s", r.Method)
			}
			if got := r.Header.Get("Content-Type"); got != "application/json" {
				t.Errorf("期望 Content-Type: application/json，得到 %q", got)
			}
			b, _ := io.ReadAll(r.Body)
			if string(b) != "{}" {
				t.Errorf("期望请求体 {}，得到 %q", string(b))
			}
			w.WriteHeader(apiCode)
			w.Write([]byte(apiBody))
		})
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)
		swapURL(t, &claudeTraceURL, srv.URL+"/trace")
		swapURL(t, &claudeAPIURL, srv.URL+"/v1/messages")
	}

	t.Run("US_401_Unlock", func(t *testing.T) {
		setupClaudeAPIMock(t, "loc=US\n", 401, `{"type":"error","error":{"type":"authentication_error"}}`)
		res := checkClaude(context.Background(), directDialer())
		if res.Status != model.StatusUnlock || res.Region != "US" {
			t.Fatalf("期望解锁(US)，得到 %+v", res)
		}
	})

	t.Run("JP_400_Unlock", func(t *testing.T) {
		setupClaudeAPIMock(t, "loc=JP\n", 400, `{"type":"error","error":{"type":"invalid_request_error"}}`)
		res := checkClaude(context.Background(), directDialer())
		if res.Status != model.StatusUnlock || res.Region != "JP" {
			t.Fatalf("期望解锁(JP)，得到 %+v", res)
		}
	})

	t.Run("UnsupportedRegion_HK_EvenOn401", func(t *testing.T) {
		setupClaudeAPIMock(t, "loc=HK\n", 401, `{"type":"error","error":{"type":"authentication_error"}}`)
		res := checkClaude(context.Background(), directDialer())
		if res.Status != model.StatusFailed || res.Region != "HK" {
			t.Fatalf("HK 即使 API 返回 401 也应判失败，得到 %+v", res)
		}
	})

	t.Run("HTTP403_UnsupportedMessage", func(t *testing.T) {
		setupClaudeAPIMock(t, "loc=US\n", 403, `{"error":{"message":"Requests from this country or region are not allowed"}}`)
		res := checkClaude(context.Background(), directDialer())
		if res.Status != model.StatusFailed {
			t.Fatalf("期望失败，得到 %+v", res)
		}
	})

	t.Run("HTTP403_CloudflareChallenge", func(t *testing.T) {
		setupClaudeAPIMock(t, "loc=US\n", 403, `<html><title>Just a moment...</title></html>`)
		res := checkClaude(context.Background(), directDialer())
		if res.Status != model.StatusFailed {
			t.Fatalf("期望失败，得到 %+v", res)
		}
	})
}

func TestTikTokHKNotFoundAndFallback(t *testing.T) {
	t.Run("HKNotFound_Redirect", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/explore", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/hk/notfound", http.StatusFound)
		})
		mux.HandleFunc("/hk/notfound", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
			w.Write([]byte(`<html>Not Found</html>`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()
		swapURL(t, &tiktokAPIURL, srv.URL+"/explore")

		res := checkTikTok(context.Background(), directDialer())
		if res.Status != model.StatusFailed || res.Region != "HK" {
			t.Fatalf("期望失败(HK)，得到 %+v", res)
		}
	})

	t.Run("HKNotFound_BodyURL", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Write([]byte(`<link rel="canonical" href="https://www.tiktok.com/hk/notfound">`))
		}))
		defer srv.Close()
		swapURL(t, &tiktokAPIURL, srv.URL)

		res := checkTikTok(context.Background(), directDialer())
		if res.Status != model.StatusFailed || res.Region != "HK" {
			t.Fatalf("期望失败(HK)，得到 %+v", res)
		}
	})

	t.Run("ExploreMissesRegion_FallbackToHomeUnlock", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/explore", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Write([]byte(`<html>explore without region</html>`))
		})
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Write([]byte(`<html><script>{"region":"sg"}</script></html>`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()
		swapURL(t, &tiktokAPIURL, srv.URL+"/explore")
		swapURL(t, &tiktokURL, srv.URL+"/")

		res := checkTikTok(context.Background(), directDialer())
		if res.Status != model.StatusUnlock || res.Region != "SG" {
			t.Fatalf("期望回退首页解锁(SG)，得到 %+v", res)
		}
	})

	t.Run("ExploreMissesRegion_FallbackHKNotFound", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/explore", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Write([]byte(`<html>explore without region</html>`))
		})
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/hk/notfound", http.StatusFound)
		})
		mux.HandleFunc("/hk/notfound", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()
		swapURL(t, &tiktokAPIURL, srv.URL+"/explore")
		swapURL(t, &tiktokURL, srv.URL+"/")

		res := checkTikTok(context.Background(), directDialer())
		if res.Status != model.StatusFailed || res.Region != "HK" {
			t.Fatalf("期望回退首页命中 HK 封锁，得到 %+v", res)
		}
	})
}

func TestDisneyPlusBamgridAndLegacy(t *testing.T) {
	t.Run("Bamgrid_Unlock_US", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/devices", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+disneyAuthToken {
				t.Errorf("/devices Authorization 不符: %q", r.Header.Get("Authorization"))
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["deviceFamily"] != "browser" || body["applicationRuntime"] != "chrome" || body["deviceProfile"] != "windows" {
				t.Errorf("/devices JSON 体不符: %+v err=%v", body, err)
			}
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"assertion":"dev-assertion-123"}`))
		})
		mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != disneyAuthToken {
				t.Errorf("/token Authorization 应为裸 token，实际: %q", r.Header.Get("Authorization"))
			}
			if err := r.ParseForm(); err != nil || r.Form.Get("subject_token") != "dev-assertion-123" || r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:token-exchange" {
				t.Errorf("/token 表单参数不符: %+v err=%v", r.Form, err)
			}
			w.Write([]byte(`{"access_token":"acc","refresh_token":"ref-xyz"}`))
		})
		mux.HandleFunc("/graph/v1/device/graphql", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != disneyAuthToken {
				t.Errorf("/graphql Authorization 应为裸 token，实际: %q", r.Header.Get("Authorization"))
			}
			b, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(b), "refreshToken") || !strings.Contains(string(b), "sessionId") || !strings.Contains(string(b), "ref-xyz") {
				t.Errorf("/graphql 请求体未包含 refreshToken/sessionId: %s", string(b))
			}
			w.Write([]byte(`{"data":{"refreshToken":{"activeSession":{"sessionId":"sess-1"}}},"extensions":{"sdk":{"session":{"inSupportedLocation":true,"location":{"countryCode":"us"}}}}}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()
		swapURL(t, &disneyDevicesURL, srv.URL+"/devices")
		swapURL(t, &disneyTokenURL, srv.URL+"/token")
		swapURL(t, &disneyGraphQLURL, srv.URL+"/graph/v1/device/graphql")

		res := checkDisneyPlus(context.Background(), directDialer())
		if res.Status != model.StatusUnlock || res.Region != "US" {
			t.Fatalf("期望解锁(US)，得到 %+v", res)
		}
	})

	t.Run("Bamgrid_ForbiddenLocation_400", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/devices", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"assertion":"dev-assertion-123"}`))
		})
		mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"unauthorized_client","error_description":"forbidden-location"}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()
		swapURL(t, &disneyDevicesURL, srv.URL+"/devices")
		swapURL(t, &disneyTokenURL, srv.URL+"/token")
		swapURL(t, &disneyGraphQLURL, srv.URL+"/graph/v1/device/graphql")

		res := checkDisneyPlus(context.Background(), directDialer())
		if res.Status != model.StatusFailed || res.Info != "地区限制" {
			t.Fatalf("期望地区限制失败，得到 %+v", res)
		}
	})

	t.Run("Bamgrid_InSupportedLocationFalse", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/devices", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"assertion":"dev-assertion-123"}`))
		})
		mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"refresh_token":"ref-xyz"}`))
		})
		mux.HandleFunc("/graph/v1/device/graphql", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"data":{"refreshToken":{"activeSession":{"sessionId":"sess-2"}}},"extensions":{"sdk":{"session":{"inSupportedLocation":false,"location":{"countryCode":"HK"}}}}}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()
		swapURL(t, &disneyDevicesURL, srv.URL+"/devices")
		swapURL(t, &disneyTokenURL, srv.URL+"/token")
		swapURL(t, &disneyGraphQLURL, srv.URL+"/graph/v1/device/graphql")

		res := checkDisneyPlus(context.Background(), directDialer())
		if res.Status != model.StatusFailed || res.Region != "HK" {
			t.Fatalf("期望地区不支持失败(HK)，得到 %+v", res)
		}
	})

	t.Run("Bamgrid_DataActiveSessionFallback", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/devices", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"assertion":"dev-assertion-123"}`))
		})
		mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"refresh_token":"ref-xyz"}`))
		})
		mux.HandleFunc("/graph/v1/device/graphql", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"data":{"refreshToken":{"activeSession":{"inSupportedLocation":true,"location":{"countryCode":"jp"}}}}}`))
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()
		swapURL(t, &disneyDevicesURL, srv.URL+"/devices")
		swapURL(t, &disneyTokenURL, srv.URL+"/token")
		swapURL(t, &disneyGraphQLURL, srv.URL+"/graph/v1/device/graphql")

		res := checkDisneyPlus(context.Background(), directDialer())
		if res.Status != model.StatusUnlock || res.Region != "JP" {
			t.Fatalf("期望解锁(JP)，得到 %+v", res)
		}
	})

	t.Run("Legacy_BackwardsCompat", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
			w.Write([]byte(`<html>Disney+</html>`))
		})
		mux.HandleFunc("/blocked", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/unavailable", http.StatusFound)
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()

		swapURL(t, &disneyPlusURL, srv.URL+"/ok")
		if res := checkDisneyPlus(context.Background(), directDialer()); res.Status != model.StatusUnlock {
			t.Fatalf("旧版兼容路径期望解锁，得到 %+v", res)
		}

		swapURL(t, &disneyPlusURL, srv.URL+"/blocked")
		if res := checkDisneyPlus(context.Background(), directDialer()); res.Status != model.StatusFailed {
			t.Fatalf("旧版兼容路径期望失败，得到 %+v", res)
		}
	})
}
