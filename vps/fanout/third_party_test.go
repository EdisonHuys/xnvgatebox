package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestThirdPartySubscriptionSourceOrderStable(t *testing.T) {
	n, _, m, _ := thirdPartyFixture(t)
	useSettings(t)
	vpn, third := n.store.Inbounds[0], n.store.Inbounds[1]
	extraThird := cloneNativeStore(n.store).Inbounds[1]
	extraThird.ID, extraThird.Port, extraThird.ThirdParty.Remark = 4, 30004, "second supplier"
	direct := cloneNativeStore(n.store).Inbounds[1]
	direct.ID, direct.Port, direct.ThirdParty, direct.HostDirect = 5, 30005, nil, true
	direct2 := cloneNativeStore(n.store).Inbounds[1]
	direct2.ID, direct2.Port, direct2.ThirdParty, direct2.HostDirect = 6, 30006, nil, true
	// Deliberately scramble storage order; neither results nor API order is a
	// ranking. Reads must leave the private store and client identities intact.
	n.store.Inbounds = []*nativeInbound{extraThird, direct2, vpn, direct, third}
	n.store.NextID = 7
	if err := n.store.save(n.dir); err != nil {
		t.Fatal(err)
	}
	invalidateInbounds()
	before, _ := json.Marshal(n.store)
	token, _ := subToken()
	for _, tc := range []struct {
		query string
		ports []string
	}{
		{"", []string{"30005", "30006", "30002", "30004", "30001"}},
		{"&direct=0", []string{"30002", "30004", "30001"}},
		{"&vpn=0&third=1&proxies=4,2", []string{"30005", "30006", "30002", "30004"}},
		{"&third=0", []string{"30005", "30006", "30001"}},
		{"&slots=1", []string{"30001"}},
	} {
		var plain string
		for _, target := range []string{"links", "base64"} {
			w := httptest.NewRecorder()
			handleSub(m)(w, httptest.NewRequest("GET", "/sub?token="+token+"&target="+target+tc.query, nil))
			if w.Code != 200 {
				t.Fatal(tc.query, w.Code)
			}
			body := w.Body.String()
			if target == "base64" {
				b, err := base64.StdEncoding.DecodeString(body)
				if err != nil {
					t.Fatal(err)
				}
				body = string(b)
				if body != plain {
					t.Fatal("format changed ordering")
				}
			} else {
				plain = body
			}
			var ports []string
			for _, link := range strings.Split(body, "\n") {
				u, err := url.Parse(link)
				if err != nil {
					t.Fatal(err)
				}
				ports = append(ports, u.Port())
			}
			if !reflect.DeepEqual(ports, tc.ports) {
				t.Fatalf("scope %s: got %v want %v", tc.query, ports, tc.ports)
			}
		}
	}
	after, _ := json.Marshal(n.store)
	if string(before) != string(after) {
		t.Fatal("subscription sorting mutated private configuration")
	}
}

func thirdPartyFixture(t *testing.T) (*Native, *faultNativeRuntime, *Manager, *Console) {
	t.Helper()
	n, run, ts := rollbackFixture(t)
	p := ThirdPartyProxy{Host: "proxy.example.invalid", Port: 1080, Username: "supplier-user", Password: "supplier-secret:@/", Country: "US", City: "洛杉矶", Remark: "nov proxy-los", Subscribe: true}
	ib := *n.store.Inbounds[0]
	ib.ID, ib.Port, ib.BoundTo, ib.Security, ib.Remark = 2, 30002, "", "reality", "第三方"
	ib.ThirdParty = &p
	ib.Reality = &realityConfig{ServerNames: []string{"example.com"}, PublicKey: "fixture-public", PrivateKey: "fixture-private", ShortIDs: []string{"abcd"}}
	n.store.NextID = 3
	n.store.Inbounds = append(n.store.Inbounds, &ib)
	if err := n.store.save(n.dir); err != nil {
		t.Fatal(err)
	}
	usePanel(t, n)
	m := NewManager(32, n.dir)
	m.tunnels[1] = ts[0]
	c, err := newConsole(m)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.shutdown)
	return n, run, m, c
}

func TestThirdPartyCredentialsAndAmbiguousBindingsRejected(t *testing.T) {
	p := ThirdPartyProxy{Host: "proxy.example.com", Port: 1080, Username: "u:@", Password: "p:@/", Subscribe: true}
	if err := validateThirdParty(p); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ThirdPartyProxy){func(p *ThirdPartyProxy) { p.Host = "socks5://host" }, func(p *ThirdPartyProxy) { p.Host = "host/path" }, func(p *ThirdPartyProxy) { p.Port = 0 }, func(p *ThirdPartyProxy) { p.Username = "" }, func(p *ThirdPartyProxy) { p.Password = "x\nsecret" }, func(p *ThirdPartyProxy) { p.Country = "美国" }, func(p *ThirdPartyProxy) { p.Remark = strings.Repeat("a", 161) }} {
		bad := p
		mutate(&bad)
		if validateThirdParty(bad) == nil {
			t.Fatal("invalid proxy accepted")
		}
	}
	n, _, _, _ := thirdPartyFixture(t)
	for _, mutate := range []func(*nativeInbound){func(ib *nativeInbound) { ib.HostDirect = true }, func(ib *nativeInbound) { ib.BoundSlot = 1 }, func(ib *nativeInbound) { ib.BoundTo = "a" }, func(ib *nativeInbound) { ib.SocksSlot = 1 }, func(ib *nativeInbound) { ib.Security = "none" }} {
		st := cloneNativeStore(n.store)
		mutate(st.Inbounds[1])
		if validateNativeStore(st) == nil {
			t.Fatal("ambiguous third-party route accepted")
		}
	}
}

func TestThirdPartyRoutingNeverFallsBackToDirectOrVPN(t *testing.T) {
	n, _, m, _ := thirdPartyFixture(t)
	for _, ts := range [][]*Tunnel{nil, m.Tunnels(), {{Slot: 1, Port: 17928, Status: "failed"}}} {
		cfg := buildXrayConfig(n.store.Inbounds, ts)
		outs := cfg["outbounds"].([]any)
		if outs[0].(map[string]any)["protocol"] != "blackhole" {
			t.Fatal("default route is not blocked")
		}
		rules := cfg["routing"].(map[string]any)["rules"].([]any)
		if rules[1].(map[string]any)["outboundTag"] != thirdPartyTag(2) {
			t.Fatal("third-party route depends on VPN")
		}
		for _, raw := range outs {
			o := raw.(map[string]any)
			if o["tag"] == thirdPartyTag(2) && o["protocol"] != "socks" {
				t.Fatal("wrong protocol")
			}
		}
	}
	bad := cloneNativeStore(n.store)
	bad.Inbounds[1].HostDirect = true
	rules := buildXrayConfig(bad.Inbounds, nil)["routing"].(map[string]any)["rules"].([]any)
	if rules[1].(map[string]any)["outboundTag"] != "block" {
		t.Fatal("corrupt route leaked")
	}
	n.store.Inbounds[1].Enable = false
	if len(buildXrayConfig(n.store.Inbounds, nil)["inbounds"].([]any)) != 1 {
		t.Fatal("stopped listener remains")
	}
}

func TestThirdPartyEditTransactionAndRestartPreserveIdentity(t *testing.T) {
	n, run, m, _ := thirdPartyFixture(t)
	original, _ := n.thirdPartySnapshot(2)
	p := *original.ThirdParty
	p.Remark = "新备注"
	p.Host = "new.example.invalid"
	if _, err := n.saveThirdParty(2, p, true, "", m.Tunnels()); err != nil {
		t.Fatal(err)
	}
	current, _ := n.thirdPartySnapshot(2)
	if current.Port != original.Port || current.Clients[0].ID != original.Clients[0].ID || current.Reality.PrivateKey != original.Reality.PrivateKey {
		t.Fatal("editing rotated identity")
	}
	run.reject = true
	p.Password = "rejected-secret"
	if _, err := n.saveThirdParty(2, p, false, "", m.Tunnels()); err == nil {
		t.Fatal("validator failure ignored")
	}
	run.reject = false
	st, err := loadNativeStore(n.dir)
	if err != nil || st.Inbounds[1].ThirdParty.Password != original.ThirdParty.Password || !st.Inbounds[1].Enable {
		t.Fatal("failed candidate changed persisted secret/intent")
	}
	if st.Inbounds[1].ThirdParty.Remark != "新备注" {
		t.Fatal("accepted remark missing")
	}
	if err := n.DeleteInbounds([]int{2}, m.Tunnels()); err != nil {
		t.Fatal(err)
	}
	if _, err := n.thirdPartySnapshot(2); err == nil {
		t.Fatal("deleted resource remains")
	}
}

func TestThirdPartyNameSeparatesManualMeasuredAndExpired(t *testing.T) {
	n, _, _, _ := thirdPartyFixture(t)
	p := *n.store.Inbounds[1].ThirdParty
	manual := thirdPartyName(2, p, ThirdPartyRecord{})
	if !strings.Contains(manual, "手动标注") || !strings.Contains(manual, "nov proxy-los") {
		t.Fatal("manual or remark lost")
	}
	r := ThirdPartyRecord{Status: "passed", Digest: thirdPartyDigest(p), ActualIP: "8.8.8.8", Country: "JP", City: "东京", IPType: "unknown", CheckedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	name := thirdPartyName(2, p, r)
	if !strings.Contains(name, "日本 · 东京 · nov proxy-los") || strings.Contains(name, "手动标注") || strings.Contains(name, "洛杉矶") {
		t.Fatal("mixed manual and measured geography", name)
	}
	r.ExpiresAt = time.Now().Add(-time.Second)
	if !strings.Contains(thirdPartyName(2, p, r), "手动标注") {
		t.Fatal("expired geography treated as measured")
	}
	p.Remark = ""
	if !strings.Contains(thirdPartyName(2, p, r), "第三方 · 02") {
		t.Fatal("default label missing")
	}
	p.Host = "different.example.invalid"
	if thirdPartyRecord(p, r).Status != "not_tested" {
		t.Fatal("changed endpoint inherits evidence")
	}
}

func TestThirdPartySubscriptionScopesAndSecrets(t *testing.T) {
	n, _, m, c := thirdPartyFixture(t)
	useSettings(t)
	token, _ := subToken()
	for _, tc := range []struct {
		q      string
		want   int
		status int
	}{
		{"", 2, 200}, {"&third=0", 1, 200}, {"&slots=1", 1, 200}, {"&slots=1&third=1", 2, 200}, {"&vpn=0&direct=0&third=1", 1, 200}, {"&vpn=0&direct=0&proxies=2", 1, 200}, {"&vpn=0&proxies=999", 0, 404}, {"&third=0&proxies=2", 0, 400}, {"&proxies=2,2", 0, 400}, {"&proxies=-1", 0, 400}} {
		w := httptest.NewRecorder()
		handleSub(m)(w, httptest.NewRequest("GET", "/sub?target=links&token="+token+tc.q, nil))
		if w.Code != tc.status {
			t.Fatal("unexpected scope response", tc.q, w.Code)
		}
		if w.Code != 200 {
			continue
		}
		links := strings.Split(w.Body.String(), "\n")
		if len(links) != tc.want {
			t.Fatal("scope count", tc.q, len(links))
		}
		if strings.Contains(w.Body.String(), n.store.Inbounds[1].ThirdParty.Password) || strings.Contains(w.Body.String(), "supplier-user") || strings.Contains(w.Body.String(), "proxy.example.invalid") {
			t.Fatal("supplier credentials leaked into subscription")
		}
		for _, link := range links {
			u, _ := url.Parse(link)
			if u.Port() == "30002" && !strings.Contains(u.Fragment, "nov proxy-los") {
				t.Fatal("custom remark lost")
			}
			if u.Port() == "30001" && !strings.Contains(u.Fragment, "VPN Gate") {
				t.Fatal("VPN source missing")
			}
		}
	}
	mux := http.NewServeMux()
	c.routes(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/console/qr?vpn=0&direct=0&proxies=2", nil))
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
		t.Fatal("third-party QR failed")
	}
	n.store.Inbounds[1].ThirdParty.Subscribe = false
	links, err := subLinksSources(m, "fixture.invalid", true, map[int]bool{}, false, true, nil)
	if err != nil || len(links) != 0 {
		t.Fatal("subscription opt-out ignored")
	}
}

func TestThirdPartyPrivateAPIAndNoPasswordEcho(t *testing.T) {
	n, _, _, c := thirdPartyFixture(t)
	mux := http.NewServeMux()
	c.third.routes(mux)
	a := newTestAuth()
	handler := a.Wrap(mux)
	for _, path := range []string{"/api/resources/third-party", "/api/resources/third-party?id=2", "/api/resources/third-party/check", "/api/resources/third-party/delete", "/api/resources/third-party/toggle", "/api/resources/third-party/subscribe"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 401 {
			t.Fatal("anonymous resource API", path, w.Code)
		}
	}
	tok, _ := a.issue()
	for _, path := range []string{"/api/resources/third-party", "/api/resources/third-party?id=2"} {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), n.store.Inbounds[1].ThirdParty.Password) {
			t.Fatal("resource response exposes password")
		}
	}
	if _, err := os.Stat(filepath.Join(n.dir, "native.json")); err != nil {
		t.Fatal(err)
	}
	// The edit form deliberately leaves the password blank. A metadata edit
	// must retain the accepted secret and public client identity.
	before, _ := n.thirdPartySnapshot(2)
	body := map[string]any{"id": 2, "host": before.ThirdParty.Host, "port": before.ThirdParty.Port, "username": before.ThirdParty.Username, "password": "", "country": "US", "city": "Los Angeles", "remark": "edited-provider-city", "enabled": true, "subscribe": true}
	b, _ := json.Marshal(body)
	r := httptest.NewRequest("POST", "/api/resources/third-party", strings.NewReader(string(b)))
	r.Header.Set("Content-Type", "application/json")
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: tok})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	after, _ := n.thirdPartySnapshot(2)
	if w.Code != 200 || after.ThirdParty.Password != before.ThirdParty.Password || after.Clients[0].ID != before.Clients[0].ID || after.ThirdParty.Remark != "edited-provider-city" {
		t.Fatal("blank-password edit lost secret or identity", w.Code)
	}
}

func TestThirdPartyCheckDedupeStaleResultAndShutdown(t *testing.T) {
	n, _, m, c := thirdPartyFixture(t)
	started := make(chan struct{})
	done := make(chan struct{})
	c.third.probe = func(ctx context.Context, p ThirdPartyProxy, dir string) ThirdPartyRecord {
		close(started)
		select {
		case <-ctx.Done():
		case <-done:
		}
		return ThirdPartyRecord{Status: "passed", ActualIP: "8.8.8.8", CheckedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	}
	if err := c.third.start(n, 2); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := c.third.start(n, 2); err != nil {
		t.Fatal(err)
	}
	ib, _ := n.thirdPartySnapshot(2)
	p := *ib.ThirdParty
	p.Host = "changed.example.invalid"
	if _, err := n.saveThirdParty(2, p, true, "", m.Tunnels()); err != nil {
		t.Fatal(err)
	}
	close(done)
	c.third.wg.Wait()
	if r := c.third.snapshot()[2]; r.Status == "passed" {
		t.Fatal("stale result attached to new endpoint")
	}
	started = make(chan struct{})
	done = make(chan struct{})
	if err := c.third.start(n, 2); err != nil {
		t.Fatal(err)
	}
	<-started
	c.third.shutdown()
	if err := c.third.start(n, 2); err == nil {
		t.Fatal("new work accepted after shutdown")
	}
	if b, err := os.ReadFile(filepath.Join(n.dir, "third-party-checks.json")); err == nil {
		var v map[int]ThirdPartyRecord
		if json.Unmarshal(b, &v) != nil {
			t.Fatal("invalid result file")
		}
	}
}
