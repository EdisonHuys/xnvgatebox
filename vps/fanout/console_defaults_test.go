package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func defaultTargetFixture(t *testing.T) *Console {
	t.Helper()
	c := consoleFixture(t)
	usePanel(t, &fakePanel{})
	panelState.mu.Lock()
	panelState.forced = "native"
	panelState.mu.Unlock()
	t.Cleanup(c.cancel)
	return c
}

func TestConsoleDefaultTargetWaitsForPoolThenAllocatesOnce(t *testing.T) {
	c := defaultTargetFixture(t)
	if err := c.reconcile(); err != nil || c.plan != nil || len(c.m.Tunnels()) != 0 {
		t.Fatal("missing pool must not create or publish exits", err)
	}
	c.m.nodes = []Node{{HostName: "fixture.example.invalid", CountryCode: "US", Config: "fixture"}}
	if err := c.reconcile(); err != nil || c.plan == nil || c.plan.Count != 16 || len(c.m.Tunnels()) != 16 {
		t.Fatal("pool did not initialize default target", err)
	}
	if c.plan.Policy.Country != "" || !containsFold(c.plan.Policy.AllowedIPTypes, "unknown") {
		t.Fatal("default differs from the quantity form or invents residential evidence")
	}
	before := c.m.Tunnels()
	if err := c.reconcile(); err != nil || !reflect.DeepEqual(before, c.m.Tunnels()) { t.Fatal("duplicate initialization", err) }
	reloaded, err := newConsole(NewManager(maximumSlots, c.m.workDir))
	if err != nil || reloaded.plan == nil || reloaded.plan.Count != 16 { t.Fatal("default plan not persisted", err) }
	reloaded.cancel()
}

func TestConsoleDefaultTargetPreservesZeroCustomAndLegacySlots(t *testing.T) {
	for _, count := range []int{0, 4, 23} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			c := defaultTargetFixture(t)
			c.m.nodes = []Node{{HostName: "candidate", Config: "fixture"}}
			if err := c.applyTarget(ExitPlan{Count: count, Policy: defaultSlotPolicyForMode("JP", false)}); err != nil { t.Fatal(err) }
			before := c.m.Tunnels()
			if err := c.reconcile(); err != nil || c.plan.Count != count || !reflect.DeepEqual(before, c.m.Tunnels()) { t.Fatal("saved target overwritten", err) }
		})
	}
	c := defaultTargetFixture(t)
	legacy, err := c.start(Node{HostName: "legacy"}, defaultSlotPolicyForMode("KR", false))
	if err != nil { t.Fatal(err) }
	c.m.nodes = []Node{{HostName: "new"}}
	if err := c.reconcile(); err != nil || c.plan != nil || len(c.m.Tunnels()) != 1 || c.m.Tunnels()[0] != legacy { t.Fatal("legacy Slot silently expanded", err) }
}

func TestConsoleDefaultTargetCapacityAndNonNativeDoNotMutate(t *testing.T) {
	c := defaultTargetFixture(t)
	c.m.nodes = []Node{{HostName: "candidate"}}
	c.m.maxSlots = 8
	if c.reconcile() == nil || c.plan != nil || len(c.m.Tunnels()) != 0 { t.Fatal("capacity silently reduced or overrun") }
	panelState.mu.Lock(); panelState.forced = "3x-ui"; panelState.mu.Unlock()
	c.m.maxSlots = maximumSlots
	if err := c.reconcile(); err != nil || c.plan != nil || len(c.m.Tunnels()) != 0 { t.Fatal("non-native backend changed", err) }
}

func TestConsoleDefaultTargetPoolRefreshWakesWatch(t *testing.T) {
	c := defaultTargetFixture(t)
	dir := t.TempDir()
	pool := filepath.Join(dir, "node_pool.json")
	if err := os.WriteFile(filepath.Join(dir, "fixture.ovpn"), []byte("client\nremote 8.8.8.8 1194\n"), 0600); err != nil { t.Fatal(err) }
	if err := os.WriteFile(pool, []byte(`{"nodes":[{"hostname":"fixture.example.invalid","server_ip":"8.8.8.8","advertised_country":"US","protocols":{"openvpn":{"profile_ref":"fixture.ovpn"}}}]}`), 0600); err != nil { t.Fatal(err) }
	t.Setenv("FANOUT_VALIDATED_POOL_FILE", pool)
	bound := make(chan int, 1)
	c.ensure = func(ts []*Tunnel, _ string) error { select { case bound <- len(ts): default: }; return nil }
	done := make(chan struct{})
	go func() { c.watch(); close(done) }()
	t.Cleanup(func() { c.cancel(); <-done })
	if _, err := c.m.RefreshNodes(); err != nil { t.Fatal(err) }
	select {
	case n := <-bound:
		if n != 16 { t.Fatal("refresh did not bind default exits") }
	case <-time.After(2*time.Second):
		t.Fatal("refresh waited for polling instead of waking reconciliation")
	}
}
