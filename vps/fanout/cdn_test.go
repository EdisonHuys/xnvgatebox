package main

import (
	"strings"
	"testing"
)

func TestCDNDomainAndDefaults(t *testing.T) {
	t.Setenv("FANOUT_CDN_DOMAIN", "")
	t.Setenv("FANOUT_PUBLIC_HOST", "")
	t.Setenv("FANOUT_PUBLIC_IP", "1.2.3.4")
	if d := cdnDomain(); d != "" {
		t.Fatalf("expected empty cdn domain, got %q", d)
	}

	t.Setenv("FANOUT_CDN_DOMAIN", "ali.edisonhu.xyz")
	if d := cdnDomain(); d != "ali.edisonhu.xyz" {
		t.Fatalf("expected ali.edisonhu.xyz, got %q", d)
	}
	if p := cdnPort(); p != 443 {
		t.Fatalf("expected port 443, got %d", p)
	}
	if p := cdnPathPrefix(); p != "/ali-ws" {
		t.Fatalf("expected /ali-ws, got %q", p)
	}
}

func TestCDNShareLink(t *testing.T) {
	t.Setenv("FANOUT_CDN_DOMAIN", "ali.edisonhu.xyz")

	ib := &nativeInbound{
		ID:       1,
		Port:     30001,
		Protocol: "vless",
		Network:  "ws",
		Security: "tls",
		Path:     "/ali-ws/old",
		Host:     "old.example.com",
		Remark:   "🇯🇵 日本 · 01 · VPN Gate",
		TLS: &tlsConfig{
			ServerName: "localhost",
			SelfSigned: true,
			CertSha256: "some_sha",
		},
	}
	client := nativeClient{
		Email: "vless-30001",
		ID:    "11111111-2222-3333-4444-555555555555",
	}

	link := shareLink(ib, client, "43.108.86.141")

	if !strings.HasPrefix(link, "vless://11111111-2222-3333-4444-555555555555@ali.edisonhu.xyz:443") {
		t.Fatalf("unexpected link prefix: %s", link)
	}
	if !strings.Contains(link, "sni=ali.edisonhu.xyz") {
		t.Fatalf("expected sni=ali.edisonhu.xyz, got: %s", link)
	}
	if !strings.Contains(link, "host=ali.edisonhu.xyz") {
		t.Fatalf("expected host=ali.edisonhu.xyz, got: %s", link)
	}
	if !strings.Contains(link, "path=%2Fali-ws") {
		t.Fatalf("expected path=/ali-ws, got: %s", link)
	}
	if strings.Contains(link, "pinSHA256") {
		t.Fatalf("expected no pinSHA256 for CDN, got: %s", link)
	}
}

func TestCDNBuildXrayConfig(t *testing.T) {
	t.Setenv("FANOUT_CDN_DOMAIN", "ali.edisonhu.xyz")

	inbounds := []*nativeInbound{
		{
			ID:         1,
			Port:       443,
			Protocol:   "vless",
			Network:    "ws",
			Security:   "tls",
			HostDirect: true,
			Enable:     true,
			Clients: []nativeClient{
				{Email: "vless-direct", ID: "aaaa-1111", Enable: true},
			},
			TLS: &tlsConfig{CertFile: "/tmp/cert.crt", KeyFile: "/tmp/cert.key"},
		},
		{
			ID:          2,
			Port:        30001,
			Protocol:    "vless",
			Network:     "ws",
			Security:    "tls",
			BoundSlot:   1,
			ManagedSlot: true,
			Enable:      true,
			Clients: []nativeClient{
				{Email: "vless-30001", ID: "bbbb-2222", Enable: true},
			},
			TLS: &tlsConfig{CertFile: "/tmp/cert.crt", KeyFile: "/tmp/cert.key"},
		},
	}

	tunnels := []*Tunnel{
		{Slot: 1, Port: 29001, Status: "up"},
	}

	cfg := buildXrayConfig(inbounds, tunnels)

	// Verify inbounds
	ins, ok := cfg["inbounds"].([]any)
	if !ok || len(ins) != 2 {
		t.Fatalf("expected 2 inbounds (CDN 443 + 30001), got %v", len(ins))
	}

	cdnIn := ins[0].(map[string]any)
	if cdnIn["port"] != 443 || cdnIn["tag"] != "in-443-ws" {
		t.Fatalf("unexpected CDN inbound: %v", cdnIn)
	}
	settings := cdnIn["settings"].(map[string]any)
	clients := settings["clients"].([]any)
	if len(clients) != 2 {
		t.Fatalf("expected 2 clients in CDN inbound, got %d", len(clients))
	}

	// Verify routing rules
	routing := cfg["routing"].(map[string]any)
	rules := routing["rules"].([]any)

	// There should be user-based rules for direct and slot-01
	foundDirect := false
	foundSlot1 := false
	for _, r := range rules {
		rule := r.(map[string]any)
		users, hasUsers := rule["user"].([]any)
		if hasUsers && len(users) > 0 {
			if users[0] == "vless-direct" && rule["outboundTag"] == "host-direct" {
				foundDirect = true
			}
			if users[0] == "vless-30001" && rule["outboundTag"] == "slot-01" {
				foundSlot1 = true
			}
		}
	}

	if !foundDirect {
		t.Fatal("missing routing rule for vless-direct")
	}
	if !foundSlot1 {
		t.Fatal("missing routing rule for vless-30001")
	}
}
