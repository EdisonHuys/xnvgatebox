package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const rollbackFixtureClientID = "12345678-1234-4123-8123-123456789abc"

type faultNativeRuntime struct {
	running    bool
	reject     bool
	failStarts int
	starts     int
	stops      int
	configs    [][]byte
}

func (r *faultNativeRuntime) verify(path string) error {
	blob, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var config map[string]interface{}
	if json.Unmarshal(blob, &config) != nil {
		return errors.New("invalid synthetic JSON")
	}
	if r.reject {
		return errors.New("synthetic validator rejected")
	}
	return nil
}
func (r *faultNativeRuntime) restart(path string) error {
	r.starts++
	r.running = false
	blob, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	r.configs = append(r.configs, blob)
	if r.failStarts > 0 {
		r.failStarts--
		return errors.New("synthetic process startup failure")
	}
	r.running = true
	return nil
}
func (r *faultNativeRuntime) stop() { r.stops++; r.running = false }

func rollbackFixture(t *testing.T) (*Native, *faultNativeRuntime, []*Tunnel) {
	t.Helper()
	dir := t.TempDir()
	store := &nativeStore{NextID: 2, Inbounds: []*nativeInbound{{ID: 1, Port: 30001, Protocol: "vless", Network: "tcp", Security: "none", Enable: true,
		Remark: "accepted", BoundTo: "a", Clients: []nativeClient{{Email: "fixture", ID: rollbackFixtureClientID, Enable: true}}}}}
	if err := store.save(dir); err != nil {
		t.Fatal(err)
	}
	tunnels := []*Tunnel{{Slot: 1, Port: 17928, Status: "up", Node: Node{HostName: "a"}, Cred: SocksCred{User: "fixture-user", Pass: "fixture-pass"}}}
	if _, err := writeXrayConfig(dir, buildXrayConfig(store.sorted(), tunnels)); err != nil {
		t.Fatal(err)
	}
	runtime := &faultNativeRuntime{running: true}
	return &Native{dir: dir, store: store, committed: store, runtime: runtime}, runtime, tunnels
}

func assertAcceptedNative(t *testing.T, n *Native) {
	t.Helper()
	if n.store.NextID != 2 || len(n.store.Inbounds) != 1 || n.store.Inbounds[0].Port != 30001 || n.store.Inbounds[0].Remark != "accepted" || n.store.Inbounds[0].Clients[0].ID != rollbackFixtureClientID {
		t.Fatal("failed candidate contaminated accepted in-memory config")
	}
	disk, err := loadNativeStore(n.dir)
	if err != nil {
		t.Fatal(err)
	}
	if disk.NextID != 2 || len(disk.Inbounds) != 1 || disk.Inbounds[0].Port != 30001 || disk.Inbounds[0].Clients[0].ID != rollbackFixtureClientID {
		t.Fatal("accepted disk config not restored")
	}
}

func TestConfigTransactionFaultBoundariesRestoreAcceptedNative(t *testing.T) {
	for _, stage := range []string{"before_restart", "after_restart", "after_native_write", "before_commit"} {
		t.Run(stage, func(t *testing.T) {
			n, r, tunnels := rollbackFixture(t)
			old, _ := os.ReadFile(filepath.Join(n.dir, "xray.json"))
			n.transactionHook = func(at string) error {
				if at == stage {
					return errors.New("injected transaction boundary")
				}
				return nil
			}
			port := 30002
			if n.UpdateInbound(1, InboundPatch{Port: &port}, tunnels) == nil {
				t.Fatal("injected failure reported success")
			}
			assertAcceptedNative(t, n)
			if !r.running || n.unavailable {
				t.Fatal("old process/config not restored")
			}
			blob, _ := os.ReadFile(filepath.Join(n.dir, "xray.json"))
			if string(blob) != string(old) {
				t.Fatal("failed candidate replaced accepted active file")
			}
			if _, err := os.Stat(filepath.Join(n.dir, nativeJournal)); !os.IsNotExist(err) {
				t.Fatal("successful rollback left pending journal")
			}
			if _, err := os.Stat(filepath.Join(n.dir, "native.previous.json")); !os.IsNotExist(err) {
				t.Fatal("failed candidate rotated successful history")
			}
		})
	}
}

func TestConfigTransactionValidatorRejectionNeverStopsAcceptedProcess(t *testing.T) {
	n, r, tunnels := rollbackFixture(t)
	r.reject = true
	if n.ResetClient(1, "fixture", tunnels) == nil {
		t.Fatal("validator rejection ignored")
	}
	assertAcceptedNative(t, n)
	if r.starts != 0 || r.stops != 0 || !r.running {
		t.Fatal("validator failure interrupted old process")
	}
	files, err := filepath.Glob(filepath.Join(n.dir, ".xray-candidate-*"))
	if err != nil || len(files) != 0 {
		t.Fatal("candidate file leaked after rejection")
	}
}

func TestConfigTransactionRuntimeFailureAndRollbackFailureBlock(t *testing.T) {
	for _, failures := range []int{1, 2} {
		t.Run(string(rune('0'+failures)), func(t *testing.T) {
			n, r, tunnels := rollbackFixture(t)
			r.failStarts = failures
			port := 30002
			if n.UpdateInbound(1, InboundPatch{Port: &port}, tunnels) == nil {
				t.Fatal("process failure reported success")
			}
			assertAcceptedNative(t, n)
			if failures == 1 && (!r.running || n.unavailable) {
				t.Fatal("old process was not restarted")
			}
			if failures == 2 {
				if r.running || !n.unavailable {
					t.Fatal("rollback process failure was not fail closed")
				}
				if _, err := n.InboundLinks([]int{1}, "example.invalid"); err == nil {
					t.Fatal("unavailable service published a usable-looking subscription")
				}
				n.transactionHook = func(at string) error {
					if at == "before_restart" {
						return errors.New("not ready")
					}
					return nil
				}
				_ = n.ResyncOutbound(tunnels[0], tunnels)
				if !n.unavailable {
					t.Fatal("unstarted retry cleared unavailable state")
				}
			}
		})
	}
}

func TestConfigTransactionRollbackRebuildsAgainstCurrentSlot(t *testing.T) {
	n, r, tunnels := rollbackFixture(t)
	n.transactionHook = func(at string) error {
		if at == "after_restart" {
			tunnels[0].mu.Lock()
			tunnels[0].Status = "failed"
			tunnels[0].Node.HostName = "b"
			tunnels[0].mu.Unlock()
			return errors.New("slot changed during failure")
		}
		return nil
	}
	remark := "candidate"
	if n.UpdateInbound(1, InboundPatch{Remark: &remark}, tunnels) == nil {
		t.Fatal("fault ignored")
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(r.configs[len(r.configs)-1], &cfg); err != nil {
		t.Fatal(err)
	}
	for _, out := range cfg["outbounds"].([]interface{}) {
		if out.(map[string]interface{})["protocol"] != "blackhole" {
			t.Fatal("rollback replayed stale healthy SOCKS route")
		}
	}
	rules := cfg["routing"].(map[string]interface{})["rules"].([]interface{})
	if rules[0].(map[string]interface{})["outboundTag"] != "block" {
		t.Fatal("old binding silently mapped to new backing node")
	}
	assertAcceptedNative(t, n)
}

func TestConfigTransactionDiskRollbackFailureRetainsRecoverableRecord(t *testing.T) {
	n, r, tunnels := rollbackFixture(t)
	n.transactionHook = func(at string) error {
		if at == "after_native_write" {
			if err := os.Remove(nativeStatePath(n.dir)); err != nil {
				return err
			}
			if err := os.Mkdir(nativeStatePath(n.dir), 0700); err != nil {
				return err
			}
			return errors.New("disk fault")
		}
		return nil
	}
	port := 30002
	if n.UpdateInbound(1, InboundPatch{Port: &port}, tunnels) == nil || r.running || !n.unavailable {
		t.Fatal("unrecoverable write did not stop service")
	}
	journal := filepath.Join(n.dir, nativeJournal)
	stat, err := os.Stat(journal)
	if err != nil || stat.Mode().Perm() != 0600 {
		t.Fatal("private recovery record absent")
	}
	n.transactionHook = nil
	if n.UpdateInbound(1, InboundPatch{Port: &port}, tunnels) == nil {
		t.Fatal("pending recovery overwritten by next mutation")
	}
	if err := os.Remove(nativeStatePath(n.dir)); err != nil {
		t.Fatal(err)
	}
	if err := recoverConfigTransaction(n.dir, nativeJournal, nativeConfigFiles); err != nil {
		t.Fatal(err)
	}
	assertAcceptedNative(t, n)
}

func TestConfigTransactionRestartRecoversInterruptedCommit(t *testing.T) {
	n, _, _ := rollbackFixture(t)
	_, err := beginConfigTransaction(n.dir, nativeJournal, nativeConfigFiles)
	if err != nil {
		t.Fatal(err)
	}
	changed := cloneNativeStore(n.store)
	changed.Inbounds[0].Port = 39999
	changed.Inbounds[0].Clients[0].ID = "uncommitted"
	if err := changed.save(n.dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(n.dir, "xray.json"), []byte(`{"uncommitted":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := recoverConfigTransaction(n.dir, nativeJournal, nativeConfigFiles); err != nil {
		t.Fatal(err)
	}
	assertAcceptedNative(t, n)
	before, _ := os.ReadFile(nativeStatePath(n.dir))
	if err := recoverConfigTransaction(n.dir, nativeJournal, nativeConfigFiles); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(nativeStatePath(n.dir))
	if string(before) != string(after) {
		t.Fatal("recovery is not idempotent")
	}
}

func TestConfigTransactionRejectsUntrustedRecoveryPaths(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(filepath.Dir(dir), "rollback-test-outside")
	if err := os.WriteFile(outside, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outside)
	tx := configTransaction{Version: 1, Files: []configFileBefore{{Name: "../rollback-test-outside", Exists: true, Data: []byte("bad")}}}
	blob, _ := json.Marshal(tx)
	if err := os.WriteFile(filepath.Join(dir, stateJournal), blob, 0600); err != nil {
		t.Fatal(err)
	}
	if recoverConfigTransaction(dir, stateJournal, []string{"state.json"}) == nil {
		t.Fatal("recovery escaped managed filenames")
	}
	content, _ := os.ReadFile(outside)
	if string(content) != "untouched" {
		t.Fatal("outside file altered")
	}
}

func TestConfigTransactionDeepCopyAndEarlyMutationFailure(t *testing.T) {
	n, _, tunnels := rollbackFixture(t)
	n.store.Inbounds[0].TLS = &tlsConfig{ServerName: "accepted"}
	n.store.Inbounds[0].Reality = &realityConfig{ServerNames: []string{"accepted"}, ShortIDs: []string{"abcd"}}
	finish := n.beginChange()
	n.store.Inbounds[0].Clients[0].ID = "candidate"
	n.store.Inbounds[0].TLS.ServerName = "candidate"
	n.store.Inbounds[0].Reality.ServerNames[0] = "candidate"
	finish()
	if n.store.Inbounds[0].Clients[0].ID != rollbackFixtureClientID || n.store.Inbounds[0].TLS.ServerName != "accepted" || n.store.Inbounds[0].Reality.ServerNames[0] != "accepted" {
		t.Fatal("candidate aliases accepted private fields")
	}
	bad := 70000
	candidate := "candidate"
	if n.UpdateInbound(1, InboundPatch{Port: &bad, Remark: &candidate}, tunnels) == nil {
		t.Fatal("invalid port accepted")
	}
	if n.store.Inbounds[0].Remark != "accepted" {
		t.Fatal("early validation error retained partial mutation")
	}
}

func TestConfigTransactionManualNativeRollbackPreservesHistoryOnRefresh(t *testing.T) {
	n, r, tunnels := rollbackFixture(t)
	candidate := "second"
	if err := n.UpdateInbound(1, InboundPatch{Remark: &candidate}, tunnels); err != nil {
		t.Fatal(err)
	}
	if err := n.OnTunnelsChanged(tunnels); err != nil {
		t.Fatal(err)
	}
	if err := n.Rollback(tunnels); err != nil {
		t.Fatal(err)
	}
	assertAcceptedNative(t, n)
	if !r.running {
		t.Fatal("rollback did not apply accepted config")
	}
	previous, err := os.ReadFile(filepath.Join(n.dir, "native.previous.json"))
	if err != nil {
		t.Fatal(err)
	}
	var st nativeStore
	if json.Unmarshal(previous, &st) != nil || st.Inbounds[0].Remark != "second" {
		t.Fatal("successful previous version not retained")
	}
}

func TestConfigTransactionStateFailureRestoresDiskPolicyAndHistory(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(3, dir)
	p := syntheticPolicy()
	tunnel := &Tunnel{Slot: 1, Port: 17928, Status: "stopped", Policy: &p}
	m.tunnels[1] = tunnel
	if err := m.saveState(); err != nil {
		t.Fatal(err)
	}
	m.stateWriteHook = func(string) error { return errors.New("injected before state commit") }
	q := p
	q.Country = "JP"
	if m.SetSlotPolicy(1, q) == nil {
		t.Fatal("state failure reported success")
	}
	if tunnel.policySnapshot().Country != "US" || tunnel.previousPolicy != nil || tunnel.Status != "stopped" {
		t.Fatal("failed state write changed accepted memory")
	}
	restored := NewManager(3, dir)
	if _, err := restored.restoreState(); err != nil {
		t.Fatal(err)
	}
	if restored.tunnels[1].policySnapshot().Country != "US" || restored.tunnels[1].previousPolicy != nil {
		t.Fatal("uncommitted policy accepted on restart")
	}
}

func TestConfigTransactionPolicyRollbackPersistsAndRevalidatesIntent(t *testing.T) {
	m := NewManager(3, t.TempDir())
	p := syntheticPolicy()
	m.tunnels[1] = &Tunnel{Slot: 1, Port: 17928, Status: "stopped", Policy: &p}
	q := p
	q.Country = "JP"
	if err := m.SetSlotPolicy(1, q); err != nil {
		t.Fatal(err)
	}
	restored := NewManager(3, m.workDir)
	if _, err := restored.restoreState(); err != nil {
		t.Fatal(err)
	}
	if err := restored.RollbackSlotPolicy(1); err != nil {
		t.Fatal(err)
	}
	t1 := restored.tunnels[1]
	if t1.policySnapshot().Country != "US" || t1.previousPolicy.Country != "JP" || t1.Status != "stopped" || t1.Port != 17928 || t1.egressSnapshot(false) != nil {
		t.Fatal("rollback changed Slot identity or resurrected health")
	}
}

func TestConfigTransactionCredentialCompensationAndFailClosed(t *testing.T) {
	for _, rollbackFails := range []bool{false, true} {
		m := NewManager(3, t.TempDir())
		old := SocksCred{User: "fixture-old", Pass: "fixture-password"}
		t1 := &Tunnel{Slot: 1, Port: 17928, Status: "up", Cred: old}
		t1.resetWork()
		m.tunnels[1] = t1
		if err := m.saveState(); err != nil {
			t.Fatal(err)
		}
		calls := 0
		m.credentialApply = func(tunnel *Tunnel) error {
			calls++
			if calls == 1 || rollbackFails {
				return errors.New("injected outbound failure")
			}
			if tunnel.credential() != old {
				t.Fatal("compensation did not restore old credential")
			}
			return nil
		}
		if _, err := m.SetCred(1, SocksCred{User: "fixture-new", Pass: "fixture-new-password"}); err == nil {
			t.Fatal("failed credential change succeeded")
		}
		if t1.credential() != old || calls != 2 {
			t.Fatal("failed credential change retained new secret")
		}
		if rollbackFails && (t1.Status != "blocked" || t1.acceptsSocks()) {
			t.Fatal("failed credential rollback did not block")
		}
	}
}

func TestConfigTransactionAPIsRequireLoginAndPost(t *testing.T) {
	m := NewManager(3, t.TempDir())
	auth, _, err := NewAuth(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	auth.Wrap(apiRollbackNative(m)).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/config/rollback", nil))
	if rec.Code != 401 {
		t.Fatal("rollback endpoint not login protected")
	}
	rec = httptest.NewRecorder()
	apiRollbackSlotPolicy(m)(rec, httptest.NewRequest(http.MethodGet, "/api/slot/policy/rollback?slot=1", nil))
	if rec.Code != 405 {
		t.Fatal("GET changed Slot policy")
	}
	rec = httptest.NewRecorder()
	apiRollbackNative(m)(rec, httptest.NewRequest(http.MethodDelete, "/api/config/rollback", nil))
	if rec.Code != 405 {
		t.Fatal("unsupported method opened native backend")
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Fatal("rollback error exposed secrets")
	}
}

func TestConfigTransactionCredentialSaveFailureCompensatesRuntime(t *testing.T) {
	m := NewManager(3, t.TempDir())
	old := SocksCred{User: "old", Pass: "fixture-password"}
	t1 := &Tunnel{Slot: 1, Port: 17928, Status: "stopped", Cred: old}
	m.tunnels[1] = t1
	if err := m.saveState(); err != nil {
		t.Fatal(err)
	}
	m.stateWriteHook = func(string) error { return errors.New("state commit failed") }
	applied := []SocksCred{}
	m.credentialApply = func(t *Tunnel) error { applied = append(applied, t.credential()); return nil }
	if _, err := m.SetCred(1, SocksCred{User: "new", Pass: "new-fixture-password"}); err == nil {
		t.Fatal("failed state commit reported success")
	}
	if len(applied) != 2 || applied[0].User != "new" || applied[1] != old || t1.credential() != old {
		t.Fatal("runtime credential not compensated")
	}
	restored := NewManager(3, m.workDir)
	if _, err := restored.restoreState(); err != nil {
		t.Fatal(err)
	}
	if restored.tunnels[1].credential() != old {
		t.Fatal("uncommitted credential restored after restart")
	}
}

func TestConfigTransactionInterruptedStateRecoversPreviousStoppedPolicy(t *testing.T) {
	m := NewManager(3, t.TempDir())
	p := syntheticPolicy()
	m.tunnels[1] = &Tunnel{Slot: 1, Port: 17928, Status: "stopped", Policy: &p}
	if err := m.saveState(); err != nil {
		t.Fatal(err)
	}
	if _, err := beginConfigTransaction(m.workDir, stateJournal, []string{"state.json"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath(m.workDir), []byte(`{"tunnels":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	restored := NewManager(3, m.workDir)
	if _, err := restored.restoreState(); err != nil {
		t.Fatal(err)
	}
	if restored.tunnels[1] == nil || restored.tunnels[1].policySnapshot().Country != "US" || restored.tunnels[1].Status != "stopped" {
		t.Fatal("interrupted commit lost durable Slot intent")
	}
}

func TestConfigTransactionCertCleanupRetainsAcceptedAndHistory(t *testing.T) {
	n, _, _ := rollbackFixture(t)
	certDir := filepath.Join(n.dir, "certs")
	if err := os.MkdirAll(certDir, 0700); err != nil {
		t.Fatal(err)
	}
	cert := func(name string) *tlsConfig {
		c, k := filepath.Join(certDir, name+".crt"), filepath.Join(certDir, name+".key")
		if err := os.WriteFile(c, []byte("synthetic cert"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(k, []byte("synthetic key"), 0600); err != nil {
			t.Fatal(err)
		}
		return &tlsConfig{CertFile: c, KeyFile: k, SelfSigned: true}
	}
	accepted, history, failed := cert("accepted"), cert("history"), cert("failed")
	n.store.Inbounds[0].TLS = accepted
	before := cloneNativeStore(n.store)
	before.Inbounds[0].TLS = history
	blob, _ := json.Marshal(before)
	if err := os.WriteFile(filepath.Join(n.dir, "native.previous.json"), blob, 0600); err != nil {
		t.Fatal(err)
	}
	candidate := cloneNativeStore(n.store)
	candidate.Inbounds = append(candidate.Inbounds, &nativeInbound{TLS: history}, &nativeInbound{TLS: failed})
	n.cleanupCandidateCerts(candidate, n.store)
	for _, keep := range []string{accepted.CertFile, accepted.KeyFile, history.CertFile, history.KeyFile} {
		if _, err := os.Stat(keep); err != nil {
			t.Fatal("rollback removed an accepted/history certificate")
		}
	}
	if _, err := os.Stat(failed.KeyFile); !os.IsNotExist(err) {
		t.Fatal("failed candidate private key not cleaned")
	}
}

func TestConfigTransactionNewSelfSignedCertDoesNotOverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	cert, key, err := selfSignedCert(dir, "localhost")
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(key)
	if err != nil {
		t.Fatal(err)
	}
	newCert, newKey, err := selfSignedCert(dir, "localhost")
	if err != nil {
		t.Fatal(err)
	}
	if cert == newCert || key == newKey {
		t.Fatal("new TLS candidate reused accepted paths")
	}
	after, _ := os.ReadFile(key)
	if string(original) != string(after) {
		t.Fatal("accepted private key overwritten")
	}
	for _, name := range []string{cert, key, newCert, newKey} {
		stat, err := os.Stat(name)
		if err != nil || stat.Mode().Perm() != 0600 {
			t.Fatal("generated certificate/key not private")
		}
	}
}

func TestConfigTransactionConcurrentNativeChangesRemainCommitted(t *testing.T) {
	n, _, tunnels := rollbackFixture(t)
	done := make(chan error, 2)
	go func() {
		remark := "committed remark"
		done <- n.UpdateInbound(1, InboundPatch{Remark: &remark}, tunnels)
	}()
	go func() { done <- n.ResetClient(1, "fixture", tunnels) }()
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if n.store.Inbounds[0].Remark != "committed remark" || n.store.Inbounds[0].Clients[0].ID == rollbackFixtureClientID {
		t.Fatal("concurrent changes lost accepted fields")
	}
	disk, err := loadNativeStore(n.dir)
	if err != nil {
		t.Fatal(err)
	}
	if disk.Inbounds[0].Remark != n.store.Inbounds[0].Remark || disk.Inbounds[0].Clients[0].ID != n.store.Inbounds[0].Clients[0].ID {
		t.Fatal("accepted memory/disk diverged")
	}
}

func managementFixture(t *testing.T) (*Auth, *webServer, string) {
	t.Helper()
	dir := t.TempDir()
	webSettingsMu.RLock()
	oldSettings, oldPath := webSettingsCur, webSettingsPath
	webSettingsMu.RUnlock()
	basePathMu.RLock()
	oldBase, oldBaseDir := basePathCur, basePathDir
	basePathMu.RUnlock()
	t.Cleanup(func() {
		webSettingsMu.Lock()
		webSettingsCur, webSettingsPath = oldSettings, oldPath
		webSettingsMu.Unlock()
		basePathMu.Lock()
		basePathCur, basePathDir = oldBase, oldBaseDir
		basePathMu.Unlock()
	})
	if _, err := loadWebSettings(dir, 8899, false); err != nil {
		t.Fatal(err)
	}
	auth, _, err := NewAuth(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.SetPassword("fixture-old-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := initBasePath(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := setBasePath("old-entry"); err != nil {
		t.Fatal(err)
	}
	webSettingsMu.Lock()
	webSettingsCur.SubToken = "fixture-old-subscription"
	webSettingsMu.Unlock()
	if err := saveWebSettings(); err != nil {
		t.Fatal(err)
	}
	return auth, newWebServer(http.NotFoundHandler()), dir
}

func TestConfigTransactionManagementValidatesWholeRequestBeforeMutation(t *testing.T) {
	auth, srv, _ := managementFixture(t)
	password, base, port := "fixture-new-password", "new-entry", 70000
	if applyManagementSettings(auth, srv, managementSettingsRequest{Password: &password, BasePath: &base, Port: &port}) == nil {
		t.Fatal("bad port accepted")
	}
	if !auth.check("fixture-old-password") || auth.check(password) || currentBasePath() != "/old-entry" || getWebSettings().Port != 8899 {
		t.Fatal("partially applied invalid management settings")
	}
}

func TestConfigTransactionManagementWriteFailureKeepsOldEntryAndListener(t *testing.T) {
	for _, stage := range []string{"after_settings_write", "after_password_write", "before_commit"} {
		t.Run(stage, func(t *testing.T) {
			auth, srv, dir := managementFixture(t)
			activated, canceled := false, false
			srv.prepareListener = func(WebSettings) (*preparedManagementListener, error) {
				return &preparedManagementListener{activate: func() { activated = true }, cancel: func() { canceled = true }}, nil
			}
			srv.managementHook = func(at string) error {
				if at == stage {
					return errors.New("injected management write failure")
				}
				return nil
			}
			password, base, port := "fixture-new-password", "new-entry", 9900
			if applyManagementSettings(auth, srv, managementSettingsRequest{Password: &password, BasePath: &base, Port: &port}) == nil {
				t.Fatal("write failure reported success")
			}
			if activated || !canceled || !auth.check("fixture-old-password") || currentBasePath() != "/old-entry" || getWebSettings().Port != 8899 || getWebSettings().SubToken != "fixture-old-subscription" {
				t.Fatal("write failure changed old management entry/secret")
			}
			diskAuth, _, err := NewAuth(dir)
			if err != nil || !diskAuth.check("fixture-old-password") {
				t.Fatal("password disk rollback failed")
			}
			bp, _, err := LoadBasePath(dir)
			if err != nil || bp != "/old-entry" {
				t.Fatal("entry disk rollback failed")
			}
		})
	}
}

func TestConfigTransactionManagementBindFailureDoesNotChangeCredentials(t *testing.T) {
	auth, srv, _ := managementFixture(t)
	srv.prepareListener = func(WebSettings) (*preparedManagementListener, error) {
		return nil, errors.New("synthetic bind denied")
	}
	password, port := "fixture-new-password", 9900
	if applyManagementSettings(auth, srv, managementSettingsRequest{Password: &password, Port: &port}) == nil || !auth.check("fixture-old-password") || getWebSettings().Port != 8899 {
		t.Fatal("failed bind changed access")
	}
}

func TestConfigTransactionManagementCommitActivatesLastAndPreservesToken(t *testing.T) {
	auth, srv, dir := managementFixture(t)
	activated := false
	srv.prepareListener = func(WebSettings) (*preparedManagementListener, error) {
		return &preparedManagementListener{activate: func() {
			activated = true
			if !auth.check("fixture-new-password") || currentBasePath() != "/new-entry" {
				t.Fatal("listener activated before all settings committed")
			}
		}, cancel: func() { t.Fatal("committed listener canceled") }}, nil
	}
	password, base, port := "fixture-new-password", "new-entry", 9900
	if err := applyManagementSettings(auth, srv, managementSettingsRequest{Password: &password, BasePath: &base, Port: &port}); err != nil {
		t.Fatal(err)
	}
	if !activated || getWebSettings().SubToken != "fixture-old-subscription" {
		t.Fatal("token changed during unrelated config commit")
	}
	if _, err := os.Stat(filepath.Join(dir, managementJournal)); !os.IsNotExist(err) {
		t.Fatal("committed transaction still pending")
	}
}

func TestConfigTransactionManagementCrashRecoveryPreservesOldAccess(t *testing.T) {
	_, _, dir := managementFixture(t)
	if _, err := beginConfigTransaction(dir, managementJournal, managementConfigFiles); err != nil {
		t.Fatal(err)
	}
	if err := atomicConfigWrite(filepath.Join(dir, "password"), []byte("uncommitted-password")); err != nil {
		t.Fatal(err)
	}
	if err := atomicConfigWrite(filepath.Join(dir, "basepath"), []byte("uncommitted-entry")); err != nil {
		t.Fatal(err)
	}
	if err := recoverConfigTransaction(dir, managementJournal, managementConfigFiles); err != nil {
		t.Fatal(err)
	}
	auth, _, err := NewAuth(dir)
	if err != nil || !auth.check("fixture-old-password") {
		t.Fatal("uncommitted password adopted after restart")
	}
	bp, _, err := LoadBasePath(dir)
	if err != nil || bp != "/old-entry" {
		t.Fatal("uncommitted entry adopted after restart")
	}
}

func TestConfigTransactionTokenWriteFailureRetainsInMemoryToken(t *testing.T) {
	_, _, dir := managementFixture(t)
	if _, err := beginConfigTransaction(dir, managementJournal, managementConfigFiles); err != nil {
		t.Fatal(err)
	}
	if _, err := resetSubToken(); err == nil {
		t.Fatal("pending transaction allowed token overwrite")
	}
	if getWebSettings().SubToken != "fixture-old-subscription" {
		t.Fatal("failed token write replaced in-memory access token")
	}
	if err := recoverConfigTransaction(dir, managementJournal, managementConfigFiles); err != nil {
		t.Fatal(err)
	}
}

// Static parsing only: no listener, no tunnel and no replacement for the real
// namespace/process fail-closed acceptance that this platform cannot run.
func TestConfigTransactionPinnedXrayParsesGeneratedCandidate(t *testing.T) {
	bin, err := filepath.Abs("../../tools/bin/xray")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skip("pinned Xray not installed for static parser smoke test")
	}
	n, _, tunnels := rollbackFixture(t)
	path, _, err := n.stageConfig(n.store, tunnels)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	if out, err := exec.Command(bin, "run", "-test", "-c", path).CombinedOutput(); err != nil {
		t.Fatalf("pinned Xray rejected synthetic static configuration: %s", out)
	}
}

func TestConfigTransactionExplicitEmptyManagementPathPersists(t *testing.T) {
	auth, srv, dir := managementFixture(t)
	empty := ""
	if err := applyManagementSettings(auth, srv, managementSettingsRequest{BasePath: &empty}); err != nil {
		t.Fatal(err)
	}
	bp, created, err := LoadBasePath(dir)
	if err != nil || bp != "" || created {
		t.Fatal("restart regenerated a path instead of preserving explicit empty setting")
	}
}
