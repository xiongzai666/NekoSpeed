package render

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

// buildFontCandidates 构建全平台（Windows 各版本/盘符、macOS、Linux、Android、CI 容器）的中文字体及兜底字体路径列表。
func buildFontCandidates() []string {
	var list []string

	// 1. 程序所在目录或工作目录的 fonts/ 子目录（支持便携版自带字体）
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		list = append(list,
			filepath.Join(exeDir, "fonts", "NotoSansCJKsc-Regular.otf"),
			filepath.Join(exeDir, "fonts", "msyh.ttc"),
			filepath.Join(exeDir, "NotoSansCJKsc-Regular.otf"),
		)
	}
	list = append(list,
		filepath.Join("fonts", "NotoSansCJKsc-Regular.otf"),
		filepath.Join("android", "app", "src", "main", "assets", "fonts", "NotoSansCJKsc-Regular.otf"),
	)

	// 2. Windows 全版本（Win7/Win10/Win11/Server，支持非 C 盘系统与用户级字体目录）
	if runtime.GOOS == "windows" {
		winDirs := []string{}
		for _, envKey := range []string{"WINDIR", "SystemRoot"} {
			if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
				winDirs = append(winDirs, filepath.Join(v, "Fonts"))
			}
		}
		winDirs = append(winDirs, `C:\Windows\Fonts`)
		if localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); localAppData != "" {
			winDirs = append(winDirs, filepath.Join(localAppData, "Microsoft", "Windows", "Fonts"))
		}
		winFontNames := []string{
			"msyh.ttc",    // 微软雅黑 (Win8.1/10/11)
			"msyh.ttf",    // 微软雅黑 (Win7)
			"deng.ttf",    // 等线 (Win10/11)
			"simhei.ttf",  // 黑体
			"simsun.ttc",  // 宋体
			"msjh.ttc",    // 微软正黑体 (繁体 Windows)
			"msjh.ttf",    // 微软正黑体 (Win7 繁体)
			"mingliu.ttc", // 细明体
			"meiryo.ttc",  // 日文 Windows
			"malgun.ttf",  // 韩文 Windows
			"segoeui.ttf", // 英文 Windows 兜底
			"arial.ttf",
		}
		seen := map[string]bool{}
		for _, d := range winDirs {
			for _, fn := range winFontNames {
				p := filepath.Join(d, fn)
				if !seen[strings.ToLower(p)] {
					seen[strings.ToLower(p)] = true
					list = append(list, p)
				}
			}
		}
	}

	// 3. macOS / Linux / Android 常见中文字体与 CI 环境兜底字体
	list = append(list,
		"/System/Library/Fonts/PingFang.ttc",
		"/System/Library/Fonts/STHeiti Light.ttc",
		"/System/Library/Fonts/Hiragino Sans GB.ttc",
		"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/opentype/noto/NotoSansCJKsc-Regular.otf",
		"/usr/share/fonts/google-noto-cjk/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
		"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
		"/usr/share/fonts/wenquanyi/wqy-microhei/wqy-microhei.ttc",
		"/system/fonts/NotoSansCJK-Regular.ttc",
		"/system/fonts/NotoSansSC-Regular.otf",
		"/system/fonts/NotoSansCJKsc-Regular.otf",
		"/system/fonts/SourceHanSansSC-Regular.otf",
		"/system/fonts/DroidSansFallbackFull.ttf",
		"/system/fonts/DroidSansFallback.ttf",
		"/system/fonts/MiSans-Regular.ttf",
		"/system/fonts/MiSans-Regular.ttc",
		"/system/fonts/MiSansVF.ttf",
		"/system/fonts/HarmonyOS_Sans_SC_Regular.ttf",
		"/system/fonts/HarmonyOS_Sans_SC.ttf",
		"/system/fonts/OnePlusSans-Regular.ttf",
		"/system/fonts/oplus-Regular.ttf",
		"/system/fonts/OPPOSans-Regular.ttf",
		"/system/fonts/Sans-Regular.ttf",
		"/system/fonts/vivo-Sans-Regular.ttf",
		// 极简 Linux / macOS CI 容器无中文字体时的西文字体兜底（配合 sanitize 自动跳过无字形字符）
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
		"/System/Library/Fonts/Helvetica.ttc",
		"/System/Library/Fonts/Supplemental/Arial.ttf",
	)
	return list
}

var fontCandidates = buildFontCandidates()

// fontBook 缓存已加载的字体（按自定义路径）与按（字体, 字号）派生的 font.Face。
type faceKey struct {
	font *opentype.Font
	size float64
}

type fontBook struct {
	mu    sync.Mutex
	fonts map[string]*opentype.Font
	faces map[faceKey]font.Face
}

var book = &fontBook{
	fonts: make(map[string]*opentype.Font),
	faces: make(map[faceKey]font.Face),
}

// loadFont 按 meta.FontPath → 系统候选路径的顺序加载字体。
func loadFont(custom string) (*opentype.Font, error) {
	key := strings.TrimSpace(custom)
	book.mu.Lock()
	defer book.mu.Unlock()
	if f, ok := book.fonts[key]; ok {
		return f, nil
	}
	paths := fontCandidates
	if key != "" {
		paths = append([]string{key}, fontCandidates...)
	}
	var errs []string
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		f, err := parseFont(b)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", p, err))
			continue
		}
		book.fonts[key] = f
		return f, nil
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("字体加载失败（%s）；请通过 -font 指定一个中文字体文件", strings.Join(errs, "; "))
	}
	return nil, fmt.Errorf("未找到可用中文字体（已尝试常见系统字体路径）；请通过 -font 指定字体文件")
}

// parseFont 解析字体文件内容，优先按 TTC 合集解析，失败再按单字体解析。
func parseFont(b []byte) (*opentype.Font, error) {
	if coll, err := opentype.ParseCollection(b); err == nil && coll.NumFonts() > 0 {
		return coll.Font(0)
	}
	return opentype.Parse(b)
}

// face 返回指定字体与字号的 font.Face（带缓存）。
func face(f *opentype.Font, size float64) (font.Face, error) {
	key := faceKey{font: f, size: size}
	book.mu.Lock()
	defer book.mu.Unlock()
	if fc, ok := book.faces[key]; ok {
		return fc, nil
	}
	fc, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, err
	}
	book.faces[key] = fc
	return fc, nil
}

// hasGlyph 判断字体是否包含某 rune 的字形。
func hasGlyph(fc font.Face, r rune) bool {
	_, ok := fc.GlyphAdvance(r)
	return ok
}

// sanitize 去掉字体中没有字形的 rune（如国旗 emoji），避免渲染 panic 或豆腐块。
func sanitize(fc font.Face, s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 0x20 || hasGlyph(fc, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
