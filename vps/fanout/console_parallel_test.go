package main

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestConsoleTwoWorkersIsolateSlotsAndPublishOnlyAfterCompletion(t *testing.T) {
	c := consoleFixture(t)
	c.m.nodes = []Node{{HostName: "a"}, {HostName: "b"}, {HostName: "c"}}
	started := make(chan int, 3)
	release := make(chan struct{})
	var mu sync.Mutex
	active := map[int]bool{}
	c.probe = func(ctx context.Context, node Node, slot int, progress func(string)) CheckResult {
		mu.Lock()
		if active[slot] || slot < diagnosticFirstSlot || slot >= diagnosticFirstSlot+diagnosticWorkers {
			t.Error("diagnostics collided or entered user namespace")
		}
		active[slot] = true
		mu.Unlock()
		progress("connecting")
		started <- slot
		select {
		case <-release:
		case <-ctx.Done():
		}
		mu.Lock()
		delete(active, slot)
		mu.Unlock()
		return CheckResult{Host: node.HostName, Status: "passed"}
	}
	j, err := c.startCheck(CheckRequest{Kind: "candidates", Hosts: []string{"a", "b", "c"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.cancel() })
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("two workers did not start concurrently")
		}
	}
	j.mu.Lock()
	running, queued := 0, 0
	for _, item := range j.Items {
		if item.Status == "running" && item.Phase == "connecting" {
			running++
		}
		if item.Status == "queued" {
			queued++
		}
	}
	j.mu.Unlock()
	if running != 2 || queued != 1 || j.Parallelism != 2 {
		t.Fatal("wrong concurrency/progress", running, queued)
	}
	c.checksMu.Lock()
	observations := len(c.results)
	c.checksMu.Unlock()
	if observations != 0 {
		t.Fatal("incomplete preflight was presented as passed")
	}
	close(release)
	waitConsoleCheck(t, j)
	if j.Done != 3 || j.Status != "done" {
		t.Fatal("work lost")
	}
}

func TestConsoleCancellationWaitsForBothWorkerCleanups(t *testing.T) {
	c := consoleFixture(t)
	c.m.nodes = []Node{{HostName: "a"}, {HostName: "b"}, {HostName: "queued"}}
	started := make(chan struct{}, 2)
	cleanup := make(chan struct{})
	c.probe = func(ctx context.Context, n Node, slot int, progress func(string)) CheckResult {
		started <- struct{}{}
		<-ctx.Done()
		progress("cleanup")
		<-cleanup
		return CheckResult{Host: n.HostName, Status: "cancelled"}
	}
	j, err := c.startCheck(CheckRequest{Kind: "candidates", Hosts: []string{"a", "b", "queued"}})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("not parallel")
		}
	}
	j.cancel()
	select {
	case <-j.finished:
		t.Fatal("finished before cleanup")
	default:
	}
	if _, err := c.startCheck(CheckRequest{Kind: "candidates", Hosts: []string{"a"}}); err == nil {
		t.Fatal("new job reused diagnostic namespace before cleanup")
	}
	close(cleanup)
	waitConsoleCheck(t, j)
	<-j.finished
	if j.Done != 3 || j.Status != "cancelled" {
		t.Fatal("queued work was not cancelled")
	}
}

func TestConsoleParallelismValidationAndSerialOption(t *testing.T) {
	c := consoleFixture(t)
	c.m.nodes = []Node{{HostName: "a"}}
	for _, n := range []int{-1, 3, 100} {
		if _, err := c.startCheck(CheckRequest{Kind: "candidates", Hosts: []string{"a"}, Parallelism: n}); err == nil {
			t.Fatal("unbounded concurrency accepted")
		}
	}
	c.probe = func(context.Context, Node, int, func(string)) CheckResult { return CheckResult{Status: "passed"} }
	j, err := c.startCheck(CheckRequest{Kind: "candidates", Hosts: []string{"a"}, Parallelism: 1})
	if err != nil {
		t.Fatal(err)
	}
	waitConsoleCheck(t, j)
	if j.Parallelism != 1 {
		t.Fatal("serial setting ignored")
	}
}

func TestDiagnosticLeaseYieldsOnlyAfterCleanupAndMatchesIPAliases(t *testing.T) {
	m := NewManager(32, t.TempDir())
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	lease := m.reserveDiagnostic(Node{HostName: "diagnostic", IP: "192.0.2.20"}, cancel)
	if lease == nil {
		t.Fatal("lease missing")
	}
	if m.reserveDiagnostic(Node{HostName: "alias", IP: "192.0.2.20"}, cancel) != nil {
		t.Fatal("same upstream dialed twice")
	}
	tun := &Tunnel{Slot: 1, Node: Node{HostName: "old"}, Status: "starting"}
	tun.resetWork()
	m.tunnels[1] = tun
	claimed := make(chan bool, 1)
	go func() {
		claimed <- m.claimNode(tun, Node{HostName: "production-alias", IP: "192.0.2.20"}, tun.workContext())
	}()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("production did not preempt")
	}
	if context.Cause(ctx) != errDiagnosticPreempted {
		t.Fatal("wrong cancel cause")
	}
	select {
	case <-claimed:
		t.Fatal("production claimed before diagnostic cleanup")
	default:
	}
	m.releaseDiagnostic("diagnostic", lease)
	select {
	case ok := <-claimed:
		if !ok {
			t.Fatal("production claim failed")
		}
	case <-time.After(time.Second):
		t.Fatal("production remained blocked")
	}
	if m.reserveDiagnostic(Node{HostName: "different-name", IP: "192.0.2.20"}, cancel) != nil {
		t.Fatal("running upstream accepted for diagnostic")
	}
}

func TestCancelledProductionDoesNotInterruptDiagnostic(t *testing.T) {
	m := NewManager(32, t.TempDir())
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	lease := m.reserveDiagnostic(Node{HostName: "a"}, cancel)
	defer m.releaseDiagnostic("a", lease)
	production, stop := context.WithCancel(context.Background())
	stop()
	tun := &Tunnel{Slot: 1, Status: "starting"}
	m.tunnels[1] = tun
	if m.claimNode(tun, Node{HostName: "a"}, production) || ctx.Err() != nil {
		t.Fatal("cancelled production preempted useful work")
	}
}

func TestDiagnosticBusyResultKeepsLastCompletedObservation(t *testing.T) {
	c := consoleFixture(t)
	node := Node{HostName: "a", Config: "fixture"}
	c.m.nodes = []Node{node}
	c.m.tunnels[1] = &Tunnel{Slot: 1, Node: node, Status: "starting"}
	c.results["a"] = CheckResult{Host: "a", Status: "passed"}
	j, err := c.startCheck(CheckRequest{Kind: "candidates", Hosts: []string{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	waitConsoleCheck(t, j)
	if j.Items[0].Status != "deferred" || j.Items[0].Code != "candidate_in_use" || c.results["a"].Status != "passed" {
		t.Fatal("active upstream was redialed or previous completed observation lost")
	}
}

func TestPreflightConservativeTCPParsing(t *testing.T) {
	for _, cfg := range []string{"proto udp\nremote 192.0.2.1 1194", "remote example.invalid 443\nproto tcp", "remote 192.0.2.1 443\nproto tcp\n<connection>\nremote 192.0.2.2 443\n</connection>", "proto tcp\nremote 192.0.2.1 0", "proto tcp\nremote 192.0.2.1 443 udp"} {
		if len(preflightEndpoints(cfg)) != 0 {
			t.Fatal("unsupported profile was incorrectly treated as TCP-only")
		}
	}
	got := preflightEndpoints("<ca>\nremote ignored 0\n</ca>\nremote 192.0.2.1 443\nproto tcp-client")
	if len(got) != 1 || got[0] != "192.0.2.1:443" {
		t.Fatal("simple TCP profile not parsed")
	}
}

func TestPreflightFallbackCancellationAndNoAvailabilityClaim(t *testing.T) {
	calls := 0
	err := preflightDial(context.Background(), []string{"first", "second"}, func(context.Context, string) error {
		calls++
		if calls == 1 {
			return errors.New("closed")
		}
		return nil
	})
	if err != nil || calls != 2 {
		t.Fatal("alternative endpoint was not tried")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if preflightDial(ctx, []string{"never"}, func(context.Context, string) error { t.Fatal("dialed after cancellation"); return nil }) == nil {
		t.Fatal("cancel ignored")
	}
}

func TestTCPPreflightCannotPassAFullCheckOrConsumeRecoveryPermits(t *testing.T) {
	c := consoleFixture(t)
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			conn.Close()
		}
	}()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ip"), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	c.m.connectSem <- struct{}{}
	c.m.connectSem <- struct{}{}
	defer func() { <-c.m.connectSem; <-c.m.connectSem }()
	node := Node{HostName: "tcp-only", Config: "proto tcp\nremote 127.0.0.1 " + strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := c.probeCandidate(ctx, node, diagnosticFirstSlot, func(string) {})
	if result.Status != "failed" || result.Code != "probe_environment_failed" || ctx.Err() != nil {
		t.Fatal("preflight substituted for full verification or waited on production permits", result.Status, result.Code)
	}
	if len(c.m.diagnostics) != 0 {
		t.Fatal("failed diagnostic leaked its host lease")
	}
}

func TestCandidateReuseCannotPassForAReplacedUpstream(t *testing.T) {
	c := consoleFixture(t)
	c.m.tunnels[1] = &Tunnel{Slot: 1, Node: Node{HostName: "new"}, Status: "up"}
	r := c.checkExit(context.Background(), 1, "old", func(string) {})
	if r.Status != "deferred" || r.Code != "candidate_changed" || r.Host != "old" {
		t.Fatal("a replacement upstream was presented as the requested candidate")
	}
}
