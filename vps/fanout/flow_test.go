package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCountryEvidenceAllowsCountryUnknownTypeButNeverResidential(t *testing.T) {
	code := "JP"
	image := unknownOpenVPNEgress("8.8.8.8", "intelligence_not_configured")
	image.Country = &code
	image.CountryEvidence = &CountryEvidence{IP: image.IP, Country: code, Status: "passed", Provider: "fixture-https", QueriedAt: time.Now().Format(time.RFC3339Nano), ExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339Nano), IdentityAssurance: "normal_https_certificate_validation"}
	blob, _ := json.Marshal(image)
	got := decodeOpenVPNEgress(blob, image.IP)
	if verifiedProfileCountry(got) != "JP" || got.IPType != "unknown" {
		t.Fatal("independent country missing or type promoted")
	}
	p := defaultSlotPolicyForMode("JP", false)
	if err := p.checkImage(got); err != nil {
		t.Fatal(err)
	}
	p.AllowedIPTypes = []string{"strict_residential"}
	if p.checkImage(got) == nil {
		t.Fatal("country-only proof became residential")
	}
	if name := subscriptionExitName(Exit{Slot: 2, Status: "up", Egress: &got}); !strings.Contains(name, "日本") {
		t.Fatal(name)
	}
	image.CountryEvidence.IP = "1.1.1.1"
	if verifiedProfileCountry(image) != "" {
		t.Fatal("wrong IP country accepted")
	}
	image.CountryEvidence.IP = image.IP
	image.CountryEvidence.ExpiresAt = "2000-01-01T00:00:00Z"
	if verifiedProfileCountry(image) != "" {
		t.Fatal("stale country accepted")
	}
}
func TestManualEntryBindsWaitingSlotAndManagedEntryCannotReappearAfterDelete(t *testing.T) {
	n, _, ts := rollbackFixture(t)
	ts[0].Status = "waiting"
	ts[0].Node = Node{}
	ib, err := n.CreateInbound(NewInboundSpec{Protocol: "vless", Network: "tcp", Port: 32145, BoundSlot: 1}, ts)
	if err != nil {
		t.Fatal(err)
	}
	manual := n.store.byID(ib.ID)
	if !manual.Manual || manual.BoundSlot != 1 {
		t.Fatal("create did not bind stable waiting slot")
	}
	cfg := buildXrayConfig(n.store.sorted(), ts)
	rules := cfg["routing"].(map[string]any)["rules"].([]any)
	if rules[len(rules)-1].(map[string]any)["outboundTag"] != "slot-01" {
		t.Fatal("stable route missing", rules)
	}
	if err = n.BindSlot(manual.tag(), 0, ts); err != nil {
		t.Fatal(err)
	}
	if manual = n.store.byID(ib.ID); manual.BoundSlot != 0 {
		t.Fatal("unbind failed")
	}
	if err = n.BindSlot(manual.tag(), 1, ts); err != nil {
		t.Fatal(err)
	}
	primary := n.store.Inbounds[0]
	primary.BoundSlot = 1
	primary.ManagedSlot = true
	if err = n.DeleteInbounds([]int{primary.ID}, ts); err == nil {
		t.Fatal("automatic entry silently deleted and recreated")
	}
	if err = n.DeleteInbounds([]int{ib.ID}, ts); err != nil {
		t.Fatal(err)
	}
	if n.store.byID(ib.ID) != nil {
		t.Fatal("manual entry not deleted")
	}
}
