package main

import (
	"context"
	"testing"
	"time"
)

func TestConnectBudgetSharedPriorityResizeAndCancellation(t *testing.T) {
	b := newConnectBudget(1)
	hold, err := b.acquire(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer hold()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	order := make(chan string, 2)
	release := make(chan struct{})
	go func() {
		f, e := b.acquire(ctx, false)
		if e == nil {
			order <- "diagnostic"
			<-release
			f()
		}
	}()
	go func() {
		f, e := b.acquire(ctx, true)
		if e == nil {
			order <- "production"
			<-release
			f()
		}
	}()
	deadline := time.Now().Add(time.Second)
	for {
		b.mu.Lock()
		waiting := b.productionWaiting
		b.mu.Unlock()
		if waiting == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("production did not queue")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-order:
		t.Fatal("shared limit bypassed")
	default:
	}
	hold()
	select {
	case who := <-order:
		if who != "production" {
			t.Fatal("recovery did not receive priority", who)
		}
	case <-ctx.Done():
		t.Fatal("waiter stuck")
	}
	b.setLimit(2)
	select {
	case who := <-order:
		if who != "diagnostic" {
			t.Fatal(who)
		}
	case <-ctx.Done():
		t.Fatal("resizing failed to wake waiter")
	}
	b.setLimit(1)
	stopped, stop := context.WithCancel(ctx)
	stop()
	if _, e := b.acquire(stopped, true); e == nil {
		t.Fatal("cancelled task got a permit")
	}
	close(release)
}

func TestConnectObservationsPrioritizeFreshPassedWithoutTrustingAsLive(t *testing.T) {
	m := NewManager(4, t.TempDir())
	m.nodes = []Node{{HostName: "failed", Config: "a", SpeedMbps: 1000}, {HostName: "untested", Config: "b", SpeedMbps: 100}, {HostName: "passed", Config: "c", SpeedMbps: 1}}
	m.recordConnectivity(CheckResult{Host: "failed", ProfileSHA: profileDigest("a"), Status: "failed", CheckedAt: time.Now()})
	m.recordConnectivity(CheckResult{Host: "passed", ProfileSHA: profileDigest("c"), Status: "passed", ActualIP: "8.8.8.8", CheckedAt: time.Now()})
	picks := m.policyCandidates(defaultSlotPolicyForMode("", false), nil, 0)
	if len(picks) != 3 || picks[0].HostName != "passed" || picks[2].HostName != "failed" {
		t.Fatal("observations not used", picks)
	}
	if len(m.Tunnels()) != 0 {
		t.Fatal("observation became a running exit")
	}
	m.nodes[2].Config = "changed"
	picks = m.policyCandidates(defaultSlotPolicyForMode("", false), nil, 0)
	if picks[0].HostName != "untested" {
		t.Fatal("changed profile reused old result")
	}
	m.nodes[2].Config = "c"
	m.mu.Lock()
	r := m.observations["passed"]
	r.CheckedAt = time.Now().Add(-16 * time.Minute)
	m.observations["passed"] = r
	m.mu.Unlock()
	picks = m.policyCandidates(defaultSlotPolicyForMode("", false), nil, 0)
	if picks[0].HostName != "untested" {
		t.Fatal("expired result stayed preferred")
	}
}
