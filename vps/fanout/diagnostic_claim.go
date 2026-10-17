package main

import (
	"context"
	"errors"
)

const diagnosticFirstSlot = 240
const diagnosticWorkers = 2

var errDiagnosticPreempted = errors.New("diagnostic yielded to exit recovery")

// A lease prevents a temporary login and a live exit login to the same host
// from displacing one another. Production can cancel a diagnostic, but must
// wait for all its network resources to be cleaned before using that host.
type diagnosticLease struct {
	cancel context.CancelCauseFunc
	done   chan struct{}
	ip     string
}

func (m *Manager) reserveDiagnostic(node Node, cancel context.CancelCauseFunc) *diagnosticLease {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tunnels {
		t.mu.Lock()
		occupied := t.Status != "stopped" && (t.Node.HostName == node.HostName || (node.IP != "" && t.Node.IP == node.IP))
		t.mu.Unlock()
		if occupied {
			return nil
		}
	}
	if m.diagnostics == nil {
		m.diagnostics = map[string]*diagnosticLease{}
	}
	if m.diagnostics[node.HostName] != nil {
		return nil
	}
	for _, existing := range m.diagnostics {
		if node.IP != "" && existing.ip == node.IP {
			return nil
		}
	}
	lease := &diagnosticLease{cancel: cancel, done: make(chan struct{}), ip: node.IP}
	m.diagnostics[node.HostName] = lease
	return lease
}

func (m *Manager) releaseDiagnostic(host string, lease *diagnosticLease) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.diagnostics[host] == lease {
		delete(m.diagnostics, host)
		close(lease.done)
	}
}

func (t *Tunnel) probePhase(phase string) {
	if t.onProbePhase != nil {
		t.onProbePhase(phase)
	}
}
