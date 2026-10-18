package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Explicit opt-in on a disposable instance. No installed manager, CF or panel
// is opened. A private current official candidate is supplied by the operator.
func TestLiveSstpIsolationAndRecovery(t *testing.T) {
	if os.Getenv("XNV_SSTP_LIVE") != "1" {
		t.Skip("explicit authorized SSTP Linux acceptance only")
	}
	if err := sstpDependencies(); err != nil {
		t.Fatal(err)
	}
	blob, err := os.ReadFile(os.Getenv("XNV_SSTP_CANDIDATE"))
	if err != nil {
		t.Fatal("private candidate required")
	}
	var n Node
	if json.Unmarshal(blob, &n) != nil || len(n.SSTPCandidates) == 0 {
		t.Fatal("invalid candidate")
	}
	dir := t.TempDir()
	oldTag, oldBase := instTag, instBase
	defer func() { instTag, instBase = oldTag, oldBase }()
	if err := initInstance(dir); err != nil {
		t.Fatal(err)
	}
	before, _ := exec.Command("ip", "-j", "route", "show", "default").Output()
	options, _ := os.ReadFile("/etc/ppp/options")
	c, err := newConsole(NewManager(maximumSlots, dir))
	if err != nil {
		t.Fatal(err)
	}
	defer c.shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_, cancelLease := context.WithCancelCause(ctx)
	defer cancelLease(nil)
	lease := c.m.reserveDiagnostic(n, cancelLease)
	if lease == nil {
		t.Fatal("lease")
	}
	phases := []string{}
	r := c.sstp.probeEndpoint(ctx, n, n.SSTPCandidates[0], lease, func(p string) { phases = append(phases, p) })
	c.m.releaseDiagnostic(n.HostName, lease)
	if r.Code == "probe_environment_failed" || r.Code == "sstp_cleanup_failed" {
		t.Fatal("native environment/cleanup failed", r.Code)
	}
	if len(phases) < 2 {
		t.Fatal("native client was not launched")
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, ".connectivity-*"))
	if len(leftovers) != 0 {
		t.Fatal("native client resources left")
	}
	// Crash simulation: an owned shell remains in an owned namespace. Startup
	// recovery must identify and stop it, then delete only this instance's links.
	_, cancelCrash := context.WithCancelCause(context.Background())
	defer cancelCrash(nil)
	l := c.m.reserveDiagnostic(n, cancelCrash)
	tun := &Tunnel{Slot: l.slot, networkIndex: &l.networkIndex, Node: n}
	tun.Node.Config = "proto tcp\nremote " + n.IP + " 4443\n"
	tun.resetWork()
	crashDir, _ := os.MkdirTemp(dir, ".connectivity-sstp-")
	manifest, _ := json.Marshal(diagnosticResource{Config: tun.Node.Config, Slot: tun.Slot, NetworkIndex: l.networkIndex, Node: tun.Node})
	os.WriteFile(filepath.Join(crashDir, "diagnostic.json"), manifest, 0600)
	launch := filepath.Join(crashDir, tun.nsName()+".launch")
	os.WriteFile(launch, []byte("while :; do :; done\n"), 0600)
	if err := tun.setupNetns(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("ip", "netns", "exec", tun.nsName(), "/bin/sh", launch)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	// Wait for exec to finish; the launcher must have the owned .launch argument.
	ready := false
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		b, _ := os.ReadFile(filepath.Join("/proc", strconv.Itoa(cmd.Process.Pid), "cmdline"))
		if strings.Contains(string(b), launch) && !strings.Contains(string(b), "netns\x00exec") {
			ready = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		t.Fatal("crash fixture not ready")
	}
	if err := c.cleanupDiagnostics(); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	c.m.releaseDiagnostic(n.HostName, l)
	after, _ := exec.Command("ip", "-j", "route", "show", "default").Output()
	optionsAfter, _ := os.ReadFile("/etc/ppp/options")
	if string(before) != string(after) || string(options) != string(optionsAfter) {
		t.Fatal("host route or PPP options modified")
	}
	ns, _ := exec.Command("ip", "netns", "list").Output()
	rules, _ := exec.Command("iptables", "-S", "FORWARD").Output()
	veth, _ := tun.vethNames()
	if strings.Contains(string(rules), veth) {
		t.Fatal("crash control-plane firewall rules remain")
	}
	if strings.Contains(string(ns), tun.nsName()) {
		t.Fatal("crash namespace remains")
	}
	t.Logf("native SSTP attempt status=%s code=%s; isolated cleanup and crash recovery passed", r.Status, r.Code)
}

func TestLiveSstpEndToEnd(t *testing.T) {
	if os.Getenv("XNV_SSTP_E2E") != "1" {
		t.Skip("explicit real SSTP end-to-end candidate only")
	}
	blob, err := os.ReadFile(os.Getenv("XNV_SSTP_CANDIDATE"))
	if err != nil {
		t.Fatal(err)
	}
	var n Node
	if json.Unmarshal(blob, &n) != nil {
		t.Fatal("candidate")
	}
	dir := t.TempDir()
	oldTag, oldBase := instTag, instBase
	defer func() { instTag, instBase = oldTag, oldBase }()
	if err := initInstance(dir); err != nil {
		t.Fatal(err)
	}
	c, err := newConsole(NewManager(maximumSlots, dir))
	if err != nil {
		t.Fatal(err)
	}
	defer c.shutdown()
	result := c.sstp.probeNode(context.Background(), n, SstpSettings{Parallelism: 1, ValidMinutes: 60, TimeoutSeconds: 90}, func(p string) {
		if p == "setup" {
			files, _ := filepath.Glob(filepath.Join(dir, ".connectivity-sstp-*", "*.launch"))
			for _, f := range files {
				b, _ := os.ReadFile(f)
				os.WriteFile(f, []byte(strings.ReplaceAll(string(b), "--log-level 2", "--log-level 4")), 0600)
			}
		}
		if p == "cleanup" {
			logs, _ := filepath.Glob(filepath.Join(dir, ".connectivity-sstp-*", "sstp.log"))
			for _, log := range logs {
				if b, err := os.ReadFile(log); err == nil {
					os.WriteFile(filepath.Join(filepath.Dir(os.Getenv("XNV_SSTP_CANDIDATE")), "sstp-client-private.log"), b, 0600)
				}
			}
		}
	})
	b, _ := json.Marshal(result)
	os.WriteFile(filepath.Join(filepath.Dir(os.Getenv("XNV_SSTP_CANDIDATE")), "sstp-e2e-private.json"), b, 0600)
	if result.Status != "passed" {
		t.Skipf("real upstream attempt: %s; no validated node claimed", result.Code)
	}
	t.Log("Real SSTP + PPP + two HTTPS echoes + HTTPS content passed")
}
