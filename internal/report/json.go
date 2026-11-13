// Package report 输出结构化 JSON 测试结果。
package report

import (
	"encoding/json"
	"os"
	"time"

	"nekospeed/internal/model"
)

// RunMeta 保存渲染图和复原重绘所需的任务级元数据。
type RunMeta struct {
	Version   string
	StartTime time.Time
	Elapsed   time.Duration
	SubName   string
	Threads   int
	Single    bool
	PingCount int
	Filter    string
	LocalISP  string
	Bandwidth string
	SortBy    string
	Watermark string
	Theme     string
}

type document struct {
	Version   string          `json:"version"`
	StartTime time.Time       `json:"startTime"`
	Elapsed   string          `json:"elapsed"`
	SubName   string          `json:"subName,omitempty"`
	Threads   int             `json:"threads,omitempty"`
	Single    *bool           `json:"single,omitempty"`
	PingCount int             `json:"pingCount,omitempty"`
	Filter    string          `json:"filter,omitempty"`
	LocalISP  string          `json:"localISP,omitempty"`
	Bandwidth string          `json:"bandwidth,omitempty"`
	SortBy    string          `json:"sortBy,omitempty"`
	Watermark string          `json:"watermark,omitempty"`
	Theme     string          `json:"theme,omitempty"`
	Total     int             `json:"total"`
	Succeeded int             `json:"succeeded"`
	Nodes     []nodeDoc       `json:"nodes"`
	Topology  *model.Topology `json:"topology,omitempty"`
}

type nodeDoc struct {
	Index    int                  `json:"index"`
	Name     string               `json:"name"`
	Type     string               `json:"type"`
	Server   string               `json:"server"`
	Error    string               `json:"error,omitempty"`
	Latency  *model.LatencyResult `json:"latency,omitempty"`
	Speed    *model.SpeedResult   `json:"speed,omitempty"`
	NAT      string               `json:"nat,omitempty"`
	Media    []model.UnlockResult `json:"media,omitempty"`
	AI       []model.UnlockResult `json:"ai,omitempty"`
	Inbound  *model.GeoInfo       `json:"inbound,omitempty"`
	Outbound *model.GeoInfo       `json:"outbound,omitempty"`
}

// SaveJSON 将结果写入 path（缩进 JSON）。
func SaveJSON(results []*model.NodeResult, topo *model.Topology, version string, start time.Time, elapsed time.Duration, path string) error {
	return saveDocument(results, topo, document{
		Version:   version,
		StartTime: start,
		Elapsed:   elapsed.String(),
		Total:     len(results),
		Topology:  topo,
	}, path)
}

// SaveJSONWithMeta 将结果连同任务级渲染元数据一起写入 path（供重绘图片精确还原页眉页脚）。
func SaveJSONWithMeta(results []*model.NodeResult, topo *model.Topology, meta RunMeta, path string) error {
	single := meta.Single
	return saveDocument(results, topo, document{
		Version:   meta.Version,
		StartTime: meta.StartTime,
		Elapsed:   meta.Elapsed.String(),
		SubName:   meta.SubName,
		Threads:   meta.Threads,
		Single:    &single,
		PingCount: meta.PingCount,
		Filter:    meta.Filter,
		LocalISP:  meta.LocalISP,
		Bandwidth: meta.Bandwidth,
		SortBy:    meta.SortBy,
		Watermark: meta.Watermark,
		Theme:     meta.Theme,
		Total:     len(results),
		Topology:  topo,
	}, path)
}

func saveDocument(results []*model.NodeResult, _ *model.Topology, doc document, path string) error {
	for _, r := range results {
		if r.OK() {
			doc.Succeeded++
		}
		doc.Nodes = append(doc.Nodes, nodeDoc{
			Index:    r.Node.Index,
			Name:     r.Node.Name,
			Type:     r.Node.Type,
			Server:   r.Node.Server,
			Error:    r.Err,
			Latency:  r.Latency,
			Speed:    r.Speed,
			NAT:      r.NAT,
			Media:    r.Media,
			AI:       r.AI,
			Inbound:  r.Inbound,
			Outbound: r.Outbound,
		})
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
