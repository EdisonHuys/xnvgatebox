package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

type thirdPartyJob struct {
	cancel context.CancelFunc
	digest string
}
type ThirdPartyManager struct {
	mu      sync.Mutex
	c       *Console
	records map[int]ThirdPartyRecord
	jobs    map[int]*thirdPartyJob
	wg      sync.WaitGroup
	closing bool
	probe   func(context.Context, ThirdPartyProxy, string) ThirdPartyRecord
}

func (c *Console) loadThirdPartyManager() {
	s := &ThirdPartyManager{c: c, records: map[int]ThirdPartyRecord{}, jobs: map[int]*thirdPartyJob{}, probe: probeThirdParty}
	if b, err := os.ReadFile(filepath.Join(c.m.workDir, "third-party-checks.json")); err == nil {
		_ = json.Unmarshal(b, &s.records)
	}
	if s.records == nil {
		s.records = map[int]ThirdPartyRecord{}
	}
	c.third = s
}

func (s *ThirdPartyManager) snapshot() map[int]ThirdPartyRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[int]ThirdPartyRecord{}
	for id, r := range s.records {
		out[id] = r
	}
	for id, j := range s.jobs {
		out[id] = ThirdPartyRecord{Status: "running", Digest: j.digest, CheckedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour), IPType: "unknown"}
	}
	return out
}

func (s *ThirdPartyManager) shutdown() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.closing = true
	for _, j := range s.jobs {
		j.cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *ThirdPartyManager) cancel(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j := s.jobs[id]; j != nil {
		j.cancel()
		delete(s.jobs, id)
	}
}

func (s *ThirdPartyManager) start(n *Native, id int) error {
	ib, err := n.thirdPartySnapshot(id)
	if err != nil {
		return err
	}
	p := *ib.ThirdParty
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing || s.c.ctx.Err() != nil {
		return fmt.Errorf("管理服务正在停止")
	}
	if s.jobs[id] != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(s.c.ctx, 100*time.Second)
	j := &thirdPartyJob{cancel: cancel, digest: thirdPartyDigest(p)}
	s.jobs[id] = j
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer cancel()
		r := s.probe(ctx, p, s.c.m.workDir)
		if ctx.Err() != nil {
			r = ThirdPartyRecord{Status: "failed", Code: "check_cancelled", IPType: "unknown", CheckedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
		}
		r.Digest = j.digest
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.jobs[id] != j {
			return
		}
		delete(s.jobs, id)
		current, err := n.thirdPartySnapshot(id)
		if err != nil || thirdPartyDigest(*current.ThirdParty) != j.digest || s.closing {
			return
		}
		s.records[id] = r
		b, err := json.Marshal(s.records)
		if err == nil {
			err = atomicConfigWrite(filepath.Join(s.c.m.workDir, "third-party-checks.json"), b)
		}
		if err != nil {
			r.Status, r.Code = "failed", "result_save_failed"
			s.records[id] = r
		}
	}()
	return nil
}

func (n *Native) thirdPartyRows(records map[int]ThirdPartyRecord) []map[string]any {
	n.mu.Lock()
	defer n.mu.Unlock()
	rows := []map[string]any{}
	for _, ib := range n.store.Inbounds {
		if !thirdPartyBound(ib) {
			continue
		}
		p := *ib.ThirdParty
		r := thirdPartyRecord(p, records[ib.ID])
		rows = append(rows, map[string]any{"id": ib.ID, "host": p.Host, "port": p.Port, "country": p.Country, "city": p.City, "remark": p.Remark, "enabled": ib.Enable, "subscribe": p.Subscribe, "entry_port": ib.Port, "name": thirdPartyName(ib.ID, p, records[ib.ID]), "check": r, "country_mismatch": r.Country != "" && p.Country != "" && r.Country != p.Country, "city_mismatch": r.City != "" && p.City != "" && r.City != p.City})
	}
	return rows
}

func thirdPartyNative(w http.ResponseWriter) *Native {
	p, err := openPanel()
	if err == nil {
		if n, ok := p.(*Native); ok {
			return n
		}
	}
	writeJSON(w, 409, map[string]string{"error": "第三方代理仅用于内置模式"})
	return nil
}

func (s *ThirdPartyManager) routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/resources/third-party", s.apiResources)
	mux.HandleFunc("/api/resources/third-party/check", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var req struct {
			ID int `json:"id"`
		}
		if err := readConsoleJSON(w, r, &req); err != nil {
			writeJSON(w, 400, map[string]string{"error": "需要代理编号"})
			return
		}
		n := thirdPartyNative(w)
		if n == nil {
			return
		}
		if err := s.start(n, req.ID); err != nil {
			writeJSON(w, 409, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 202, map[string]any{"id": req.ID, "status": "running"})
	})
	for _, action := range []string{"delete", "toggle", "subscribe"} {
		action := action
		mux.HandleFunc("/api/resources/third-party/"+action, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			var req struct {
				ID        int  `json:"id"`
				Enabled   bool `json:"enabled"`
				Subscribe bool `json:"subscribe"`
			}
			if err := readConsoleJSON(w, r, &req); err != nil {
				writeJSON(w, 400, map[string]string{"error": "请求格式无效"})
				return
			}
			n := thirdPartyNative(w)
			if n == nil {
				return
			}
			_, err := n.thirdPartySnapshot(req.ID)
			if err != nil {
				writeJSON(w, 404, map[string]string{"error": err.Error()})
				return
			}
			if action == "delete" {
				err = n.DeleteInbounds([]int{req.ID}, s.c.m.Tunnels())
			} else {
				value := req.Enabled
				if action == "subscribe" {
					value = req.Subscribe
				}
				err = n.setThirdPartyFlag(req.ID, action, value, s.c.m.Tunnels())
			}
			if err != nil {
				writeJSON(w, 409, map[string]string{"error": "配置未生效，已保留原设置"})
				return
			}
			if action != "subscribe" {
				s.cancel(req.ID)
			}
			invalidateInbounds()
			writeJSON(w, 200, map[string]bool{"ok": true})
		})
	}
}

func (s *ThirdPartyManager) apiResources(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	n := thirdPartyNative(w)
	if n == nil {
		return
	}
	if r.Method == http.MethodGet {
		if text := r.URL.Query().Get("id"); text != "" {
			id, err := strconv.Atoi(text)
			if err != nil {
				writeJSON(w, 400, map[string]string{"error": "代理编号无效"})
				return
			}
			ib, err := n.thirdPartySnapshot(id)
			if err != nil {
				writeJSON(w, 404, map[string]string{"error": err.Error()})
				return
			}
			p := *ib.ThirdParty
			p.Password = ""
			writeJSON(w, 200, map[string]any{"id": id, "proxy": p, "enabled": ib.Enable, "password_saved": true})
			return
		}
		codes := make([]string, 0, len(countryCN))
		for code := range countryCN {
			codes = append(codes, code)
		}
		sort.Strings(codes)
		countries := make([]map[string]string, 0, len(codes))
		for _, code := range codes {
			countries = append(countries, map[string]string{"code": code, "name": countryLabel(code, "")})
		}
		writeJSON(w, 200, map[string]any{"proxies": n.thirdPartyRows(s.snapshot()), "countries": countries})
		return
	}
	var req struct {
		ID int `json:"id"`
		ThirdPartyProxy
		Enabled bool `json:"enabled"`
	}
	if err := readConsoleJSON(w, r, &req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "代理字段格式无效"})
		return
	}
	if req.ID < 0 {
		writeJSON(w, 400, map[string]string{"error": "代理编号无效"})
		return
	}
	if req.ID != 0 && req.Password == "" {
		ib, err := n.thirdPartySnapshot(req.ID)
		if err != nil {
			writeJSON(w, 404, map[string]string{"error": err.Error()})
			return
		}
		req.Password = ib.ThirdParty.Password
	}
	p := cleanThirdParty(req.ThirdPartyProxy)
	if err := validateThirdParty(p); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	host := "www.tesla.com"
	s.c.targetMu.Lock()
	if s.c.plan != nil {
		host = s.c.plan.ServerName
	}
	s.c.targetMu.Unlock()
	id, err := n.saveThirdParty(req.ID, p, req.Enabled, host, s.c.m.Tunnels())
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": err.Error()})
		return
	}
	s.cancel(id)
	invalidateInbounds()
	writeJSON(w, 200, map[string]any{"id": id, "ok": true})
}
