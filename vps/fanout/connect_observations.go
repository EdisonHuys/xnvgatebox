package main

import "time"

// Observations only affect selection order. Every new connection still passes
// the full isolation, policy and independent HTTPS/SOCKS checks.
func (m *Manager) recordConnectivity(r CheckResult) {
	if r.Host == "" || (r.Status != "passed" && r.Status != "failed") {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.observations == nil {
		m.observations = map[string]CheckResult{}
	}
	if old, ok := m.observations[r.Host]; !ok || !old.CheckedAt.After(r.CheckedAt) {
		m.observations[r.Host] = r
	}
}
func (m *Manager) connectivityRank(n Node) (int, float64) {
	r, ok := m.observations[n.HostName]
	if !ok || r.ProfileSHA != profileDigest(n.Config) || r.CheckedAt.IsZero() || time.Since(r.CheckedAt) < 0 || time.Since(r.CheckedAt) > 15*time.Minute {
		return 1, 0
	}
	if r.Status == "passed" {
		return 0, r.LatencyMS
	}
	if r.Status == "failed" {
		return 2, 0
	}
	return 1, 0
}
