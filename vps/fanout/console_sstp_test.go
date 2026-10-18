package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sstpFixture(t *testing.T) *SstpManager {
	t.Helper()
	c := consoleFixture(t)
	c.sstp.dependencies = nil
	for _, host := range []string{"a.opengw.net", "b.opengw.net", "c.opengw.net"} {
		c.m.nodes = append(c.m.nodes, Node{HostName: host, IP: "8.8.4.4", CountryCode: "JP", SSTPCandidates: []SstpEndpoint{{Host: host, Port: 443}}})
	}
	t.Cleanup(c.shutdown)
	return c.sstp
}
func sstpObservation(n Node) CheckResult {
	return CheckResult{Host: n.HostName, Status: "passed", ObservedBy: "vps", Protocol: "sstp", Port: 443, ActualIP: "8.8.8.8", IPType: "unknown", CheckedAt: time.Now().UTC(), ExpiresAt: time.Now().Add(time.Hour).UTC(), ProfileSHA: sstpProfile(n)}
}

func TestSstpExportRejectsExpiredChangedUnboundAndUnmeasured(t *testing.T) {
	s := sstpFixture(t)
	n := s.c.m.nodes[0]
	good := sstpObservation(n)
	for _, change := range []func(*CheckResult){func(r *CheckResult) { r.ExpiresAt = time.Now().Add(-time.Second) }, func(r *CheckResult) { r.ProfileSHA = "old" }, func(r *CheckResult) { r.ActualIP = "10.0.0.1" }, func(r *CheckResult) { r.ObservedBy = "cf" }, func(r *CheckResult) { r.Port = 444 }, func(r *CheckResult) { r.Status = "running" }, func(r *CheckResult) { r.CheckedAt = time.Now().Add(time.Hour) }} {
		r := good
		change(&r)
		s.state.Results[n.HostName] = r
		w := httptest.NewRecorder()
		s.apiExport(w, httptest.NewRequest("GET", "/api/resources/sstp/export", nil))
		if w.Code != 409 {
			t.Fatal("invalid evidence exported")
		}
	}
	s.state.Results[n.HostName] = good
	w := httptest.NewRecorder()
	s.apiExport(w, httptest.NewRequest("GET", "/api/resources/sstp/export?format=json", nil))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code)
	}
	var v map[string]any
	json.Unmarshal(w.Body.Bytes(), &v)
	if v["kind"] != "vpngate_vps_sstp_observations" || v["cf_status"] != "not_tested" {
		t.Fatal("CF success misrepresented")
	}
	r := v["nodes"].([]any)[0].(map[string]any)["result"].(map[string]any)
	if r["country"] != nil || r["ip_type"] != "unknown" {
		t.Fatal("advertised country leaked as measured evidence")
	}
	w = httptest.NewRecorder()
	s.apiExport(w, httptest.NewRequest("GET", "/api/resources/sstp/export?format=text&hosts=b.opengw.net", nil))
	if w.Code != 409 {
		t.Fatal("selected range expanded")
	}
	w = httptest.NewRecorder()
	s.apiExport(w, httptest.NewRequest("GET", "/api/resources/sstp/export?format=text&hosts=a.opengw.net", nil))
	if w.Code != 200 || w.Body.String() != "sstp://vpn:vpn@a.opengw.net:443\n" {
		t.Fatal("wrong private connection")
	}
}
func TestSstpSettingsAndResultsSurviveRestartWithoutRestoringTunnels(t *testing.T) {
	s := sstpFixture(t)
	s.state.Settings = SstpSettings{128, 0, 35, 80}
	s.state.Results[s.c.m.nodes[0].HostName] = sstpObservation(s.c.m.nodes[0])
	if err := s.persist(); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(filepath.Join(s.c.m.workDir, "sstp.json"))
	if info.Mode().Perm() != 0600 {
		t.Fatal("private state permissions")
	}
	c, err := newConsole(NewManager(maximumSlots, s.c.m.workDir))
	if err != nil {
		t.Fatal(err)
	}
	defer c.shutdown()
	if c.sstp.state.Settings.Parallelism != 128 || len(c.sstp.state.Results) != 1 || len(c.m.Tunnels()) != 0 || c.checkParallelism != 2 {
		t.Fatal("restart changes intent or restores diagnostic tunnel")
	}
	for _, setting := range []SstpSettings{{0, 0, 60, 90}, {1, -1, 60, 90}, {1, 0, 0, 90}, {1, 0, 60, 9}} {
		if validateSstpSettings(setting) == nil {
			t.Fatal("invalid setting accepted")
		}
	}
	dir := migrationFixture(t)
	os.WriteFile(filepath.Join(dir, "sstp.json"), []byte(`{"settings":{}}`), 0600)
	before := migrationTree(t, dir)
	if checkMigrationState(dir, 32, 8899).ConfigValid {
		t.Fatal("corruption missed")
	}
	after := migrationTree(t, dir)
	if len(before) != len(after) {
		t.Fatal("preflight wrote files")
	}
}
func TestSstpPriorityDedupeCancellationAndEvidenceIsolation(t *testing.T) {
	s := sstpFixture(t)
	s.state.Settings.Parallelism = 1
	started := make(chan string, 4)
	release := make(chan struct{}, 4)
	s.probe = func(ctx context.Context, n Node, _ SstpSettings, progress func(string)) CheckResult {
		progress("sstp_connecting")
		started <- n.HostName
		select {
		case <-release:
		case <-ctx.Done():
		}
		return sstpObservation(n)
	}
	j, err := s.start(CheckRequest{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if <-started != "a.opengw.net" {
		t.Fatal("wrong first")
	}
	for i := 0; i < 2; i++ {
		if _, err = s.start(CheckRequest{Hosts: []string{"c.opengw.net"}, Priority: true}); err != nil {
			t.Fatal(err)
		}
	}
	j.mu.Lock()
	total := j.Total
	j.mu.Unlock()
	if total != 3 {
		t.Fatal("duplicate scheduled")
	}
	release <- struct{}{}
	if <-started != "c.opengw.net" {
		t.Fatal("manual priority ignored")
	}
	s.state.Results["c.opengw.net"] = sstpObservation(s.c.m.nodes[2])
	j.cancel()
	waitConsoleCheck(t, j)
	<-j.finished
	if s.state.Results["c.opengw.net"].Status != "passed" || len(s.c.results) != 0 {
		t.Fatal("cancellation overwrote evidence or contaminated OpenVPN")
	}
	if j.Done != 3 || j.Status != "cancelled" {
		t.Fatal("cancel didn't drain")
	}
}
func TestSstpRoutesArePrivateAndDoNotExposeInternalClientCredentials(t *testing.T) {
	s := sstpFixture(t)
	s.state.Results[s.c.m.nodes[0].HostName] = sstpObservation(s.c.m.nodes[0])
	mux := http.NewServeMux()
	s.routes(mux)
	a := newTestAuth()
	wrapped := a.Wrap(mux)
	for _, path := range []string{"/api/resources/sstp", "/api/resources/sstp/export?format=text"} {
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 401 || strings.Contains(w.Body.String(), "sstp://") {
			t.Fatal("private resource anonymous")
		}
	}
	token, _ := a.issue()
	r := httptest.NewRequest("GET", "/api/resources/sstp", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, r)
	if w.Code != 200 || strings.Contains(w.Body.String(), `"password"`) {
		t.Fatal("view leaks credentials")
	}
}
func TestSstpPersistenceFailureDoesNotPublishPass(t *testing.T) {
	s := sstpFixture(t)
	s.probe = func(context.Context, Node, SstpSettings, func(string)) CheckResult {
		return CheckResult{Status: "passed"}
	}
	j := &CheckRun{Items: []CheckResult{{}}, finished: make(chan struct{})}
	os.Mkdir(filepath.Join(s.c.m.workDir, "sstp.json"), 0700)
	s.runItem(context.Background(), j, 0, CheckResult{}, s.c.m.nodes[0], s.state.Settings)
	if len(s.state.Results) != 0 || j.Items[0].Code != "result_save_failed" {
		t.Fatal("unsaved pass published")
	}
}
func TestSstpEndpointAndTLSRejectUnsafeInputs(t *testing.T) {
	n := Node{HostName: "a.opengw.net", IP: "8.8.8.8"}
	if !officialSstpEndpoint(n, SstpEndpoint{n.HostName, 443}) {
		t.Fatal("official rejected")
	}
	for _, e := range []SstpEndpoint{{"localhost", 443}, {"a.opengw.net:1", 443}, {n.HostName, 0}, {n.HostName, 65536}} {
		if officialSstpEndpoint(n, e) {
			t.Fatal("unsafe endpoint accepted")
		}
	}
	for _, ip := range []string{"127.0.0.1", "100.64.0.1", "10.0.0.1", "192.0.2.1", "::1", "203.0.113.1", "198.18.0.1"} {
		if sstpPublicIPv4(ip) {
			t.Fatal("not public", ip)
		}
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	addr := server.Listener.Addr().(*net.TCPAddr)
	if verifySstpTLS(context.Background(), "127.0.0.1", SstpEndpoint{"localhost", addr.Port}) == nil {
		t.Fatal("self-signed certificate accepted")
	}
}
func TestSstpPrivatePPPFilesAndProcessOwnership(t *testing.T) {
	dir := t.TempDir()
	work := filepath.Join(dir, ".connectivity-sstp-test")
	os.Mkdir(work, 0700)
	ns := "fixture-d1"
	if err := writeSstpFiles(work, ns); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(work, ns+".ppp"))
	if !strings.Contains(string(b), "nodefaultroute") || strings.Contains(sstpLaunch, "--cert-warn") || !strings.Contains(sstpLaunch, "mount --bind") || !strings.Contains(sstpLaunch, "tmpfs /run") {
		t.Fatal("isolation or TLS regression")
	}
	if !diagnosticProcessOwned(dir, ns, "/usr/sbin/pppd", filepath.Join(work, ns+".ppp")) || diagnosticProcessOwned(dir, ns, "/usr/sbin/pppd", "/etc/ppp/host") || diagnosticProcessOwned(dir, ns, "/bin/other", filepath.Join(work, ns+".ppp")) {
		t.Fatal("wrong ownership")
	}
	m := NewManager(32, dir)
	_, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	a := m.reserveDiagnostic(Node{HostName: "a", IP: "8.8.8.8"}, cancel)
	bLease := m.reserveDiagnostic(Node{HostName: "b", IP: "8.8.4.4"}, cancel)
	if a == nil || bLease == nil || a.slot == bLease.slot || a.networkIndex == bLease.networkIndex {
		t.Fatal("protocols collide")
	}
	m.releaseDiagnostic("a", a)
	m.releaseDiagnostic("b", bLease)
}

func TestSstpOptionalSchedulerDoesNotOverlapOrRestartEarly(t *testing.T) {
	s := sstpFixture(t)
	s.runScheduled(time.Now())
	if s.job != nil {
		t.Fatal("disabled scheduler started scan")
	}
	s.state.Settings.IntervalMinutes = 60
	s.state.LastStarted = time.Now().Add(-2 * time.Hour)
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	s.probe = func(ctx context.Context, n Node, _ SstpSettings, _ func(string)) CheckResult {
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return sstpObservation(n)
	}
	s.runScheduled(time.Now())
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("due scheduler did not start")
	}
	j := s.job
	s.runScheduled(time.Now().Add(2 * time.Hour))
	if s.job != j {
		t.Fatal("overlapping automatic job")
	}
	close(release)
	waitConsoleCheck(t, j)
	<-j.finished
	s.runScheduled(time.Now())
	if s.job != j {
		t.Fatal("restart before interval elapsed")
	}
}
