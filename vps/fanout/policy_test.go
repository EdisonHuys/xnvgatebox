package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func syntheticPolicy() SlotPolicy {
	return SlotPolicy{Country: "US", AllowedIPTypes: []string{"strict_residential", "likely_residential"},
		PreferredISP: []string{"Comcast"}, AllowedISP: []string{"Comcast", "Spectrum"}, BlockedISP: []string{"Hosting Co"},
		AllowedASN: []string{"AS7922", "AS20115"}, BlockedASN: []string{"AS64500"}, MaximumLatencyMS: 250}
}

func TestSlotPolicyHardConstraintsNeedFreshActualFields(t *testing.T) {
	p := syntheticPolicy()
	good := testEgressImage()
	if err := p.checkImage(good); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		change func(*EgressProfile)
	}{
		{"wrong-country", func(g *EgressProfile) { v := "JP"; g.Country = &v }},
		{"missing-country", func(g *EgressProfile) { g.Country = nil }},
		{"unknown-type", func(g *EgressProfile) { g.IPType = "unknown" }},
		{"missing-isp", func(g *EgressProfile) { g.ISP = nil }},
		{"not-allowed-isp", func(g *EgressProfile) { v := "Comcast Business Other"; g.ISP = &v }},
		{"blocked-isp", func(g *EgressProfile) { v := "Hosting Co"; g.ISP = &v }},
		{"missing-asn", func(g *EgressProfile) { g.ASN = nil }},
		{"not-allowed-asn", func(g *EgressProfile) { v := "AS123"; g.ASN = &v }},
		{"blocked-asn", func(g *EgressProfile) { v := "AS64500"; g.ASN = &v }},
		{"expired", func(g *EgressProfile) { g.ExpiresAt = "2000-01-01T00:00:00Z" }},
		{"sstp", func(g *EgressProfile) { g.Protocol = "sstp" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := good
			tc.change(&g)
			if p.checkImage(g) == nil {
				t.Fatal("hard constraint bypassed")
			}
		})
	}
	lower := "comcast"
	good.ISP = &lower
	if p.checkImage(good) != nil {
		t.Fatal("exact ISP comparison should ignore case")
	}
}

func TestSlotPolicyUnknownAndBlacklistsDoNotInventEvidence(t *testing.T) {
	p := SlotPolicy{AllowedIPTypes: []string{"unknown"}, PreferredISP: []string{"Favorite"}}
	unknown := unknownOpenVPNEgress("73.1.1.1", "intelligence_not_configured")
	if p.checkImage(unknown) != nil {
		t.Fatal("explicit unknown-only technical policy should work without provider")
	}
	for _, change := range []func(*SlotPolicy){
		func(p *SlotPolicy) { p.Country = "JP" },
		func(p *SlotPolicy) { p.BlockedISP = []string{"Hosting"} },
		func(p *SlotPolicy) { p.BlockedASN = []string{"AS64500"} },
	} {
		q := p
		change(&q)
		if q.checkImage(unknown) == nil {
			t.Fatal("missing fields claimed to satisfy a hard policy")
		}
	}
}

func TestSlotPolicyNormalizationRejectsContradictionsAndEmptyTypes(t *testing.T) {
	cases := []SlotPolicy{
		{Country: "Japan", AllowedIPTypes: []string{"unknown"}},
		{AllowedIPTypes: []string{}},
		{AllowedIPTypes: []string{"home"}},
		{AllowedIPTypes: []string{"unknown"}, AllowedISP: []string{"ISP"}, BlockedISP: []string{"isp"}},
		{AllowedIPTypes: []string{"unknown"}, AllowedASN: []string{"7922"}},
		{AllowedIPTypes: []string{"unknown"}, MaximumLatencyMS: -1},
	}
	for _, p := range cases {
		if _, err := normalizePolicy(p); err == nil {
			t.Fatalf("invalid policy accepted: %+v", p)
		}
	}
	p, err := normalizePolicy(SlotPolicy{Country: " jp ", AllowedIPTypes: []string{"residential", "STRICT_RESIDENTIAL", "unknown"}, AllowedASN: []string{"as7922"}})
	if err != nil || p.Country != "JP" || len(p.AllowedIPTypes) != 2 || p.AllowedIPTypes[0] != "strict_residential" || p.AllowedASN[0] != "AS7922" {
		t.Fatalf("normalization wrong: %+v %v", p, err)
	}
}

func TestSlotPolicyPreferenceRanksWithoutBecomingAllowList(t *testing.T) {
	m := NewManager(3, t.TempDir())
	a := testEgressImage()
	b := a
	isp := "Spectrum"
	b.ISP = &isp
	m.nodes = []Node{
		{HostName: "fast", CountryCode: "US", SpeedMbps: 900, OpenVPNStatus: "passed", ExpectedExitIP: a.IP, OpenVPNEgress: &b},
		{HostName: "preferred", CountryCode: "US", SpeedMbps: 50, OpenVPNStatus: "passed", ExpectedExitIP: a.IP, OpenVPNEgress: &a},
		{HostName: "unknown", CountryCode: "US", SpeedMbps: 1000, Ping: 1},
	}
	p := SlotPolicy{Country: "US", AllowedIPTypes: []string{"likely_residential"}, PreferredISP: []string{"Comcast"}}
	got := m.policyCandidates(p, nil, 0)
	if len(got) != 3 || got[0].HostName != "preferred" {
		t.Fatalf("preference excluded candidates or did not rank: %+v", got)
	}
	p.AllowedISP = []string{"Comcast"}
	got = m.policyCandidates(p, nil, 0)
	for _, n := range got {
		if n.HostName == "fast" {
			t.Fatal("known disallowed ISP admitted")
		}
	}
	// An unknown candidate is admitted to probing, not marked compliant/up.
	if len(got) != 2 {
		t.Fatalf("unknown should remain probeable: %+v", got)
	}
	p.Country = "JP"
	if got := m.policyCandidates(p, nil, 0); len(got) != 0 {
		t.Fatal("silently crossed country")
	}
}

func TestSlotPolicyUsesSlowerSocksHTTPSMeasurementNotSourcePing(t *testing.T) {
	calls := 0
	ip, latency, err := probeSocksPair(func(url string) ([]byte, error) {
		calls++
		if calls == 1 {
			return []byte("73.1.1.1\n0.050"), nil
		}
		return []byte("73.1.1.1\n0.350"), nil
	})
	if err != nil || ip != "73.1.1.1" || latency != 350 || calls != 2 {
		t.Fatalf("measurement wrong: %s %v %v", ip, latency, err)
	}
	p := syntheticPolicy()
	if p.checkLatency(latency) == nil {
		t.Fatal("slow second HTTPS request ignored")
	}
	for _, v := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if p.checkLatency(v) == nil {
			t.Fatal("invalid local measurement accepted")
		}
	}
	for _, response := range []string{"73.1.1.1\nNaN", "73.1.1.1\n0", "73.1.1.1", "73.1.1.1\nInfinity"} {
		if _, _, err := probeSocksPair(func(string) ([]byte, error) { return []byte(response), nil }); err == nil {
			t.Fatal("malformed measurement accepted")
		}
	}
	// Source ping is deliberately irrelevant to the final threshold.
	m := NewManager(3, t.TempDir())
	m.nodes = []Node{{HostName: "source-fast", CountryCode: "US", Ping: 1}}
	if len(m.policyCandidates(p, nil, 0)) != 1 || p.checkLatency(500) == nil {
		t.Fatal("source ping used as measured SOCKS latency")
	}
}

func TestSocksProbeAcceptsEchoTrailingNewline(t *testing.T) {
	for _, ending := range []string{"\n", "\r\n"} {
		t.Run(strconv.Quote(ending), func(t *testing.T) {
			calls := 0
			ip, latency, err := probeSocksPair(func(string) ([]byte, error) {
				calls++
				if calls == 1 {
					return []byte("73.1.1.1\n0.050"), nil
				}
				return []byte("73.1.1.1" + ending + "\n0.350"), nil
			})
			if err != nil || ip != "73.1.1.1" || latency != 350 || calls != 2 {
				t.Fatal("valid newline-terminated echo rejected or timing lost")
			}
		})
	}
	for _, body := range []string{"73.1.1.1\n8.8.8.8", "73.1.1.1\nunexpected", ""} {
		if _, _, err := probeSocksPair(func(string) ([]byte, error) { return []byte(body + "\n0.100"), nil }); err == nil {
			t.Fatal("malformed echo body accepted")
		}
	}
}

func TestCandidateBudgetDoesNotStarveUntriedNodesAfterCooldown(t *testing.T) {
	m := NewManager(3, t.TempDir())
	p := SlotPolicy{Country: "JP", AllowedIPTypes: []string{"unknown"}}
	failed := map[string]time.Time{"current": time.Now().Add(-time.Second)}
	for i := 0; i < 10; i++ {
		host := "candidate-" + strconv.Itoa(i)
		m.nodes = append(m.nodes, Node{HostName: host, CountryCode: "JP", SpeedMbps: float64(100 - i)})
		if i < 5 {
			failed[host] = time.Now().Add(-time.Second)
		}
	}
	tunnel := &Tunnel{Slot: 1, Status: "waiting_for_nodes", Policy: &p,
		Node: Node{HostName: "current", CountryCode: "JP"}, cooldown: failed}
	m.tunnels[1] = tunnel
	got := m.candidatesFor(tunnel)
	if len(got) != 6 || got[0].HostName != "current" {
		t.Fatal("current candidate preference or round budget changed")
	}
	for i := 1; i < len(got); i++ {
		if got[i].HostName != "candidate-"+strconv.Itoa(i+4) {
			t.Fatal("expired high-ranked failures starved untried candidates")
		}
	}
}

func TestSlotPolicyPersistsStoppedIdentityAndLeavesOtherSlotsAlone(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(3, dir)
	p := syntheticPolicy()
	m.tunnels[1] = &Tunnel{Slot: 1, Port: 17928, Node: Node{HostName: "a", CountryCode: "US", Config: "a-profile"}, Status: "stopped", Policy: &p, Cred: SocksCred{User: "u", Pass: "p"}}
	other := SlotPolicy{Country: "KR", AllowedIPTypes: []string{"unknown"}}
	m.tunnels[2] = &Tunnel{Slot: 2, Port: 17929, Node: Node{HostName: "b", CountryCode: "KR", Config: "b-profile"}, Status: "stopped", Policy: &other, Cred: SocksCred{User: "u2", Pass: "p2"}}
	q := p
	q.Country = "JP"
	q.MaximumLatencyMS = 700
	if err := m.SetSlotPolicy(1, q); err != nil {
		t.Fatal(err)
	}
	if m.tunnels[2].policySnapshot().Country != "KR" || m.tunnels[1].Port != 17928 || m.tunnels[1].nsName() != (&Tunnel{Slot: 1}).nsName() {
		t.Fatal("other slot or identity changed")
	}
	restored := NewManager(3, dir)
	if n, err := restored.restoreState(); err != nil || n != 2 {
		t.Fatalf("restore failed: %v %d", err, n)
	}
	got := restored.tunnels[1]
	if got.Status != "stopped" || got.Port != 17928 || got.policySnapshot().Country != "JP" || got.policySnapshot().MaximumLatencyMS != 700 || got.egressSnapshot(false) != nil {
		t.Fatal("policy/port changed or disk evidence treated as live")
	}
	if len(got.policySnapshot().AllowedISP) != 2 || len(got.policySnapshot().BlockedASN) != 1 {
		t.Fatal("policy lists lost on disk")
	}
	stat, _ := os.Stat(statePath(dir))
	if stat.Mode().Perm() != 0600 {
		t.Fatal("state should remain private")
	}
}

func TestSlotPolicyValidationFailureDoesNotMutateRunningIntent(t *testing.T) {
	m := NewManager(3, t.TempDir())
	p := syntheticPolicy()
	tunnel := &Tunnel{Slot: 1, Status: "up", Policy: &p}
	tunnel.resetWork()
	m.tunnels[1] = tunnel
	bad := p
	bad.AllowedIPTypes = nil
	if m.SetSlotPolicy(1, bad) == nil || tunnel.Status != "up" || tunnel.workContext().Err() != nil || tunnel.policySnapshot().Country != "US" {
		t.Fatal("invalid edit mutated running state")
	}
}

func TestSlotPolicySaveFailureBlocksAndKeepsPriorPolicy(t *testing.T) {
	m := NewManager(3, filepath.Join(t.TempDir(), "missing"))
	p := syntheticPolicy()
	tunnel := &Tunnel{Slot: 1, Status: "up", Policy: &p}
	tunnel.resetWork()
	m.tunnels[1] = tunnel
	q := p
	q.Country = "JP"
	if m.SetSlotPolicy(1, q) == nil {
		t.Fatal("save failure swallowed")
	}
	if tunnel.Status != "blocked" || tunnel.acceptsSocks() || tunnel.policySnapshot().Country != "US" {
		t.Fatal("save failure reopened traffic or lost old policy")
	}
}

func TestSlotPolicyPlannerKeepsWaitingIntentWithoutRelaxation(t *testing.T) {
	m := NewManager(3, t.TempDir())
	p := syntheticPolicy()
	p.Country = "JP"
	m.nodes = []Node{{HostName: "kr", CountryCode: "KR"}}
	picks, saved, err := m.planProvision(ProvisionRequest{Region: "JP", Count: 2, Policy: &p})
	if err != nil || len(picks) != 2 || picks[0].HostName != "" || saved.Country != "JP" || saved.AllowedIPTypes[0] != "strict_residential" {
		t.Fatal("no candidates should retain constrained intent, not broaden")
	}
	tunnel := &Tunnel{Slot: 1, Policy: &saved, Status: "waiting_for_nodes", Node: picks[0]}
	m.tunnels[1] = tunnel
	if len(m.candidatesFor(tunnel)) != 0 || tunnel.acceptsSocks() {
		t.Fatal("waiting slot admitted foreign candidate/traffic")
	}
	if _, _, err := m.planProvision(ProvisionRequest{Region: "KR", Count: 1, Policy: &p}); err == nil {
		t.Fatal("conflicting country accepted")
	}
}

func TestSlotPolicyManualNodeCannotBypassKnownHardRestriction(t *testing.T) {
	m := NewManager(3, t.TempDir())
	p := syntheticPolicy()
	image := testEgressImage()
	isp := "Disallowed"
	image.ISP = &isp
	m.nodes = []Node{{HostName: "bad", CountryCode: "US", OpenVPNStatus: "passed", ExpectedExitIP: image.IP, OpenVPNEgress: &image}}
	tunnel := &Tunnel{Slot: 1, Port: 17928, Status: "up", Policy: &p, Node: Node{HostName: "old", CountryCode: "US"}}
	m.tunnels[1] = tunnel
	if m.SwapTo(1, "bad") == nil || tunnel.Node.HostName != "old" || tunnel.Port != 17928 || tunnel.Status != "up" {
		t.Fatal("manual override bypassed policy")
	}
}

func TestSlotPolicyCancellationClosesClientsAndRejectsStaleWork(t *testing.T) {
	tunnel := &Tunnel{Slot: 1, Status: "up"}
	tunnel.resetWork()
	old := tunnel.workContext()
	a, b := net.Pipe()
	defer b.Close()
	if !tunnel.trackClient(a) {
		t.Fatal("cannot track live test client")
	}
	tunnel.blockWork()
	if old.Err() == nil || tunnel.acceptsSocks() {
		t.Fatal("cancel did not gate business")
	}
	var one [1]byte
	if _, err := b.Read(one[:]); err != io.EOF {
		t.Fatalf("old client not closed: %v", err)
	}
	tunnel.resetWork()
	tunnel.Status = "waiting_for_nodes"
	m := NewManager(3, t.TempDir())
	m.tunnels[1] = tunnel
	m.bringUpPersistContext(tunnel, false, true, old)
	if tunnel.Status != "waiting_for_nodes" {
		t.Fatal("canceled operation published under a new context")
	}
}

func TestSlotPolicyStopCleanupDoesNotHoldImageMutex(t *testing.T) {
	tunnel := &Tunnel{Slot: 1, Status: "up"}
	tunnel.resetWork()
	done := make(chan struct{})
	go func() { tunnel.stopWithCleanup(func() { tunnel.clearEgress() }); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop deadlocked during image cleanup")
	}
	if tunnel.Status != "stopped" || tunnel.acceptsSocks() {
		t.Fatal("stop did not block")
	}
}

func TestSlotPolicyProbeCredentialsAreSeparateFromBusiness(t *testing.T) {
	tunnel := &Tunnel{Status: "starting", Cred: SocksCred{User: "business", Pass: "business-password"}}
	tunnel.resetWork()
	tunnel.setValidating(true)
	probe := tunnel.probeCredential()
	if probe.User == tunnel.credential().User || probe.Pass == tunnel.credential().Pass || !tunnel.acceptsSocks() {
		t.Fatal("manager validation shares business credentials")
	}
	tunnel.setValidating(false)
	if tunnel.acceptsSocks() || tunnel.probeCredential().User != "business" {
		t.Fatal("validation window remained open")
	}
}

func TestSlotPolicyAPIAuthStrictJSONAndPersistence(t *testing.T) {
	m := NewManager(3, t.TempDir())
	p := syntheticPolicy()
	m.tunnels[1] = &Tunnel{Slot: 1, Port: 17928, Status: "stopped", Policy: &p}
	auth, _, err := NewAuth(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	auth.Wrap(apiSlotPolicy(m)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/slot/policy?slot=1", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatal("policy leaked without login")
	}
	for _, body := range []string{`{"allowed_ip_types":["unknown"],"typo":1}`, `{"allowed_ip_types":["unknown"]} {}`} {
		rec = httptest.NewRecorder()
		apiSlotPolicy(m)(rec, httptest.NewRequest(http.MethodPost, "/api/slot/policy?slot=1", strings.NewReader(body)))
		if rec.Code != 400 {
			t.Fatal("unknown/trailing JSON fields accepted")
		}
	}
	body, _ := json.Marshal(p)
	rec = httptest.NewRecorder()
	apiSlotPolicy(m)(rec, httptest.NewRequest(http.MethodPost, "/api/slot/policy?slot=1", strings.NewReader(string(body))))
	if rec.Code != 200 {
		t.Fatalf("policy save failed: %s", rec.Body.String())
	}
	rec = httptest.NewRecorder()
	apiSlotLifecycle(m, false)(rec, httptest.NewRequest(http.MethodGet, "/api/slot/resume?slot=1", nil))
	if rec.Code != 405 {
		t.Fatal("GET mutated lifecycle")
	}
}

func TestSlotPolicyCandidateCooldownAndProfileRefresh(t *testing.T) {
	m := NewManager(3, t.TempDir())
	p := SlotPolicy{Country: "US", AllowedIPTypes: []string{"likely_residential"}}
	m.nodes = []Node{{HostName: "old", CountryCode: "US", Config: "new-profile"}, {HostName: "next", CountryCode: "US"}}
	image := testEgressImage()
	bad := image
	isp := "Wrong"
	bad.ISP = &isp
	m.localEvidence["old"] = Node{HostName: "old", CountryCode: "US", Config: "obsolete-profile", OpenVPNStatus: "passed", ExpectedExitIP: image.IP, OpenVPNEgress: &bad}
	p.AllowedISP = []string{"Comcast"}
	// Local evidence for a different profile must not filter the new profile.
	if len(m.policyCandidates(p, nil, 0)) != 2 {
		t.Fatal("obsolete-profile evidence inherited")
	}
	tunnel := &Tunnel{Slot: 1, Status: "waiting_for_nodes", Policy: &p, Node: m.nodes[0], cooldown: map[string]time.Time{"old": time.Now().Add(time.Minute)}}
	m.tunnels[1] = tunnel
	got := m.candidatesFor(tunnel)
	if len(got) != 1 || got[0].HostName != "next" {
		t.Fatal("cooldown ignored or policy broadened")
	}
	if _, _, err := probeSocksPair(func(string) ([]byte, error) { return nil, errors.New("probe failed") }); err == nil {
		t.Fatal("probe error swallowed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if waitForWork(ctx, time.Hour) {
		t.Fatal("canceled backoff did not exit")
	}
}

func TestSlotPolicyAtomicCandidateClaimAndPublication(t *testing.T) {
	m := NewManager(3, t.TempDir())
	a := &Tunnel{Slot: 1, Status: "starting"}
	a.resetWork()
	m.tunnels[1] = a
	b := &Tunnel{Slot: 2, Status: "starting"}
	b.resetWork()
	m.tunnels[2] = b
	n := Node{HostName: "same", CountryCode: "US"}
	if !m.claimNode(a, n, a.workContext()) || m.claimNode(b, n, b.workContext()) {
		t.Fatal("two Slots claimed same candidate")
	}
	image := testEgressImage()
	a.ExitIP = image.IP
	if err := a.acceptExitEvidence(image.IP, image.IP, image); err != nil {
		t.Fatal(err)
	}
	if !m.publishUp(a, a.workContext()) {
		t.Fatal("valid candidate not published")
	}
	b.Node = Node{HostName: "different"}
	b.ExitIP = image.IP
	if err := b.acceptExitEvidence(image.IP, image.IP, image); err != nil {
		t.Fatal(err)
	}
	if m.publishUp(b, b.workContext()) {
		t.Fatal("two independent Slots published same exit IP")
	}
	old := b.workContext()
	b.blockWork()
	b.resetWork()
	if m.publishUp(b, old) {
		t.Fatal("late canceled operation published under new context")
	}
}

func TestSlotPolicyConcurrentEditsAndSnapshots(t *testing.T) {
	m := NewManager(3, t.TempDir())
	p := syntheticPolicy()
	m.tunnels[1] = &Tunnel{Slot: 1, Port: 17928, Status: "stopped", Policy: &p}
	m.tunnels[2] = &Tunnel{Slot: 2, Port: 17929, Status: "stopped", Policy: &p}
	done := make(chan error, 2)
	for slot := 1; slot <= 2; slot++ {
		go func(slot int) {
			for i := 0; i < 10; i++ {
				q := m.tunnels[slot].policySnapshot()
				q.MaximumLatencyMS = 300 + i
				if err := m.SetSlotPolicy(slot, q); err != nil {
					done <- err
					return
				}
			}
			done <- nil
		}(slot)
	}
	for i := 0; i < 20; i++ {
		for _, tunnel := range m.Tunnels() {
			_ = tunnel.policySnapshot()
			_ = tunnel.egressSnapshot(false)
			_, _ = json.Marshal(tunnel)
		}
	}
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	var state persistedState
	blob, err := os.ReadFile(statePath(m.workDir))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(blob, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Tunnels) != 2 || state.Tunnels[0].Policy.MaximumLatencyMS != 309 || state.Tunnels[1].Policy.MaximumLatencyMS != 309 {
		t.Fatal("concurrent saves lost Slot intent")
	}
}

func TestSlotPolicyRuntimeExpiryBlocksAndExportDeletionIsExplicit(t *testing.T) {
	p := syntheticPolicy()
	image := testEgressImage()
	tunnel := &Tunnel{Slot: 1, Status: "up", ExitIP: image.IP, Policy: &p, LatencyMS: 100}
	if err := tunnel.acceptExitEvidence(image.IP, image.IP, image); err != nil {
		t.Fatal(err)
	}
	if !tunnel.acceptsSocks() {
		t.Fatal("valid live evidence unexpectedly blocked")
	}
	tunnel.mu.Lock()
	tunnel.egress.ExpiresAt = "2000-01-01T00:00:00Z"
	tunnel.mu.Unlock()
	if tunnel.acceptsSocks() {
		t.Fatal("expired evidence admitted new business")
	}
	m := NewManager(3, t.TempDir())
	m.deletedSlots[1] = true
	rec := httptest.NewRecorder()
	apiExportXray(m)(rec, httptest.NewRequest(http.MethodGet, "/api/export/xray", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `"slot-01"`) || strings.Contains(body, `"protocol":"socks"`) {
		t.Fatal("deleted Slot lost explicit block mapping")
	}
	if err := m.saveState(); err != nil {
		t.Fatal(err)
	}
	restored := NewManager(3, m.workDir)
	if _, err := restored.restoreState(); err != nil {
		t.Fatal(err)
	}
	if !restored.deletedSlots[1] {
		t.Fatal("deletion block tombstone lost on restart")
	}
}

func TestSlotPolicyLegacyStoppedStateMigratesLoadedDefaults(t *testing.T) {
	withResidentialOnly(t, boolPtr(false))
	dir := t.TempDir()
	old := `{"tunnels":[{"slot":2,"port":17929,"hostname":"old","country_code":"JP","config":"profile","socks_user":"u","socks_pass":"p","stopped":true}]}`
	if err := os.WriteFile(statePath(dir), []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	m := NewManager(3, dir)
	if _, err := m.restoreState(); err != nil {
		t.Fatal(err)
	}
	tunnel := m.tunnels[2]
	p := tunnel.policySnapshot()
	if p.Country != "JP" || !containsFold(p.AllowedIPTypes, "unknown") || tunnel.Port != 17929 || tunnel.Status != "stopped" || tunnel.egressSnapshot(false) != nil {
		t.Fatal("migration ignored loaded defaults or inherited health")
	}
}

func TestSlotPolicyBadRestoreDoesNotStartDefaultOrPartialSlots(t *testing.T) {
	for _, body := range []string{
		`{"tunnels":[{"slot":1,"port":17928,"stopped":true,"policy":{"allowed_ip_types":[]}}]}`,
		`{"tunnels":[{"slot":1,"port":17928,"stopped":true},{"slot":2,"port":17928,"stopped":true}]}`,
	} {
		dir := t.TempDir()
		if err := os.WriteFile(statePath(dir), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		m := NewManager(3, dir)
		if _, err := m.restoreState(); err == nil || len(m.tunnels) != 0 {
			t.Fatal("invalid restore broadened intent or launched partial Slots")
		}
	}
}
