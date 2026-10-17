package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type CheckResult struct {
	Host       string    `json:"host"`
	Slot       int       `json:"slot,omitempty"`
	Status     string    `json:"status"`
	Phase      string    `json:"phase,omitempty"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	Code       string    `json:"code,omitempty"`
	ActualIP   string    `json:"actual_ip,omitempty"`
	Country    string    `json:"country,omitempty"`
	IPType     string    `json:"ip_type,omitempty"`
	LatencyMS  float64   `json:"latency_ms,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
	ProfileSHA string    `json:"profile_sha256"`
	Stale      bool      `json:"stale"`
}

type CheckRun struct {
	finished    chan struct{}
	mu          sync.Mutex
	ID          string        `json:"id"`
	Status      string        `json:"status"`
	Kind        string        `json:"kind"`
	Items       []CheckResult `json:"items"`
	Done        int           `json:"done"`
	Total       int           `json:"total"`
	Parallelism int           `json:"parallelism"`
	cancel      context.CancelFunc
}

func (j *CheckRun) view() any {
	j.mu.Lock()
	defer j.mu.Unlock()
	return struct {
		ID          string        `json:"id"`
		Status      string        `json:"status"`
		Kind        string        `json:"kind"`
		Items       []CheckResult `json:"items"`
		Done        int           `json:"done"`
		Total       int           `json:"total"`
		Parallelism int           `json:"parallelism"`
	}{j.ID, j.Status, j.Kind, append([]CheckResult(nil), j.Items...), j.Done, j.Total, j.Parallelism}
}

func checkFailure(err error, ctx context.Context) string {
	if ctx.Err() != nil {
		if ctx.Err() == context.Canceled {
			return "cancelled"
		}
		return "check_timeout"
	}
	text := err.Error()
	switch {
	case strings.Contains(text, "tun0"):
		return "openvpn_timeout"
	case strings.Contains(text, "openvpn"):
		return "openvpn_failed"
	case strings.Contains(text, "画像"):
		return "evidence_unavailable"
	case strings.Contains(text, "出口") && strings.Contains(text, "不一致"):
		return "exit_ip_mismatch"
	case strings.Contains(text, "namespace") || strings.Contains(text, "netns"):
		return "probe_environment_failed"
	default:
		return "https_unreachable"
	}
}

// Each worker reserves a distinct diagnostic Slot outside user capacity. The
// job bounds diagnostics to two and leaves production's two dial permits free.
func (c *Console) probeCandidate(parent context.Context, node Node, slot int, progress func(string)) CheckResult {
	result := CheckResult{Host: node.HostName, Status: "failed", IPType: "unknown", ProfileSHA: profileDigest(node.Config)}
	if slot < diagnosticFirstSlot || slot >= diagnosticFirstSlot+diagnosticWorkers {
		result.Code = "probe_environment_failed"
		return result
	}
	leaseCtx, cancelLease := context.WithCancelCause(parent)
	defer cancelLease(nil)
	lease := c.m.reserveDiagnostic(node, cancelLease)
	if lease == nil {
		result.Status, result.Code = "deferred", "candidate_in_use"
		return result
	}
	// Registered before resource defers: production cannot claim the same host
	// until the temporary OpenVPN, namespace, SOCKS and files are all gone.
	defer c.m.releaseDiagnostic(node.HostName, lease)
	ctx, cancel := context.WithTimeout(leaseCtx, 120*time.Second)
	defer cancel()
	progress("precheck")
	if err := preflightCandidate(ctx, node); err != nil {
		result.Code = "tcp_unreachable"
		if ctx.Err() != nil {
			result.Code = checkFailure(err, ctx)
			if result.Code == "cancelled" {
				result.Status = "cancelled"
			}
		}
		if context.Cause(leaseCtx) == errDiagnosticPreempted {
			result.Status, result.Code = "deferred", "exit_recovery_priority"
		}
		return result
	}
	dir, err := os.MkdirTemp(c.m.workDir, ".connectivity-")
	if err != nil {
		result.Code = "probe_environment_failed"
		return result
	}
	defer os.RemoveAll(dir)
	cred, err := newSocksCred()
	if err != nil {
		result.Code = "probe_environment_failed"
		return result
	}
	port, err := freeRandomPort(map[int]bool{})
	if err != nil {
		result.Code = "probe_environment_failed"
		return result
	}
	policy := SlotPolicy{AllowedIPTypes: []string{"strict_residential", "likely_residential", "mobile", "datacenter", "business_isp", "unknown"}}
	t := &Tunnel{Slot: slot, Port: port, Node: node, Status: "starting", Cred: cred, Policy: &policy, workCtx: ctx, cancelWork: cancel, onProbePhase: progress}
	helper := NewManager(1, dir)
	helper.tunnels[t.Slot] = t
	defer func() { progress("cleanup"); t.stop(); _ = os.RemoveAll(filepath.Join("/etc/netns", t.nsName())) }()
	if err := helper.tryNode(t); err != nil {
		result.Code = checkFailure(err, ctx)
		if result.Code == "cancelled" {
			result.Status = "cancelled"
		}
		if context.Cause(leaseCtx) == errDiagnosticPreempted {
			result.Status, result.Code = "deferred", "exit_recovery_priority"
		}
		return result
	}
	// tryNode independently matches two namespace echoes with two SOCKS echoes
	// and rejects the host IP. Its accepted country may still be unknown.
	t.mu.Lock()
	result.ActualIP = t.ExitIP
	result.LatencyMS = t.LatencyMS
	if t.egress != nil {
		if t.egress.IntelligenceStatus == "passed" {
			result.Country = stringValue(t.egress.Country)
		}
		result.IPType = t.egress.IPType
	}
	t.mu.Unlock()
	result.Status = "passed"
	return result
}

func (c *Console) checkExit(ctx context.Context, slot int, expectedHost string, progress func(string)) CheckResult {
	c.m.mu.RLock()
	t := c.m.tunnels[slot]
	c.m.mu.RUnlock()
	result := CheckResult{Slot: slot, Status: "failed", Code: "exit_not_ready", IPType: "unknown"}
	if t == nil {
		return result
	}
	progress("waiting_exit")
	for !t.opMu.TryLock() {
		select {
		case <-ctx.Done():
			result.Status = "cancelled"
			result.Code = "cancelled"
			return result
		case <-time.After(20 * time.Millisecond):
		}
	}
	defer t.opMu.Unlock()
	v := t.snapshot()
	result.Host = v.Node.HostName
	result.ProfileSHA = profileDigest(v.Node.Config)
	if expectedHost != "" && v.Node.HostName != expectedHost {
		result.Host, result.Status, result.Code = expectedHost, "deferred", "candidate_changed"
		return result
	}
	if v.Status != "up" {
		return result
	}
	probeCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	progress("verify_exit")
	ip, latency, err := t.probeSocksWithContext(probeCtx)
	if err != nil {
		result.Code = checkFailure(err, probeCtx)
		if result.Code == "cancelled" {
			result.Status = "cancelled"
		}
		return result
	}
	if ip == hostPublicIP() || ip != v.ExitIP {
		t.blockForPolicy("检测到实际出口变化，保持阻断")
		result.Code = "exit_ip_mismatch"
		return result
	}
	if err := t.policySnapshot().checkLatency(latency); err != nil {
		result.Code = "policy_latency_exceeded"
		return result
	}
	result.Status, result.Code, result.ActualIP, result.LatencyMS = "passed", "", ip, latency
	if v.Egress != nil {
		if v.Egress.IntelligenceStatus == "passed" {
			result.Country = stringValue(v.Egress.Country)
		}
		result.IPType = v.Egress.IPType
	}
	return result
}

type CheckRequest struct {
	Kind        string   `json:"kind"`
	Hosts       []string `json:"hosts"`
	Slots       []int    `json:"slots"`
	Parallelism int      `json:"parallelism,omitempty"`
}

func (c *Console) startCheck(in CheckRequest) (*CheckRun, error) {
	if in.Kind != "candidates" && in.Kind != "exits" {
		return nil, fmt.Errorf("检测类型无效")
	}
	if in.Parallelism == 0 {
		in.Parallelism = diagnosticWorkers
	}
	if in.Parallelism < 1 || in.Parallelism > diagnosticWorkers {
		return nil, fmt.Errorf("同时检测数量须为 1-%d", diagnosticWorkers)
	}
	nodes, _ := c.m.Nodes()
	byHost := map[string]Node{}
	for _, n := range nodes {
		byHost[n.HostName] = n
	}
	items := []CheckResult{}
	seen := map[string]bool{}
	if in.Kind == "candidates" {
		if len(in.Hosts) < 1 || len(in.Hosts) > 200 {
			return nil, fmt.Errorf("请选择 1-200 个候选")
		}
		for _, host := range in.Hosts {
			n, ok := byHost[host]
			if !ok {
				return nil, fmt.Errorf("候选已从当前池消失，请刷新列表")
			}
			if !seen[host] {
				items = append(items, CheckResult{Host: host, Status: "queued", ProfileSHA: profileDigest(n.Config)})
				seen[host] = true
			}
		}
	} else {
		if len(in.Slots) < 1 || len(in.Slots) > maximumSlots {
			return nil, fmt.Errorf("请选择运行出口")
		}
		for _, slot := range in.Slots {
			key := fmt.Sprint(slot)
			c.m.mu.RLock()
			t := c.m.tunnels[slot]
			c.m.mu.RUnlock()
			if t == nil {
				return nil, fmt.Errorf("出口不存在")
			}
			if !seen[key] {
				items = append(items, CheckResult{Slot: slot, Status: "queued"})
				seen[key] = true
			}
		}
	}
	c.checksMu.Lock()
	defer c.checksMu.Unlock()
	if c.ctx.Err() != nil {
		return nil, fmt.Errorf("管理服务正在停止")
	}
	if c.check != nil {
		c.check.mu.Lock()
		running := c.check.Status == "running"
		c.check.mu.Unlock()
		if running {
			return nil, fmt.Errorf("已有检测任务，请等待或取消")
		}
	}
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(c.ctx)
	job := &CheckRun{ID: hex.EncodeToString(buf), Status: "running", Kind: in.Kind, Items: items, Total: len(items), Parallelism: in.Parallelism, cancel: cancel, finished: make(chan struct{})}
	c.check = job
	go c.runChecks(ctx, job, byHost)
	return job, nil
}

func (c *Console) runChecks(ctx context.Context, j *CheckRun, nodes map[string]Node) {
	defer close(j.finished)
	defer j.cancel()
	next := 0
	var workers sync.WaitGroup
	for worker := 0; worker < min(j.Parallelism, len(j.Items)); worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for {
				j.mu.Lock()
				i := next
				next++
				j.mu.Unlock()
				if i >= len(j.Items) {
					return
				}
				j.mu.Lock()
				item := j.Items[i]
				if ctx.Err() != nil {
					j.Items[i].Status = "cancelled"
					j.Done++
					j.mu.Unlock()
					continue
				}
				j.Items[i].Status = "running"
				j.Items[i].StartedAt = time.Now().UTC()
				j.mu.Unlock()
				progress := func(phase string) {
					j.mu.Lock()
					j.Items[i].Phase = phase
					j.mu.Unlock()
				}
				var result CheckResult
				if j.Kind == "candidates" {
					// Reuse a running candidate's real SOCKS check. A second OpenVPN
					// login to the same upstream can displace the user's active session.
					reused := false
					for _, t := range c.m.Tunnels() {
						v := t.snapshot()
						if v.Node.HostName == item.Host && v.Status == "up" {
							result = c.checkExit(ctx, v.Slot, item.Host, progress)
							reused = true
							break
						}
					}
					if !reused {
						result = c.probe(ctx, nodes[item.Host], diagnosticFirstSlot+worker, progress)
					}
				} else {
					result = c.checkExit(ctx, item.Slot, "", progress)
				}
				result.CheckedAt = time.Now().UTC()
				if ctx.Err() != nil {
					result.Status, result.Code = "cancelled", "cancelled"
				}
				j.mu.Lock()
				result.StartedAt = j.Items[i].StartedAt
				j.Items[i] = result
				j.Done++
				j.mu.Unlock()
				// Never let a cancelled/incomplete probe replace a completed observation.
				if (result.Status == "passed" || result.Status == "failed") && result.Host != "" {
					c.checksMu.Lock()
					if len(c.results) >= 1000 {
						c.results = map[string]CheckResult{}
					}
					c.results[result.Host] = result
					if blob, err := json.Marshal(c.results); err == nil {
						_ = atomicConfigWrite(filepath.Join(c.m.workDir, "checks.json"), blob)
					}
					c.checksMu.Unlock()
				}
			}
		}(worker)
	}
	workers.Wait()

	j.mu.Lock()
	j.Status = "done"
	if ctx.Err() != nil {
		j.Status = "cancelled"
	}
	j.mu.Unlock()
}

func (c *Console) apiCheck(w http.ResponseWriter, r *http.Request) {
	var in CheckRequest
	if err := readConsoleJSON(w, r, &in); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	job, err := c.startCheck(in)
	if err != nil {
		writeJSON(w, 409, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 202, job.view())
}

func (c *Console) apiCheckCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	c.checksMu.Lock()
	j := c.check
	c.checksMu.Unlock()
	if j == nil {
		writeJSON(w, 404, map[string]string{"error": "没有检测任务"})
		return
	}
	j.cancel()
	writeJSON(w, 202, map[string]string{"status": "cancelling"})
}
