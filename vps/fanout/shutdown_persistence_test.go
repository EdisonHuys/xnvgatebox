package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func shutdownFixture(t *testing.T) *Manager {
	t.Helper()
	m := NewManager(maximumSlots, t.TempDir())
	for slot := 1; slot <= 3; slot++ {
		status := "up"
		if slot == 3 {
			status = "stopped"
		}
		m.tunnels[slot] = &Tunnel{Slot: slot, Port: 17927 + slot, Status: status, Node: Node{HostName: "fixture"}}
	}
	if err := m.saveState(); err != nil {
		t.Fatal(err)
	}
	return m
}

func assertShutdownIntent(t *testing.T, m *Manager) {
	t.Helper()
	blob, err := os.ReadFile(statePath(m.workDir))
	if err != nil {
		t.Fatal(err)
	}
	var state persistedState
	if err := json.Unmarshal(blob, &state); err != nil {
		t.Fatal(err)
	}
	for _, row := range state.Tunnels {
		if row.Stopped != (row.Slot == 3) {
			t.Fatal("transient shutdown replaced operator intent", row.Slot)
		}
	}
}

func TestShutdownPersistenceRejectsBackgroundWritesDuringPartialTeardown(t *testing.T) {
	m := shutdownFixture(t)
	m.shutdownWithStop(func(tn *Tunnel) {
		tn.mu.Lock()
		tn.Status = "stopped"
		tn.mu.Unlock()
		if m.saveState() == nil {
			t.Error("background writer accepted partial shutdown")
		}
		assertShutdownIntent(t, m)
	})
	assertShutdownIntent(t, m)
}

func TestShutdownWaitsForExistingCommitBeforeStoppingAnyTunnel(t *testing.T) {
	m := shutdownFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	m.stateWriteHook = func(string) error { close(entered); <-release; return nil }
	saved := make(chan error, 1)
	go func() { saved <- m.saveState() }()
	<-entered
	stopping := make(chan struct{}, 3)
	done := make(chan struct{})
	go func() {
		m.shutdownWithStop(func(tn *Tunnel) { stopping <- struct{}{}; tn.mu.Lock(); tn.Status = "stopped"; tn.mu.Unlock() })
		close(done)
	}()
	select {
	case <-stopping:
		t.Fatal("shutdown raced in-flight commit")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-saved; err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not complete")
	}
	assertShutdownIntent(t, m)
	if m.saveState() == nil {
		t.Fatal("writer accepted after shutdown")
	}
}
