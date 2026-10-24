package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"time"
)

const poolMirrorProvider = "GeorgeXie2333/vpngate-list-mirror"
var poolDataCommitPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)

type poolSourceOptions struct {
	MirrorEnabled bool `json:"mirror_enabled"`
	SourceTimeout int `json:"source_timeout_seconds"`
	MirrorMaxAgeHours int `json:"mirror_max_age_hours"`
}

type poolMirrorOptions struct {
	Enabled bool `json:"enabled"`
	MaxAgeHours int `json:"max_age_hours"`
}

func validSourceOptions(s poolSourceOptions) bool {
	return s.SourceTimeout >= 2 && s.SourceTimeout <= 20 && s.MirrorMaxAgeHours >= 1 && s.MirrorMaxAgeHours <= 168
}

func readSourceOptions(path string) (poolSourceOptions, map[string]json.RawMessage, error) {
	s := poolSourceOptions{MirrorEnabled: true, SourceTimeout: 10, MirrorMaxAgeHours: 24}
	blob, err := os.ReadFile(path)
	var fields map[string]json.RawMessage
	if err != nil || len(blob) > 1<<20 || json.Unmarshal(blob, &fields) != nil || fields == nil {
		return s, nil, fmt.Errorf("来源配置读取失败，保留原文件")
	}
	if raw, ok := fields["pool_source_timeout_seconds"]; ok && json.Unmarshal(raw, &s.SourceTimeout) != nil {
		return s, nil, fmt.Errorf("来源等待设置无效")
	}
	if raw, ok := fields["mirror_fallback"]; ok {
		m := poolMirrorOptions{MaxAgeHours: 24}
		if string(raw) == "null" || json.Unmarshal(raw, &m) != nil {
			return s, nil, fmt.Errorf("镜像设置无效")
		}
		s.MirrorEnabled, s.MirrorMaxAgeHours = m.Enabled, m.MaxAgeHours
	}
	if !validSourceOptions(s) {
		return s, nil, fmt.Errorf("来源等待应为 2–20 秒，镜像最大年龄应为 1–168 小时")
	}
	return s, fields, nil
}

func (c *Console) apiSourceSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	var req struct {
		MirrorEnabled *bool `json:"mirror_enabled"`
		SourceTimeout *int `json:"source_timeout_seconds"`
		MirrorMaxAgeHours *int `json:"mirror_max_age_hours"`
	}
	if r.Method == http.MethodPost && (readConsoleJSON(w, r, &req) != nil || req.MirrorEnabled == nil || req.SourceTimeout == nil || req.MirrorMaxAgeHours == nil) {
		writeJSON(w, 400, map[string]string{"error": "需要明确设置来源等待与镜像开关/年龄"})
		return
	}
	c.sourceMu.Lock()
	defer c.sourceMu.Unlock()
	if c.ctx.Err() != nil {
		w.WriteHeader(503)
		return
	}
	path := envOr("FANOUT_POOL_CONFIG", "/etc/xnvgatebox/pool.json")
	s, fields, err := readSourceOptions(path)
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": err.Error()})
		return
	}
	if r.Method == http.MethodPost {
		s = poolSourceOptions{*req.MirrorEnabled, *req.SourceTimeout, *req.MirrorMaxAgeHours}
		if !validSourceOptions(s) {
			writeJSON(w, 400, map[string]string{"error": "来源等待应为 2–20 秒，镜像最大年龄应为 1–168 小时"})
			return
		}
		fields["pool_source_timeout_seconds"], _ = json.Marshal(s.SourceTimeout)
		mirrorFields := map[string]json.RawMessage{}
		if raw, ok := fields["mirror_fallback"]; ok {
			_ = json.Unmarshal(raw, &mirrorFields)
		}
		mirrorFields["enabled"], _ = json.Marshal(s.MirrorEnabled)
		mirrorFields["max_age_hours"], _ = json.Marshal(s.MirrorMaxAgeHours)
		fields["mirror_fallback"], _ = json.Marshal(mirrorFields)
		blob, _ := json.MarshalIndent(fields, "", "  ")
		if atomicConfigWrite(path, append(blob, '\n')) != nil {
			writeJSON(w, 409, map[string]string{"error": "来源设置保存失败"})
			return
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, s)
}

type poolSourceMetadata struct {
	Kind string `json:"kind"`
	Retrieval string `json:"retrieval"`
	RetrievedAt string `json:"retrieved_at"`
	MirrorProvider string `json:"mirror_provider"`
	SnapshotFetchedAt string `json:"snapshot_fetched_at"`
	DataCommit string `json:"data_commit"`
	IntegrityVerified bool `json:"integrity_verified"`
	RejectedRows int `json:"rejected_rows"`
}

type poolSourceView struct {
	Kind string `json:"kind"`
	Label string `json:"label"`
	RetrievedAt string `json:"retrieved_at"`
	SnapshotFetchedAt string `json:"snapshot_fetched_at"`
	DataCommit string `json:"data_commit"`
	Stale bool `json:"stale"`
	IntegrityVerified bool `json:"integrity_verified"`
}

func publicPoolSource(s poolSourceMetadata) poolSourceView {
	v := poolSourceView{Kind: "legacy", Label: "既有清单"}
	if s.Retrieval != "local_builder" {
		return v
	}
	if t, err := time.Parse(time.RFC3339Nano, s.RetrievedAt); err == nil {
		v.RetrievedAt = t.UTC().Format(time.RFC3339)
	}
	switch s.Kind {
	case "official":
		v.Kind, v.Label = "official", "官方来源 · 本 VPS 获取"
	case "official_backup":
		v.Kind, v.Label = "official_backup", "VPN Gate 官方备用 · 本 VPS 获取"
	case "configured":
		v.Kind, v.Label = "configured", "配置来源 · 本 VPS 获取"
	case "local_csv":
		v.Kind, v.Label = "local_csv", "本机 CSV 导入"
	case "mirror":
		if s.MirrorProvider != poolMirrorProvider {
			return v
		}
		v.Kind, v.Label = "mirror", "第三方镜像 GeorgeXie2333 · 本 VPS 获取"
		if t, err := time.Parse(time.RFC3339Nano, s.SnapshotFetchedAt); err == nil {
			v.SnapshotFetchedAt = t.UTC().Format(time.RFC3339)
			v.Stale = time.Since(t) > 3*time.Hour
			if poolDataCommitPattern.MatchString(s.DataCommit) && time.Until(t) <= 5*time.Minute {
				v.DataCommit, v.IntegrityVerified = s.DataCommit, s.IntegrityVerified
			}
		}
	}
	return v
}
