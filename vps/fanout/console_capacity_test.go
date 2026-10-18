package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNetworkLinksPreserveLegacyAndCoverInstanceWithoutOverlap(t *testing.T) {
	seen := map[int]int{}
	for slot := 1; slot <= maximumSlots; slot++ {
		index := slotNetworkIndex(slot)
		if index < 0 || index >= networkLinkCount {
			t.Fatal("invalid link", slot, index)
		}
		if old := seen[index]; old != 0 {
			t.Fatal("network collision", old, slot)
		}
		seen[index] = slot
		tn := &Tunnel{Slot: slot}
		ip, block, err := net.ParseCIDR(tn.linkCIDR())
		if err != nil || !block.Contains(net.ParseIP(tn.linkAddress(1))) || !block.Contains(net.ParseIP(tn.linkAddress(2))) || !ip.Equal(block.IP) {
			t.Fatal("invalid /30", slot)
		}
		if slot <= 255 && tn.linkCIDR() != fmt.Sprintf("10.%d.%d.0/30", instBase, slot) {
			t.Fatal("legacy link changed", slot)
		}
	}
	if len(seen) != networkLinkCount || slotNetworkIndex(maximumSlots+1) != -1 {
		t.Fatal("address capacity incorrect")
	}
}

func TestDiagnostic128LinksExcludeUserIdentities(t *testing.T) {
	m := NewManager(maximumSlots, t.TempDir())
	used := map[int]bool{}
	for _, slot := range []int{1, 32, 128, 240, 241, 256, maximumSlots} {
		m.tunnels[slot] = &Tunnel{Slot: slot, Status: "stopped"}
		used[slotNetworkIndex(slot)] = true
	}
	for i := 0; i < 128; i++ {
		lease := m.reserveDiagnostic(Node{HostName: fmt.Sprint(i)}, func(error) {})
		if lease == nil || used[lease.networkIndex] {
			t.Fatal("link collision", i)
		}
		used[lease.networkIndex] = true
		id := diagnosticFirstSlot - i
		tn := &Tunnel{Slot: id, networkIndex: &lease.networkIndex}
		a, b := tn.vethNames()
		if len(a) > 15 || len(b) > 15 || slotNetworkIndex(id) != -1 {
			t.Fatal("diagnostic identity is not isolated")
		}
	}
}

func TestProductionNetworkClaimPreemptsDiagnosticEvenForDifferentHost(t *testing.T) {
	m := NewManager(maximumSlots, t.TempDir())
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	lease := m.reserveDiagnostic(Node{HostName: "diagnostic"}, cancel)
	tn := &Tunnel{Slot: maximumSlots, Status: "starting"}
	tn.resetWork()
	m.tunnels[tn.Slot] = tn
	claimed := make(chan bool, 1)
	go func() { claimed <- m.claimNode(tn, Node{HostName: "production"}, tn.workContext()) }()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("production did not preempt occupied link")
	}
	select {
	case <-claimed:
		t.Fatal("production raced cleanup")
	default:
	}
	m.releaseDiagnostic("diagnostic", lease)
	select {
	case ok := <-claimed:
		if !ok {
			t.Fatal("claim failed")
		}
	case <-time.After(time.Second):
		t.Fatal("claim stuck")
	}
}

func TestConsole128ConfiguredWorkersAndSettingsPersistence(t *testing.T) {
	c := consoleFixture(t)
	started := make(chan int, 128)
	release := make(chan struct{})
	var mu sync.Mutex
	active := map[int]bool{}
	for i := 0; i < 128; i++ {
		c.m.nodes = append(c.m.nodes, Node{HostName: fmt.Sprint(i)})
	}
	c.probe = func(ctx context.Context, n Node, slot int, p func(string)) CheckResult {
		mu.Lock()
		if active[slot] || slot >= 0 {
			t.Error("identity reused")
		}
		active[slot] = true
		mu.Unlock()
		started <- slot
		select {
		case <-release:
		case <-ctx.Done():
		}
		mu.Lock()
		delete(active, slot)
		mu.Unlock()
		return CheckResult{Host: n.HostName, Status: "passed"}
	}
	j, err := c.startCheck(CheckRequest{All: true, Parallelism: 128})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.cancel() })
	for i := 0; i < 128; i++ {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("configured concurrency silently reduced", i)
		}
	}
	if j.Parallelism != 128 {
		t.Fatal("configured count changed")
	}
	close(release)
	waitConsoleCheck(t, j)
	<-j.finished
	again, err := newConsole(NewManager(maximumSlots, c.m.workDir))
	if err != nil || again.checkParallelism != 128 {
		t.Fatal("configuration not persisted", err)
	}
}

func TestConsoleManualPriorityJoinsBatchAndDeduplicates(t *testing.T) {
	c := consoleFixture(t)
	c.m.nodes = []Node{{HostName: "a"}, {HostName: "b"}, {HostName: "manual"}}
	started := make(chan string, 4)
	release := make(chan struct{}, 4)
	c.probe = func(ctx context.Context, n Node, _ int, _ func(string)) CheckResult {
		started <- n.HostName
		select {
		case <-release:
		case <-ctx.Done():
		}
		return CheckResult{Host: n.HostName, Status: "passed"}
	}
	j, err := c.startCheck(CheckRequest{Hosts: []string{"a", "b"}, Parallelism: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer j.cancel()
	if <-started != "a" {
		t.Fatal("unexpected first item")
	}
	manual := CheckRequest{Hosts: []string{"manual"}, Priority: true}
	same, err := c.startCheck(manual)
	if err != nil || same != j {
		t.Fatal("manual was refused", err)
	}
	if _, err = c.startCheck(manual); err != nil {
		t.Fatal(err)
	}
	j.mu.Lock()
	total := j.Total
	j.mu.Unlock()
	if total != 3 {
		t.Fatal("duplicate enqueued", total)
	}
	release <- struct{}{}
	select {
	case host := <-started:
		if host != "manual" {
			t.Fatal("manual did not take next permit", host)
		}
	case <-time.After(time.Second):
		t.Fatal("manual stuck")
	}
	release <- struct{}{}
	if <-started != "b" {
		t.Fatal("batch item lost")
	}
	release <- struct{}{}
	waitConsoleCheck(t, j)
	<-j.finished
	if j.Done != 3 {
		t.Fatal("append lost during completion")
	}
}

func TestConsoleFullSourceAndTargetBeyond32(t *testing.T) {
	c := consoleFixture(t)
	t.Cleanup(c.shutdown)
	for i := 0; i < 321; i++ {
		c.m.nodes = append(c.m.nodes, Node{HostName: fmt.Sprint(i)})
	}
	w := httptest.NewRecorder()
	apiNodes(c.m)(w, httptest.NewRequest("GET", "/api/nodes?all=1", nil))
	var view struct {
		Nodes []Node `json:"nodes"`
	}
	if json.Unmarshal(w.Body.Bytes(), &view) != nil || len(view.Nodes) != 321 {
		t.Fatal("source was truncated")
	}
	if err := c.applyTarget(ExitPlan{Count: 128, ServerName: "fixture.example.invalid", Policy: defaultSlotPolicyForMode("AQ", false)}); err != nil {
		t.Fatal(err)
	}
	if len(c.m.Tunnels()) != 128 {
		t.Fatal("old capacity limit remains")
	}
	reloaded, err := newConsole(NewManager(maximumSlots, c.m.workDir))
	if err != nil || reloaded.plan.Count != 128 {
		t.Fatal("larger target did not survive restart", err)
	}
	c.probe = func(_ context.Context, n Node, _ int, _ func(string)) CheckResult {
		return CheckResult{Host: n.HostName, Status: "failed"}
	}
	j, err := c.startCheck(CheckRequest{All: true, Parallelism: 64})
	if err != nil {
		t.Fatal(err)
	}
	// This case durably writes 321 observations. Race instrumentation and
	// shared CI disks can exceed the small-job helper's 3-second deadline.
	select {
	case <-j.finished:
	case <-time.After(30 * time.Second):
		t.Fatal("full source check did not finish durable writes")
	}

	if j.Total != 321 || j.Done != 321 {
		t.Fatal("batch truncated")
	}
}

func TestConsoleCheckSettingsValidateAndRetainCorruption(t *testing.T) {
	c := consoleFixture(t)
	for _, body := range []string{`{"parallelism":0}`, `{"parallelism":-1}`, `{"parallelism":16385}`} {
		w := httptest.NewRecorder()
		c.apiCheckSettings(w, httptest.NewRequest("POST", "/", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatal("invalid count accepted")
		}
	}
	for _, n := range []int{1, 4, 64, 128} {
		w := httptest.NewRecorder()
		c.apiCheckSettings(w, httptest.NewRequest("POST", "/", strings.NewReader(fmt.Sprintf(`{"parallelism":%d}`, n))))
		if w.Code != 200 || c.checkParallelism != n {
			t.Fatal("user count rejected", n)
		}
	}
	p := filepath.Join(c.m.workDir, "check-settings.json")
	os.WriteFile(p, []byte(`{"parallelism":-5}`), 0600)
	if _, err := newConsole(NewManager(maximumSlots, c.m.workDir)); err == nil {
		t.Fatal("corrupt authoritative settings ignored")
	}
}

func TestDynamicDiagnosticCleanupPreservesUserNetwork(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires Linux root network namespaces")
	}
	if err := initMainNetns(); err != nil {
		t.Fatal(err)
	}
	oldTag, oldBase := instTag, instBase
	t.Cleanup(func() { instTag, instBase = oldTag, oldBase })
	dir := t.TempDir()
	if err := initInstance(dir); err != nil {
		t.Fatal(err)
	}
	node := Node{HostName: "test", IP: "192.0.2.1", Config: "proto tcp\nremote 192.0.2.1 1194\n"}
	user := &Tunnel{Slot: 256, Node: node}
	index := networkLinkCount - 2
	diagnostic := &Tunnel{Slot: -128, networkIndex: &index, Node: node}
	legacy := &Tunnel{Slot: 240, Node: node}
	for _, tn := range []*Tunnel{user, diagnostic, legacy} {
		tn := tn
		t.Cleanup(func() { tn.teardownNetns(); _ = os.RemoveAll(filepath.Join("/etc/netns", tn.nsName())) })
		if err := tn.setupNetns(); err != nil {
			t.Fatal(err)
		}
	}
	c, err := newConsole(NewManager(maximumSlots, dir))
	if err != nil {
		t.Fatal(err)
	}
	modernDir, _ := os.MkdirTemp(dir, ".connectivity-")
	blob, _ := json.Marshal(diagnosticResource{Slot: diagnostic.Slot, NetworkIndex: index, Node: node})
	if err := atomicConfigWrite(filepath.Join(modernDir, "diagnostic.json"), blob); err != nil {
		t.Fatal(err)
	}
	legacyDir, _ := os.MkdirTemp(dir, ".connectivity-")
	os.WriteFile(filepath.Join(legacyDir, legacy.nsName()+".ovpn"), []byte(node.Config), 0600)
	if err := c.cleanupDiagnostics(); err != nil {
		t.Fatal(err)
	}
	for _, tn := range []*Tunnel{diagnostic, legacy} {
		if _, err := os.Stat(filepath.Join("/run/netns", tn.nsName())); !os.IsNotExist(err) {
			t.Fatal("diagnostic namespace retained", err)
		}
	}
	if _, err := os.Stat(filepath.Join("/run/netns", user.nsName())); err != nil {
		t.Fatal("user namespace was removed", err)
	}
	dirs, _ := filepath.Glob(filepath.Join(dir, ".connectivity-*"))
	if len(dirs) != 0 {
		t.Fatal("temporary state retained")
	}
}

func TestDynamicDiagnosticCorruptManifestRetainsEvidence(t *testing.T) {
	c := consoleFixture(t)
	dir, _ := os.MkdirTemp(c.m.workDir, ".connectivity-")
	p := filepath.Join(dir, "diagnostic.json")
	os.WriteFile(p, []byte(`{"slot":1,"network_index":0}`), 0600)
	if c.cleanupDiagnostics() == nil {
		t.Fatal("positive user identity accepted for cleanup")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal("corrupt evidence removed")
	}
}
