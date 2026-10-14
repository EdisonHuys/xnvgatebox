package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestXrayExportRequiresLoginAndPreservesStoppedSlot(t *testing.T) {
	m := NewManager(3, t.TempDir())
	m.tunnels[2] = &Tunnel{Slot: 2, Port: 17929, Status: "failed"}
	a, _, err := NewAuth(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	a.Wrap(apiExportXray(m)).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/export/xray", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated export: %d", w.Code)
	}
	w = httptest.NewRecorder()
	apiExportXray(m).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/export/xray", nil))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Result())
	}
	var cfg struct {
		Outbounds []struct {
			Tag      string
			Protocol string
		}
		Routing struct {
			Rules []struct{ OutboundTag string }
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Outbounds) != 2 || cfg.Outbounds[1].Tag != "slot-02" || cfg.Outbounds[1].Protocol != "blackhole" || cfg.Routing.Rules[0].OutboundTag != "slot-02" {
		t.Fatalf("stopped slot lost: %s", w.Body.String())
	}
}
