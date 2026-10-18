package main

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplicitHostDirectNeverBecomesSlotFallback(t *testing.T) {
	for _, status := range []string{"up", "waiting", "failed", "stopped"} {
		for _, present := range []bool{true, false} {
			tunnels := []*Tunnel{}
			if present {
				tunnels = append(tunnels, &Tunnel{Slot: 1, Port: 17928, Status: status})
			}
			inbounds := []*nativeInbound{
				{Port: 30000, Protocol: "vless", Enable: true, HostDirect: true},
				{Port: 30001, Protocol: "vless", Enable: true, BoundSlot: 1},
				{Port: 30002, Protocol: "vless", Enable: true},
				// Ambiguous/corrupt direct plus Slot must not grant host access.
				{Port: 30003, Protocol: "vless", Enable: true, BoundSlot: 1, HostDirect: true},
			}
			cfg := buildXrayConfig(inbounds, tunnels)
			outs := cfg["outbounds"].([]any)
			if outs[0].(map[string]any)["protocol"] != "blackhole" {
				t.Fatal("default must block")
			}
			rules := cfg["routing"].(map[string]any)["rules"].([]any)
			for i, rule := range rules {
				tag := rule.(map[string]any)["outboundTag"]
				if (i == 0) != (tag == "host-direct") {
					t.Fatal("direct leaked to another entry", status, present, i, tag)
				}
			}
		}
	}
}

func TestPublicSocksUsesCurrentCredentialAndBlockedSlot(t *testing.T) {
	n, runtime, ts := rollbackFixture(t)
	if err := n.setPublicSocks(ts[0], true, ts); err != nil {
		t.Fatal(err)
	}
	exposure := n.socksExposures()[1]
	if exposure == nil || !exposure.Enable || exposure.Port == ts[0].Port {
		t.Fatal("separate listener missing")
	}
	starts := runtime.starts
	if err := n.setPublicSocks(ts[0], true, ts); err != nil || runtime.starts != starts {
		t.Fatal("idempotent enable restarted")
	}
	ts[0].setCredential(SocksCred{User: "changed-user", Pass: "changed-pass"})
	ts[0].Status = "failed"
	cfg := buildXrayConfig(n.store.Inbounds, ts)
	var public map[string]any
	for _, raw := range cfg["inbounds"].([]any) {
		ib := raw.(map[string]any)
		if ib["protocol"] == "socks" {
			public = ib
		}
	}
	settings := public["settings"].(map[string]any)
	account := settings["accounts"].([]any)[0].(map[string]any)
	if settings["auth"] != "password" || settings["udp"] != false || account["user"] != "changed-user" || account["pass"] != "changed-pass" {
		t.Fatal("public credential/auth not synchronized")
	}
	for _, raw := range cfg["outbounds"].([]any) {
		out := raw.(map[string]any)
		if out["tag"] == "slot-01" && out["protocol"] != "blackhole" {
			t.Fatal("failed Slot not blocked")
		}
	}
	list, err := n.Inbounds(nil)
	if err != nil || len(list) != 1 {
		t.Fatal("managed SOCKS leaked into normal subscription entries")
	}
	runtime.reject = true
	if err := n.setPublicSocks(ts[0], false, ts); err == nil || !n.socksExposures()[1].Enable {
		t.Fatal("rejected toggle lost accepted configuration")
	}
	runtime.reject = false
	if err := n.setPublicSocks(ts[0], false, ts); err != nil {
		t.Fatal(err)
	}
	if n.socksExposures()[1].Enable || n.socksExposures()[1].Port != exposure.Port {
		t.Fatal("disable lost reserved identity")
	}
	stored, err := loadNativeStore(n.dir)
	if err != nil || stored.Inbounds[1].Enable || stored.Inbounds[1].SocksSlot != 1 {
		t.Fatal("external access not persisted", err)
	}
}

func TestHostDirectSubscriptionScopesAndPersistence(t *testing.T) {
	n, runtime, ts := rollbackFixture(t)
	direct := *n.store.Inbounds[0]
	direct.ID, direct.Port, direct.HostDirect, direct.BoundTo = 2, 30000, true, ""
	n.store.NextID = 3
	n.store.Inbounds = append(n.store.Inbounds, &direct)
	usePanel(t, n)
	m := NewManager(32, t.TempDir())
	m.tunnels[1] = ts[0]
	if err := n.setHostDirect(true, "unused.invalid", ts); err != nil || runtime.starts != 0 {
		t.Fatal("existing direct unexpectedly regenerated")
	}
	for _, tc := range []struct {
		slots  map[int]bool
		direct bool
		want   int
	}{
		{nil, true, 2}, {nil, false, 1}, {map[int]bool{1: true}, false, 1}, {map[int]bool{}, true, 1},
	} {
		links, err := subLinksScoped(m, "fixture.invalid", true, tc.slots, tc.direct)
		if err != nil || len(links) != tc.want {
			t.Fatal("scope incorrect", err, len(links), tc.want)
		}
		for _, link := range links {
			u, _ := url.Parse(link)
			if u.Port() == "30000" && u.Fragment != "VPS 本机直出" {
				t.Fatal("direct was mislabeled")
			}
		}
	}
	useSettings(t)
	token, err := subToken()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		query string
		want  int
	}{{"", 2}, {"&direct=0", 1}, {"&vpn=0&direct=1", 1}, {"&direct=bad", 400}} {
		r := httptest.NewRequest("GET", "/sub?target=links&token="+token+tc.query, nil)
		w := httptest.NewRecorder()
		handleSub(m)(w, r)
		if tc.want == 400 {
			if w.Code != 400 {
				t.Fatal("bad scope accepted")
			}
			continue
		}
		if w.Code != 200 || len(strings.Split(w.Body.String(), "\n")) != tc.want {
			t.Fatal("HTTP scope failed", w.Code)
		}
	}
	if err := n.setHostDirect(false, "unused.invalid", ts); err != nil {
		t.Fatal(err)
	}
	st, err := loadNativeStore(n.dir)
	if err != nil || st.Inbounds[1].Enable || !st.Inbounds[1].HostDirect {
		t.Fatal("direct intent lost")
	}
	before, _ := os.ReadFile(filepath.Join(n.dir, "native.json"))
	if err := n.setHostDirect(true, "unused.invalid", ts); err != nil {
		t.Fatal(err)
	}
	if n.store.Inbounds[1].Clients[0].ID != direct.Clients[0].ID || n.store.Inbounds[1].Port != 30000 {
		t.Fatal("direct toggle rotated clients")
	}
	var preserved nativeStore
	if json.Unmarshal(before, &preserved) != nil {
		t.Fatal("invalid persisted state")
	}
}

func TestAccessStoreRejectsAmbiguousDirectAndUnauthenticatedSocks(t *testing.T) {
	n, _, _ := rollbackFixture(t)
	n.store.Inbounds[0].HostDirect = true
	if validateNativeStore(n.store) == nil {
		t.Fatal("ambiguous direct accepted")
	}
	n.store.Inbounds[0].HostDirect = false
	n.store.Inbounds[0].SocksSlot = 1
	if validateNativeStore(n.store) == nil {
		t.Fatal("unrestricted SOCKS accepted")
	}
}
