package main

// Explicitly enabled Linux acceptance against disposable copies. These tests
// use the real pinned Xray, real listeners and real filesystem failures. They
// never open a panel, restore Slots, or change an installed service.
import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func requireLiveAcceptance(t *testing.T) {
	t.Helper()
	if os.Getenv("XNV_LIVE_ACCEPTANCE") != "1" {
		t.Skip("set XNV_LIVE_ACCEPTANCE=1 on an authorized disposable Linux test copy")
	}
	if os.Geteuid() != 0 {
		t.Fatal("live filesystem acceptance requires root")
	}
	if err := initMainNetns(); err != nil {
		t.Fatal(err)
	}
}

func liveFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}

func liveFiles(t *testing.T, dir string, names []string) map[string][]byte {
	t.Helper()
	x := map[string][]byte{}
	for _, name := range names {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		x[name] = b
	}
	return x
}

func liveNative(t *testing.T) (*Native, string) {
	t.Helper()
	requireLiveAcceptance(t)
	dir := t.TempDir()
	source := os.Getenv("XNV_LIVE_NATIVE_COPY")
	bin := os.Getenv("XNV_LIVE_XRAY")
	if source == "" || bin == "" {
		t.Fatal("explicit private native COPY and pinned Xray paths required")
	}
	blob, err := os.ReadFile(source)
	if err != nil {
		t.Fatal("cannot read private native copy")
	}
	store, err := decodeNativeStore(blob)
	if err != nil || len(store.Inbounds) != 3 {
		t.Fatal("expected three valid copied native inbounds")
	}
	used := map[int]bool{}
	for _, ib := range store.Inbounds {
		for {
			ib.Port = liveFreePort(t)
			if !used[ib.Port] {
				used[ib.Port] = true
				break
			}
		}
		ib.BoundTo = ""
	}
	blob, _ = json.MarshalIndent(store, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "native.json"), blob, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal("cannot read pinned Xray")
	}
	// A real copy keeps /proc/exe equal to the owned executable path, including
	// when testing orphan cleanup after SIGKILL.
	if err := os.WriteFile(filepath.Join(dir, "bin/xray"), binary, 0700); err != nil {
		t.Fatal(err)
	}
	n, err := openNative(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(n.Close)
	if err := n.OnTunnelsChanged(nil); err != nil {
		t.Fatal("real Xray baseline failed:", err)
	}
	return n, dir
}

func liveRuntimeReady(t *testing.T, n *Native) {
	t.Helper()
	if n.proc.cmd == nil || n.proc.cmd.Process == nil {
		t.Fatal("accepted Xray process missing")
	}
	c, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(n.store.Inbounds[0].Port)), time.Second)
	if err != nil {
		t.Fatal("accepted Xray listener missing")
	}
	c.Close()
	cfg, err := os.ReadFile(filepath.Join(n.dir, "xray.json"))
	if err != nil || bytes.Contains(cfg, []byte(`"freedom"`)) || !bytes.Contains(cfg, []byte(`"blackhole"`)) {
		t.Fatal("disposable runtime must remain blackhole")
	}
}

func TestLiveNativeTransactions(t *testing.T) {
	n, dir := liveNative(t)
	liveRuntimeReady(t, n)
	t.Run("static_rejection_preserves_process_and_files", func(t *testing.T) {
		before := liveFiles(t, dir, nativeConfigFiles)
		pid := n.proc.cmd.Process.Pid
		finish := n.beginChange()
		defer finish()
		if n.store.Inbounds[0].Reality == nil {
			t.Fatal("REALITY copy required")
		}
		n.store.Inbounds[0].Reality.PrivateKey = "invalid-key"
		if n.apply(nil) == nil || n.proc.cmd.Process.Pid != pid || !reflect.DeepEqual(before, liveFiles(t, dir, nativeConfigFiles)) {
			t.Fatal("real parser rejection changed accepted runtime or files")
		}
		liveRuntimeReady(t, n)
	})
	t.Run("occupied_port_restores_previous_runtime", func(t *testing.T) {
		before := liveFiles(t, dir, nativeConfigFiles)
		occupied, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer occupied.Close()
		port := occupied.Addr().(*net.TCPAddr).Port
		if n.UpdateInbound(n.store.Inbounds[0].ID, InboundPatch{Port: &port}, nil) == nil {
			t.Fatal("real Xray startup port conflict accepted")
		}
		if !reflect.DeepEqual(before, liveFiles(t, dir, nativeConfigFiles)) {
			t.Fatal("startup compensation changed accepted files")
		}
		liveRuntimeReady(t, n)
	})
	t.Run("manual_rollback_uses_current_blackhole_routes", func(t *testing.T) {
		old := n.store.Inbounds[0].Remark
		remark := "disposable-live-acceptance"
		if err := n.UpdateInbound(n.store.Inbounds[0].ID, InboundPatch{Remark: &remark}, nil); err != nil {
			t.Fatal(err)
		}
		if err := n.Rollback(nil); err != nil || n.store.Inbounds[0].Remark != old {
			t.Fatal("manual rollback failed")
		}
		liveRuntimeReady(t, n)
	})
	t.Run("real_commit_failure_blocks_then_recovers_from_journal", func(t *testing.T) {
		before := liveFiles(t, dir, nativeConfigFiles)
		target := filepath.Join(dir, "native.json")
		if err := unix.Mount(target, target, "", unix.MS_BIND, ""); err != nil {
			t.Fatal("cannot create isolated filesystem fault:", err)
		}
		mounted := true
		defer func() {
			if mounted {
				_ = unix.Unmount(target, 0)
			}
		}()
		remark := "must-not-commit"
		if n.UpdateInbound(n.store.Inbounds[0].ID, InboundPatch{Remark: &remark}, nil) == nil {
			t.Fatal("rename over real mountpoint unexpectedly succeeded")
		}
		if !n.unavailable || n.proc.cmd != nil {
			t.Fatal("failed disk rollback left Xray running")
		}
		if _, err := os.Stat(filepath.Join(dir, nativeJournal)); err != nil {
			t.Fatal("failed rollback lost recovery record")
		}
		if err := unix.Unmount(target, 0); err != nil {
			t.Fatal(err)
		}
		mounted = false
		n.Close()
		recovered, err := openNative(dir)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(recovered.Close)
		if !reflect.DeepEqual(before, liveFiles(t, dir, nativeConfigFiles)) {
			t.Fatal("restart did not restore journaled files")
		}
		if err := recovered.OnTunnelsChanged(nil); err != nil {
			t.Fatal(err)
		}
		liveRuntimeReady(t, recovered)
	})
}

func TestLiveNativeInterruptedChild(t *testing.T) {
	if os.Getenv("XNV_LIVE_CHILD") != "1" {
		t.Skip("internal SIGKILL acceptance child")
	}
	requireLiveAcceptance(t)
	dir := os.Getenv("XNV_LIVE_CHILD_DIR")
	n, err := openNative(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := n.OnTunnelsChanged(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := beginConfigTransaction(dir, nativeJournal, nativeConfigFiles); err != nil {
		t.Fatal(err)
	}
	changed := cloneNativeStore(n.store)
	changed.Inbounds[0].Remark = "interrupted-uncommitted"
	blob, _ := json.Marshal(changed)
	if err := atomicConfigWrite(filepath.Join(dir, "native.json"), blob); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "child-ready"), []byte(strconv.Itoa(n.proc.cmd.Process.Pid)), 0600); err != nil {
		t.Fatal(err)
	}
	for {
		time.Sleep(time.Second)
	}
}

func TestLiveNativeSIGKILLRecovery(t *testing.T) {
	n, dir := liveNative(t)
	before := liveFiles(t, dir, nativeConfigFiles)
	n.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLiveNativeInterruptedChild$", "-test.timeout=1m")
	cmd.Env = append(os.Environ(), "XNV_LIVE_CHILD=1", "XNV_LIVE_CHILD_DIR="+dir)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	ready := filepath.Join(dir, "child-ready")
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not reach partially committed transaction")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	recovered, err := openNative(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(recovered.Close)
	if !reflect.DeepEqual(before, liveFiles(t, dir, nativeConfigFiles)) {
		t.Fatal("SIGKILL recovery did not restore committed files")
	}
	if _, err := os.Stat(filepath.Join(dir, nativeJournal)); !os.IsNotExist(err) {
		t.Fatal("recovered transaction still pending")
	}
	if err := recovered.OnTunnelsChanged(nil); err != nil {
		t.Fatal("orphan cleanup or real runtime recovery failed:", err)
	}
	liveRuntimeReady(t, recovered)
}

func TestLiveManagementFailures(t *testing.T) {
	requireLiveAcceptance(t)
	auth, srv, dir := managementFixture(t)
	// The existing fixture provides only private synthetic credentials. Replace
	// its handler with a real loopback listener; no listener/runtime hooks.
	srv.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("old-listener")) })
	next := getWebSettings()
	next.ListenAddr, next.Port = "127.0.0.1", liveFreePort(t)
	if err := commitManagementSettings(auth, srv, next, nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.srv.Close() })
	before := liveFiles(t, dir, managementConfigFiles)
	assertOld := func() {
		t.Helper()
		if !reflect.DeepEqual(before, liveFiles(t, dir, managementConfigFiles)) || !auth.check("fixture-old-password") {
			t.Fatal("failure changed committed management access")
		}
		c, err := net.DialTimeout("tcp4", next.listenAddrString(), time.Second)
		if err != nil {
			t.Fatal("old listener lost")
		}
		c.Close()
	}
	occupied, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	port := occupied.Addr().(*net.TCPAddr).Port
	password := "fixture-uncommitted-password"
	if applyManagementSettings(auth, srv, managementSettingsRequest{Port: &port, Password: &password}) == nil {
		t.Fatal("occupied management port accepted")
	}
	assertOld()
	target := filepath.Join(dir, "settings.json")
	if err := unix.Mount(target, target, "", unix.MS_BIND, ""); err != nil {
		t.Fatal(err)
	}
	defer unix.Unmount(target, 0)
	if applyManagementSettings(auth, srv, managementSettingsRequest{Password: &password}) == nil {
		t.Fatal("real management write failure accepted")
	}
	assertOld()
	if _, err := os.Stat(filepath.Join(dir, managementJournal)); err != nil {
		t.Fatal("management recovery record lost")
	}
	if err := unix.Unmount(target, 0); err != nil {
		t.Fatal(err)
	}
	if err := recoverConfigTransaction(dir, managementJournal, managementConfigFiles); err != nil {
		t.Fatal(err)
	}
	assertOld()
}
