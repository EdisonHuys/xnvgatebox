package main

import (
	"encoding/json"
	"time"
)

// A plain immutable view avoids copying mutexes or reading changing Node/Policy
// fields while a Slot operation is committing a new verified binding.
type tunnelView struct {
	Slot              int            `json:"slot"`
	Port              int            `json:"port"`
	Node              Node           `json:"node"`
	Phase             string         `json:"phase,omitempty"`
	Status            string         `json:"status"`
	ExitIP            string         `json:"exit_ip"`
	Err               string         `json:"err,omitempty"`
	Since             time.Time      `json:"since"`
	Cred              SocksCred      `json:"cred"`
	Policy            *SlotPolicy    `json:"policy,omitempty"`
	LatencyMS         float64        `json:"latency_ms,omitempty"`
	Egress            *EgressProfile `json:"egress"`
	CanRollbackPolicy bool           `json:"can_rollback_policy"`
}

func (t *Tunnel) snapshot() tunnelView {
	t.mu.Lock()
	defer t.mu.Unlock()
	view := tunnelView{Slot: t.Slot, Port: t.Port, Node: t.Node, Status: t.Status, Phase: t.probeStage, ExitIP: t.ExitIP, Err: t.Err,
		Since: t.Since, Cred: t.Cred, Policy: t.Policy, LatencyMS: t.LatencyMS, Egress: t.egressSnapshotLocked(false), CanRollbackPolicy: t.previousPolicy != nil}
	if view.Status == "up" && !t.acceptsSocksLocked() {
		view.Status, view.Err = "blocked", "当前出口证据不满足策略，等待重新验活"
	}
	return view
}

func (t *Tunnel) MarshalJSON() ([]byte, error) { return json.Marshal(t.snapshot()) }
