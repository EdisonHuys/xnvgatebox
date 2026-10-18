package main

import (
	"context"
	"errors"
)

var errDiagnosticPreempted = errors.New("diagnostic yielded to exit recovery")

// A lease prevents a temporary login and a live exit login to the same host
// from displacing one another. Production can cancel a diagnostic, but must
// wait for all its network resources to be cleaned before using that host.
type diagnosticLease struct {
	cancel       context.CancelCauseFunc
	done         chan struct{}
	ip           string
	networkIndex int
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
	used := map[int]bool{}
	for _, t := range m.tunnels {
		used[slotNetworkIndex(t.Slot)] = true
	}
	for _, l := range m.diagnostics {
		used[l.networkIndex] = true
	}
	index := networkLinkCount - 1
	for index >= 0 && used[index] {
		index--
	}
	if index < 0 {
		return nil
	}
	lease := &diagnosticLease{cancel: cancel, done: make(chan struct{}), ip: node.IP, networkIndex: index}
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
