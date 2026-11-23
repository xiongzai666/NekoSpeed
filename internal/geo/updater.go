package geo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"
)

// GeoIP 离线库更新源定义（包级变量以便单测替换）。
var (
	GeoIPReleaseBaseURL     = "https://github.com/xiongzai666/NekoSpeed/releases/download/geoip-latest"
	GeoCNUpstreamURL        = "https://github.com/ljxi/GeoCN/releases/latest/download/GeoCN.mmdb"
	GeoLiteCityUpstreamURL  = "https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-City.mmdb"
	GeoLiteASNUpstreamURL   = "https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-ASN.mmdb"
	IPinfoLiteDownloadBaseURL = "https://ipinfo.io/data/ipinfo_lite.mmdb"
)

const (
	maxDBDownloadBytes = 150 << 20
	perDBTimeout       = 3 * time.Minute
	staleDBAge         = 7 * 24 * time.Hour
)

// DBFileStatus 描述单个本地 MMDB 文件的状态。
type DBFileStatus struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Present   bool   `json:"present"`
	Size      int64  `json:"size"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	AgeDays   int    `json:"ageDays"`
	Stale     bool   `json:"stale"`
}

// DBStatus 汇总当前离线 IP 库目录的可用状态与运行模式。
type DBStatus struct {
	Dir         string         `json:"dir"`
	Ready       bool           `json:"ready"`
	FullReady   bool           `json:"fullReady"`
	Stale       bool           `json:"stale"`
	Mode        string         `json:"mode"`     // "offline" | "hybrid" | "online"
	ModeText    string         `json:"modeText"` // 人类可读状态描述
	Attribution string         `json:"attribution"`
	Files       []DBFileStatus `json:"files"`
}

// UpdateOptions 控制 UpdateDBs 的目标目录、代理、可选官方 Token 与进度回调。
type UpdateOptions struct {
	Dir         string
	Proxy       string
	IPinfoToken string
	OnProgress  func(stage string, done, total int)
}

// UpdateResult 记录单次更新的结果摘要。
type UpdateResult struct {
	Dir     string   `json:"dir"`
	Updated []string `json:"updated"`
	Failed  []string `json:"failed,omitempty"`
	Status  DBStatus `json:"status"`
}

// GetDBStatus 检查 dir 下各离线 MMDB 的存在情况、体积与新鲜度。
func GetDBStatus(dir string) DBStatus {
	if strings.TrimSpace(dir) == "" {
		dir = DefaultDBDir()
	}
	specs := []struct {
		name  string
		label string
	}{
		{geoCNFileName, "国内区县+运营商 (GeoCN)"},
		{ipinfoLiteFileName, "全球日更 ASN (IPinfo Lite)"},
		{geoLiteASNFileName, "全球兜底 ASN (GeoLite2-ASN)"},
		{geoLiteCityName, "全球城市 (GeoLite2-City)"},
	}

	now := time.Now()
	files := make([]DBFileStatus, 0, len(specs))
	presentMap := make(map[string]bool, len(specs))
	anyStale := false

	for _, sp := range specs {
		fs := DBFileStatus{Name: sp.name, Label: sp.label}
		if info, err := os.Stat(filepath.Join(dir, sp.name)); err == nil && !info.IsDir() && info.Size() > 0 {
			fs.Present = true
			fs.Size = info.Size()
			fs.UpdatedAt = info.ModTime().Format("2006-01-02 15:04")
			age := now.Sub(info.ModTime())
			if age < 0 {
				age = 0
			}
			fs.AgeDays = int(age / (24 * time.Hour))
			fs.Stale = age > staleDBAge
			if fs.Stale {
				anyStale = true
			}
			presentMap[sp.name] = true
		}
		files = append(files, fs)
	}

	hasCN := presentMap[geoCNFileName]
	hasASN := presentMap[ipinfoLiteFileName] || presentMap[geoLiteASNFileName]
	hasCity := presentMap[geoLiteCityName]

	ready := hasCN || hasASN || hasCity
	fullReady := hasCN && hasASN && hasCity

	mode := "online"
	modeText := "在线多源模式（尚未下载离线 IP 库）"
	if fullReady {
		mode = "offline"
		if presentMap[ipinfoLiteFileName] {
			modeText = "离线多库加速（GeoCN + IPinfo Lite + GeoLite2-City）"
		} else {
			modeText = "离线多库加速（GeoCN + GeoLite2-ASN + GeoLite2-City）"
		}
	} else if ready {
		mode = "hybrid"
		modeText = "混合模式（部分离线库 + 在线补全）"
	}

	return DBStatus{
		Dir:         dir,
		Ready:       ready,
		FullReady:   fullReady,
		Stale:       anyStale,
		Mode:        mode,
		ModeText:    modeText,
		Attribution: "IP 数据由 IPinfo (CC BY-SA 4.0)、MaxMind GeoLite2、GeoCN 提供",
		Files:       files,
	}
}

// UpdateDBs 下载并校验最新离线 MMDB（优先仓内 geoip-latest Release，失败自动回退上游镜像），
// 并行拉取 3 个库，原子替换本地目录中的数据库文件并刷新进程级 GeoIP 缓存。
func UpdateDBs(ctx context.Context, opts UpdateOptions) (UpdateResult, error) {
	dir := strings.TrimSpace(opts.Dir)
	if dir == "" {
		dir = DefaultDBDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return UpdateResult{}, fmt.Errorf("创建 GeoIP 目录 %q 失败: %w", dir, err)
	}

	client, err := newUpdaterHTTPClient(opts.Proxy)
	if err != nil {
		return UpdateResult{}, err
	}
	defer client.CloseIdleConnections()

	report := func(stage string, done, total int) {
		if opts.OnProgress != nil {
			opts.OnProgress(stage, done, total)
		}
	}

	const totalSteps = 3
	res := UpdateResult{Dir: dir}
	token := strings.TrimSpace(opts.IPinfoToken)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("NEKOSPEED_IPINFO_TOKEN"))
	}

	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		done int
	)

	report("正在并行更新离线 IP 库…", 0, totalSteps)

	recordResult := func(updatedFile, failName string, failErr error, stage string) {
		mu.Lock()
		defer mu.Unlock()
		if failErr == nil {
			res.Updated = append(res.Updated, updatedFile)
		} else {
			res.Failed = append(res.Failed, fmt.Sprintf("%s: %v", failName, failErr))
		}
		done++
		report(stage, done, totalSteps)
	}

	relBase := strings.TrimRight(GeoIPReleaseBaseURL, "/")

	// 1. GeoCN.mmdb（国内双栈区县+运营商）
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := downloadFirstAvailable(ctx, client, []string{
			relBase + "/" + geoCNFileName,
			GeoCNUpstreamURL,
		}, filepath.Join(dir, geoCNFileName))
		if err != nil {
			recordResult("", geoCNFileName, err, "国内库 (GeoCN.mmdb) 更新失败")
			return
		}
		recordResult(geoCNFileName, "", nil, "国内库更新完成 (GeoCN.mmdb)")
	}()

	// 2. 全球 ASN 库（优先 IPinfo Lite，未配置或未发布时自动回退 GeoLite2-ASN）
	wg.Add(1)
	go func() {
		defer wg.Done()
		var ipinfoCandidates []string
		if token != "" {
			ipinfoCandidates = append(ipinfoCandidates, IPinfoLiteDownloadBaseURL+"?token="+url.QueryEscape(token))
		}
		ipinfoCandidates = append(ipinfoCandidates, relBase+"/"+ipinfoLiteFileName)
		if err := downloadFirstAvailable(ctx, client, ipinfoCandidates, filepath.Join(dir, ipinfoLiteFileName)); err == nil {
			recordResult(ipinfoLiteFileName, "", nil, "全球 ASN 库更新完成 (IPinfo Lite)")
			return
		}
		if err := downloadFirstAvailable(ctx, client, []string{
			relBase + "/" + geoLiteASNFileName,
			GeoLiteASNUpstreamURL,
		}, filepath.Join(dir, geoLiteASNFileName)); err != nil {
			recordResult("", geoLiteASNFileName, err, "全球 ASN 库更新失败")
			return
		}
		recordResult(geoLiteASNFileName, "", nil, "全球 ASN 库更新完成 (GeoLite2-ASN)")
	}()

	// 3. GeoLite2-City.mmdb（全球城市中文名）
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := downloadFirstAvailable(ctx, client, []string{
			relBase + "/" + geoLiteCityName,
			GeoLiteCityUpstreamURL,
		}, filepath.Join(dir, geoLiteCityName))
		if err != nil {
			recordResult("", geoLiteCityName, err, "全球城市库 (GeoLite2-City.mmdb) 更新失败")
			return
		}
		recordResult(geoLiteCityName, "", nil, "全球城市库更新完成 (GeoLite2-City.mmdb)")
	}()

	wg.Wait()
	sort.Strings(res.Updated)
	sort.Strings(res.Failed)

	if len(res.Updated) > 0 {
		ClearGlobalCache()
	}
	res.Status = GetDBStatus(dir)
	report("IP 库更新完成", totalSteps, totalSteps)

	if len(res.Updated) == 0 {
		return res, fmt.Errorf("全部 IP 库下载失败（%s）；若访问 GitHub 受限，请在高级参数中填写代理或开启系统代理/TUN", strings.Join(res.Failed, "；"))
	}
	return res, nil
}

func newUpdaterHTTPClient(proxyStr string) (*http.Client, error) {
	proxyFn := http.ProxyFromEnvironment
	proxyStr = strings.TrimSpace(proxyStr)
	if proxyStr != "" {
		u, err := url.Parse(proxyStr)
		if err != nil {
			return nil, fmt.Errorf("解析代理地址 %q 失败: %w", proxyStr, err)
		}
		switch u.Scheme {
		case "http", "https", "socks5", "socks5h":
		default:
			return nil, fmt.Errorf("不支持的代理协议 %q（支持 http/https/socks5）", u.Scheme)
		}
		proxyFn = http.ProxyURL(u)
	}
	return &http.Client{
		Transport: &http.Transport{
			Proxy: proxyFn,
		},
	}, nil
}

func downloadFirstAvailable(ctx context.Context, client *http.Client, urls []string, dstPath string) error {
	var errs []string
	for _, u := range urls {
		if strings.TrimSpace(u) == "" {
			continue
		}
		if err := downloadAndVerifyMMDB(ctx, client, u, dstPath); err == nil {
			return nil
		} else {
			errs = append(errs, err.Error())
		}
		if ctx.Err() != nil {
			break
		}
	}
	if len(errs) == 0 {
		return fmt.Errorf("无可用下载地址")
	}
	return fmt.Errorf("%s", errs[len(errs)-1])
}

func downloadAndVerifyMMDB(ctx context.Context, client *http.Client, rawURL, dstPath string) error {
	safeURL := sanitizeURLForError(rawURL)
	fctx, cancel := context.WithTimeout(ctx, perDBTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(fctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("构造请求失败 (%s): %w", safeURL, err)
	}
	req.Header.Set("User-Agent", "NekoSpeed/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败 (%s): %s", safeURL, sanitizeErrToken(err, rawURL))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d (%s)", resp.StatusCode, safeURL)
	}

	tmpPath := dstPath + ".tmp"
	_ = os.Remove(tmpPath)
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}

	n, copyErr := io.Copy(out, io.LimitReader(resp.Body, maxDBDownloadBytes+1))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("写入失败 (%s): %v", safeURL, copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("关闭文件失败: %w", closeErr)
	}
	if n > maxDBDownloadBytes {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("文件体积超过 %d MiB 上限 (%s)", maxDBDownloadBytes>>20, safeURL)
	}

	db, err := maxminddb.Open(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("MMDB 格式校验失败 (%s): %v", safeURL, err)
	}
	_ = db.Close()

	if err := os.Rename(tmpPath, dstPath); err != nil {
		// Windows 下若目标文件存在且重命名受阻，先移除旧文件再替换。
		_ = os.Remove(dstPath)
		if err2 := os.Rename(tmpPath, dstPath); err2 != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("替换文件失败: %w", err2)
		}
	}
	return nil
}

func sanitizeURLForError(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "(invalid-url)"
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.User = nil
	return u.String()
}

func sanitizeErrToken(err error, rawURL string) string {
	msg := err.Error()
	u, parseErr := url.Parse(rawURL)
	if parseErr == nil && u.RawQuery != "" {
		msg = strings.ReplaceAll(msg, u.RawQuery, "***")
		if tok := u.Query().Get("token"); tok != "" {
			msg = strings.ReplaceAll(msg, tok, "***")
		}
	}
	return msg
}
