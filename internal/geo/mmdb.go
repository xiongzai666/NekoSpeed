package geo

import (
	"bufio"
	_ "embed"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/oschwald/maxminddb-golang"
	"nekospeed/internal/model"
)

//go:embed cn_divisions.txt
var cnDivisionsRaw string

var (
	cnDivOnce sync.Once
	cnDivMap  map[uint32]string
)

const (
	geoCNFileName      = "GeoCN.mmdb"
	ipinfoLiteFileName = "ipinfo_lite.mmdb"
	geoLiteASNFileName = "GeoLite2-ASN.mmdb"
	geoLiteCityName    = "GeoLite2-City.mmdb"
)

// DefaultDBDir 返回离线 GeoIP 数据库目录，可通过 NEKOSPEED_GEOIP_DIR 覆盖，默认工作目录下的 geoip/。
func DefaultDBDir() string {
	if d := strings.TrimSpace(os.Getenv("NEKOSPEED_GEOIP_DIR")); d != "" {
		return d
	}
	return "geoip"
}

func loadCNDivisions() map[uint32]string {
	cnDivOnce.Do(func() {
		m := make(map[uint32]string, 4096)
		sc := bufio.NewScanner(strings.NewReader(cnDivisionsRaw))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) != 2 {
				continue
			}
			code, err := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 32)
			if err != nil {
				continue
			}
			m[uint32(code)] = strings.TrimSpace(parts[1])
		}
		cnDivMap = m
	})
	return cnDivMap
}

func isGenericAdminName(name string) bool {
	return name == "" || name == "市辖区" || name == "未知" || name == "县" || strings.Contains(name, "直辖县")
}

func cleanUnknownLoc(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "未知" || strings.EqualFold(s, "Unknown") {
		return ""
	}
	s = strings.TrimSuffix(s, "未知")
	return strings.TrimSpace(s)
}

func fuseCityDistrict(city, district string) string {
	city = cleanUnknownLoc(city)
	district = cleanUnknownLoc(district)
	if isGenericAdminName(city) {
		city = ""
	}
	if isGenericAdminName(district) {
		district = ""
	}
	if district != "" && city != "" {
		shortCity := strings.TrimSuffix(city, "市")
		if shortCity != "" && !strings.Contains(district, shortCity) {
			return shortCity + strings.TrimSuffix(district, "市")
		}
		return district
	}
	if district != "" {
		return district
	}
	return city
}

// decodeCNDivision 根据 GB/T 2260 六位行政区划代码解析国内省与精简市县名（如 340881 -> "安徽省", "安庆桐城"）。
func decodeCNDivision(code uint32) (region, city string) {
	if code == 0 {
		return "", ""
	}
	divs := loadCNDivisions()
	prov := divs[(code/10000)*10000]
	cityName := divs[(code/100)*100]
	dist := divs[code]

	if isGenericAdminName(cityName) {
		cityName = ""
	}
	if dist == cityName || dist == prov || isGenericAdminName(dist) {
		dist = ""
	}

	region = prov
	switch {
	case dist != "" && cityName != "" && cityName != prov:
		city = fuseCityDistrict(cityName, dist)
	case cityName != "" && cityName != prov:
		city = cityName
	case dist != "" && prov != "":
		city = fuseCityDistrict(prov, dist)
	default:
		city = prov
	}
	return region, city
}

func normalizeCNISP(isp string) string {
	isp = strings.TrimSpace(isp)
	switch isp {
	case "移动":
		return "中国移动"
	case "电信":
		return "中国电信"
	case "联通":
		return "中国联通"
	case "广电":
		return "中国广电"
	case "教育网":
		return "中国教育网"
	default:
		return isp
	}
}

func lookupGeoCNReader(db *maxminddb.Reader, parsed net.IP) (model.GeoInfo, bool) {
	if db == nil || parsed == nil {
		return model.GeoInfo{}, false
	}
	var rec struct {
		DivisionCode uint64 `maxminddb:"division_code"`
		ISP          string `maxminddb:"isp"`
	}
	if err := db.Lookup(parsed, &rec); err != nil {
		return model.GeoInfo{}, false
	}
	isp := normalizeCNISP(rec.ISP)
	if rec.DivisionCode == 0 && isp == "" {
		return model.GeoInfo{}, false
	}
	region, city := decodeCNDivision(uint32(rec.DivisionCode))
	return model.GeoInfo{
		Country: "CN",
		Region:  region,
		City:    city,
		ISP:     isp,
	}, true
}

func lookupIPinfoReader(db *maxminddb.Reader, parsed net.IP, ip string) (model.GeoInfo, bool) {
	if db == nil || parsed == nil {
		return model.GeoInfo{}, false
	}
	var rec struct {
		CountryCode string `maxminddb:"country_code"`
		Country     string `maxminddb:"country"`
		ASN         string `maxminddb:"asn"`
		ASName      string `maxminddb:"as_name"`
	}
	if err := db.Lookup(parsed, &rec); err != nil {
		return model.GeoInfo{}, false
	}
	g := model.GeoInfo{IP: ip, Stack: stackOf(ip)}
	g.Country = strings.ToUpper(strings.TrimSpace(rec.CountryCode))
	if g.Country == "" && len(strings.TrimSpace(rec.Country)) == 2 {
		g.Country = strings.ToUpper(strings.TrimSpace(rec.Country))
	}
	if asn := strings.TrimSpace(rec.ASN); asn != "" {
		if strings.HasPrefix(strings.ToUpper(asn), "AS") {
			g.ASN = strings.ToUpper(asn)
		} else {
			g.ASN = "AS" + asn
		}
	}
	name := strings.TrimSpace(rec.ASName)
	g.Org = name
	g.ISP = name
	if g.Country == "" && g.ASN == "" && g.Org == "" {
		return model.GeoInfo{}, false
	}
	return g, true
}

// lookupMMDBReader 自适应解析 MaxMind（City/ASN/Country）、IPinfo Lite 与 GeoCN 格式的 MMDB。
func lookupMMDBReader(db *maxminddb.Reader, ip string) model.GeoInfo {
	g := model.GeoInfo{IP: ip, Stack: stackOf(ip)}
	if db == nil {
		return g
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return g
	}

	// IPinfo Lite MMDB 的 `country` 字段为字符串（国家全称），优先走专用结构避免与 MaxMind `country` map 冲突。
	if strings.Contains(strings.ToLower(db.Metadata.DatabaseType), "ipinfo") {
		if info, ok := lookupIPinfoReader(db, parsed, ip); ok {
			return info
		}
	}

	var rec struct {
		Country struct {
			ISOCode string            `maxminddb:"iso_code"`
			Names   map[string]string `maxminddb:"names"`
		} `maxminddb:"country"`
		RegisteredCountry struct {
			ISOCode string            `maxminddb:"iso_code"`
			Names   map[string]string `maxminddb:"names"`
		} `maxminddb:"registered_country"`
		Subdivisions []struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"subdivisions"`
		City struct {
			Names map[string]string `maxminddb:"names"`
		} `maxminddb:"city"`
		AutonomousSystemNumber       uint   `maxminddb:"autonomous_system_number"`
		AutonomousSystemOrganization string `maxminddb:"autonomous_system_organization"`
		ISP                          string `maxminddb:"isp"`
		Organization                 string `maxminddb:"organization"`
		// IPinfo 兼容字段（避免重复声明 `country` tag）
		IPinfoCountryCode string `maxminddb:"country_code"`
		IPinfoASN         string `maxminddb:"asn"`
		IPinfoASName      string `maxminddb:"as_name"`
		// GeoCN 字段
		DivisionCode uint64 `maxminddb:"division_code"`
	}
	if err := db.Lookup(parsed, &rec); err != nil {
		if info, ok := lookupIPinfoReader(db, parsed, ip); ok {
			return info
		}
		return g
	}

	// 单库传入 GeoCN.mmdb 时优先用内置行政区划码与运营商表解码。
	if rec.DivisionCode != 0 || (rec.Country.ISOCode == "" && rec.ISP != "" && rec.AutonomousSystemNumber == 0 && rec.IPinfoASN == "") {
		if cn, ok := lookupGeoCNReader(db, parsed); ok {
			cn.IP = ip
			cn.Stack = stackOf(ip)
			return cn
		}
	}

	pickName := func(m map[string]string) string {
		if s, ok := m["zh-CN"]; ok && s != "" {
			return s
		}
		if s, ok := m["en"]; ok && s != "" {
			return s
		}
		for _, s := range m {
			if s != "" {
				return s
			}
		}
		return ""
	}

	g.Country = rec.Country.ISOCode
	if g.Country == "" {
		g.Country = rec.RegisteredCountry.ISOCode
	}
	if g.Country == "" && rec.IPinfoCountryCode != "" {
		g.Country = strings.ToUpper(strings.TrimSpace(rec.IPinfoCountryCode))
	}

	if len(rec.Subdivisions) > 0 {
		g.Region = cleanUnknownLoc(pickName(rec.Subdivisions[0].Names))
	}
	g.City = cleanUnknownLoc(pickName(rec.City.Names))

	if rec.AutonomousSystemNumber != 0 {
		g.ASN = "AS" + strconv.FormatUint(uint64(rec.AutonomousSystemNumber), 10)
	} else if rec.IPinfoASN != "" {
		if strings.HasPrefix(strings.ToUpper(rec.IPinfoASN), "AS") {
			g.ASN = strings.ToUpper(rec.IPinfoASN)
		} else {
			g.ASN = "AS" + rec.IPinfoASN
		}
	}

	switch {
	case rec.ISP != "":
		g.ISP = rec.ISP
	case rec.Organization != "":
		g.ISP = rec.Organization
	case rec.IPinfoASName != "":
		g.ISP = rec.IPinfoASName
	default:
		g.ISP = rec.AutonomousSystemOrganization
	}

	g.Org = rec.AutonomousSystemOrganization
	if g.Org == "" {
		g.Org = rec.Organization
	}
	if g.Org == "" {
		g.Org = rec.IPinfoASName
	}
	return g
}

func openOptionalMMDB(path string) *maxminddb.Reader {
	db, err := maxminddb.Open(path)
	if err != nil {
		return nil
	}
	return db
}

func (r *Resolver) openDirDBs(dir string) {
	if dir == "" {
		return
	}
	r.cnDB = openOptionalMMDB(filepath.Join(dir, geoCNFileName))
	r.ipinfoDB = openOptionalMMDB(filepath.Join(dir, ipinfoLiteFileName))
	r.asnDB = openOptionalMMDB(filepath.Join(dir, geoLiteASNFileName))
	r.cityDB = openOptionalMMDB(filepath.Join(dir, geoLiteCityName))
}

func (r *Resolver) hasLocalDB() bool {
	return r.customDB != nil || r.cnDB != nil || r.ipinfoDB != nil || r.asnDB != nil || r.cityDB != nil
}

// ModeSummary 返回当前 GeoIP 解析器启用的数据源模式描述。
func (r *Resolver) ModeSummary() string {
	if r.customDB != nil {
		return "本地 MMDB 数据库"
	}
	var loaded []string
	if r.cnDB != nil {
		loaded = append(loaded, "GeoCN")
	}
	if r.ipinfoDB != nil {
		loaded = append(loaded, "IPinfo Lite")
	} else if r.asnDB != nil {
		loaded = append(loaded, "GeoLite2-ASN")
	}
	if r.cityDB != nil {
		loaded = append(loaded, "GeoLite2-City")
	}
	if len(loaded) == 0 {
		return "在线多源模式（可在 Web UI 一键下载离线 IP 库）"
	}
	if r.cnDB != nil && (r.ipinfoDB != nil || r.asnDB != nil) && r.cityDB != nil {
		return "离线多库加速（" + strings.Join(loaded, " + ") + "）"
	}
	return "混合模式（本地 " + strings.Join(loaded, " + ") + " + 在线补全）"
}

func (r *Resolver) lookupOffline(ip string) model.GeoInfo {
	g := model.GeoInfo{IP: ip, Stack: stackOf(ip)}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return g
	}

	if r.customDB != nil {
		g = mergeGeoMissing(g, lookupMMDBReader(r.customDB, ip))
	}
	if r.ipinfoDB != nil {
		g = mergeGeoMissing(g, lookupMMDBReader(r.ipinfoDB, ip))
	}
	if r.asnDB != nil && (g.ASN == "" || g.Org == "") {
		asnMeta := lookupMMDBReader(r.asnDB, ip)
		g = mergeGeoMissing(g, model.GeoInfo{ASN: asnMeta.ASN, Org: asnMeta.Org, ISP: asnMeta.ISP})
	}
	if r.cityDB != nil {
		g = mergeGeoMissing(g, lookupMMDBReader(r.cityDB, ip))
	}
	if r.cnDB != nil && (g.Country == "" || g.Country == "CN") {
		if cn, ok := lookupGeoCNReader(r.cnDB, parsed); ok {
			g.Country = "CN"
			if cn.Region != "" {
				g.Region = cn.Region
			}
			if cn.City != "" {
				g.City = cn.City
			}
			if cn.ISP != "" {
				g.ISP = cn.ISP
			}
		}
	}
	return g
}
