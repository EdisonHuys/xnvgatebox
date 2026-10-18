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
	Priority   bool      `json:"priority,omitempty"`
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
	ctx         context.Context
	wake        chan struct{}
	nodes       map[string]Node
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
// job uses the saved operator limit and leaves production dial permits free.
func (c *Console) probeCandidate(parent context.Context, node Node, slot int, progress func(string)) CheckResult {
	result := CheckResult{Host: node.HostName, Status: "failed", IPType: "unknown", ProfileSHA: profileDigest(node.Config)}
	if slot > diagnosticFirstSlot || slot < -networkLinkCount {
		result.Code = "probe_environment_failed"
		return result
	}
	leaseCtx, cancelLease := context.WithCancelCause(parent)
	defer cancelLease(nil)
	lease := c.m.reserveDiagnostic(node, cancelLease)
	if lease == nil {
		result.Status, result.Code = "deferred", "candidate_in_use"
		c.m.mu.RLock()
		full := len(c.m.tunnels)+len(c.m.diagnostics) >= networkLinkCount
		c.m.mu.RUnlock()
		if full {
			result.Status, result.Code = "failed", "network_capacity_exhausted"
		}
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
	index := lease.networkIndex
	manifest, _ := json.Marshal(diagnosticResource{Slot: slot, NetworkIndex: index, Node: node})
	if err := atomicConfigWrite(filepath.Join(dir, "diagnostic.json"), manifest); err != nil {
		result.Code = "probe_environment_failed"
		return result
	}
	t := &Tunnel{networkIndex: &index, Slot: slot, Port: port, Node: node, Status: "starting", Cred: cred, Policy: &policy, workCtx: ctx, cancelWork: cancel, onProbePhase: progress}
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
	Priority    bool     `json:"priority,omitempty"`
	All         bool     `json:"all,omitempty"`
}

func checkItemKey(i CheckResult) string {
	if i.Host != "" {
		return "host:" + i.Host
	}
	return fmt.Sprintf("slot:%d", i.Slot)
}

func (c *Console) startCheck(in CheckRequest) (*CheckRun, error) {
	if in.Kind != "" && in.Kind != "checks" && in.Kind != "candidates" && in.Kind != "exits" {
		return nil, fmt.Errorf("检测类型无效")
	}
	nodes, _ := c.m.Nodes()
	byHost := map[string]Node{}
	for _, n := range nodes {
		byHost[n.HostName] = n
	}
	if in.All {
		for _, n := range nodes {
			in.Hosts = append(in.Hosts, n.HostName)
		}
	}
	items := []CheckResult{}
	seen := map[string]bool{}
	add := func(i CheckResult) {
		k := checkItemKey(i)
		if !seen[k] {
			seen[k] = true
			i.Status = "queued"
			i.Priority = in.Priority
			items = append(items, i)
		}
	}
	for _, host := range in.Hosts {
		n, ok := byHost[host]
		if !ok {
			return nil, fmt.Errorf("候选已从当前池消失，请刷新列表")
		}
		add(CheckResult{Host: host, ProfileSHA: profileDigest(n.Config)})
	}
	for _, slot := range in.Slots {
		c.m.mu.RLock()
		t := c.m.tunnels[slot]
		c.m.mu.RUnlock()
		if t == nil {
			return nil, fmt.Errorf("出口不存在")
		}
		v := t.snapshot()
		add(CheckResult{Slot: slot, Host: v.Node.HostName, ProfileSHA: profileDigest(v.Node.Config)})
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("请选择要检测的节点")
	}
	c.checksMu.Lock()
	defer c.checksMu.Unlock()
	if c.ctx.Err() != nil {
		return nil, fmt.Errorf("管理服务正在停止")
	}
	if in.Parallelism == 0 {
		in.Parallelism = c.checkParallelism
	}
	if err := validateCheckParallelism(in.Parallelism); err != nil {
		return nil, err
	}
	if c.check != nil {
		j := c.check
		j.mu.Lock()
		if j.Status == "running" {
			if !in.Priority || j.ctx.Err() != nil {
				j.mu.Unlock()
				return nil, fmt.Errorf("已有检测任务，请等待或取消")
			}
			for _, item := range items {
				duplicate := false
				for i := range j.Items {
					old := &j.Items[i]
					if checkItemKey(*old) == checkItemKey(item) && (old.Status == "queued" || old.Status == "running") {
						old.Priority = true
						duplicate = true
						break
					}
				}
				if !duplicate {
					j.Items = append(j.Items, item)
					j.Total++
				}
			}
			for k, n := range byHost {
				j.nodes[k] = n
			}
			j.mu.Unlock()
			select {
			case j.wake <- struct{}{}:
			default:
			}
			return j, nil
		}
		j.mu.Unlock()
	}
	if in.Parallelism != c.checkParallelism {
		if err := c.saveCheckSettings(in.Parallelism); err != nil {
			return nil, err
		}
	}
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(c.ctx)
	job := &CheckRun{ID: hex.EncodeToString(buf), Status: "running", Kind: "checks", Items: items, Total: len(items), Parallelism: in.Parallelism, cancel: cancel, ctx: ctx, finished: make(chan struct{}), wake: make(chan struct{}, 1), nodes: byHost}
	c.check = job
	go c.runChecks(ctx, job)
	return job, nil
}

// One dispatcher owns all running diagnostic identities. Manual items can join
// a batch and take the next free permit without exceeding its configured limit.
func (c *Console) runChecks(ctx context.Context, j *CheckRun) {
	defer close(j.finished)
	defer j.cancel()
	finished := make(chan int, j.Parallelism)
	active := map[int]bool{}
	cancelSignal := ctx.Done()
	for {
		j.mu.Lock()
		for len(active) < j.Parallelism && ctx.Err() == nil {
			next := -1
			for i, item := range j.Items {
				if item.Status == "queued" && (next < 0 || item.Priority && !j.Items[next].Priority) {
					next = i
				}
			}
			if next < 0 {
				break
			}
			slot := diagnosticFirstSlot
			for active[slot] {
				slot--
			}
			active[slot] = true
			j.Items[next].Status = "running"
			j.Items[next].StartedAt = time.Now().UTC()
			item := j.Items[next]
			node := j.nodes[item.Host]
			go func(index, slot int, item CheckResult, node Node) {
				c.runCheckItem(ctx, j, index, item, node, slot)
				finished <- slot
			}(next, slot, item, node)
		}
		if ctx.Err() != nil {
			for i := range j.Items {
				if j.Items[i].Status == "queued" {
					j.Items[i].Status = "cancelled"
					j.Done++
				}
			}
		}
		idle := len(active) == 0
		j.mu.Unlock()
		if idle {
			// Serialize completion with startCheck's append, so no late manual item
			// can be accepted after the last worker has gone away.
			c.checksMu.Lock()
			j.mu.Lock()
			queued := false
			for _, i := range j.Items {
				if i.Status == "queued" {
					queued = true
					break
				}
			}
			if !queued {
				j.Status = "done"
				if ctx.Err() != nil {
					j.Status = "cancelled"
				}
				j.mu.Unlock()
				c.checksMu.Unlock()
				return
			}
			j.mu.Unlock()
			c.checksMu.Unlock()
			continue
		}
		select {
		case slot := <-finished:
			delete(active, slot)
		case <-j.wake:
		case <-cancelSignal:
			cancelSignal = nil
		}
	}
}

func (c *Console) runCheckItem(ctx context.Context, j *CheckRun, index int, item CheckResult, node Node, slot int) {
	progress := func(phase string) { j.mu.Lock(); j.Items[index].Phase = phase; j.mu.Unlock() }
	var result CheckResult
	if item.Slot > 0 {
		result = c.checkExit(ctx, item.Slot, item.Host, progress)
	} else {
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
			result = c.probe(ctx, node, slot, progress)
		}
	}
	result.CheckedAt = time.Now().UTC()
	result.StartedAt = item.StartedAt
	result.Priority = item.Priority
	if result.Host == "" {
		result.Host = item.Host
	}
	if result.Slot == 0 && item.Slot > 0 {
		result.Slot = item.Slot
	}
	if ctx.Err() != nil {
		result.Status, result.Code = "cancelled", "cancelled"
	}
	// Only completed observations replace previous evidence; no count-based
	// truncation discards a large user's results.
	if (result.Status == "passed" || result.Status == "failed") && result.Host != "" {
		c.checksMu.Lock()
		c.results[result.Host] = result
		if blob, err := json.Marshal(c.results); err == nil {
			_ = atomicConfigWrite(filepath.Join(c.m.workDir, "checks.json"), blob)
		}
		c.checksMu.Unlock()
	}
	j.mu.Lock()
	j.Items[index] = result
	j.Done++
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
