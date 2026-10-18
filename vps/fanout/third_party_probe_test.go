package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestThirdPartyRealSocksHandshakeHTTPSAndNoFallback(t *testing.T) {
	var requests atomic.Int32
	web := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/content" {
			io.WriteString(w, "Example Domain")
			return
		}
		io.WriteString(w, "8.8.8.8\n")
	}))
	defer web.Close()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	cred := SocksCred{User: "user:@", Pass: "pass:@/ test"}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go serveSocks(c, &cred, func(network, addr string) (net.Conn, error) {
				if addr != web.Listener.Addr().String() {
					return nil, io.EOF
				}
				return net.DialTimeout(network, addr, time.Second)
			})
		}
	}()
	p := ThirdPartyProxy{Host: "127.0.0.1", Port: l.Addr().(*net.TCPAddr).Port, Username: cred.User, Password: cred.Pass}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	trusted := thirdPartyHTTPClient(p)
	defer trusted.CloseIdleConnections()
	trusted.Transport.(*http.Transport).TLSClientConfig = web.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	check := func(client *http.Client) ThirdPartyRecord {
		return probeThirdPartyRequests(ctx, p, client, []string{web.URL + "/a", web.URL + "/b"}, web.URL+"/content")
	}
	if r := check(trusted); r.Status != "passed" || r.ActualIP != "8.8.8.8" || r.LatencyMS <= 0 {
		t.Fatal("real SOCKS/HTTPS request failed", r.Status, r.Code)
	}
	before := requests.Load()
	p.Password = "wrong"
	wrong := thirdPartyHTTPClient(p)
	defer wrong.CloseIdleConnections()
	wrong.Transport.(*http.Transport).TLSClientConfig = web.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	if r := check(wrong); r.Status != "failed" || requests.Load() != before {
		t.Fatal("bad credentials reached target or fell back")
	}
	p.Password = cred.Pass
	untrusted := thirdPartyHTTPClient(p)
	defer untrusted.CloseIdleConnections()
	if r := check(untrusted); r.Status != "failed" {
		t.Fatal("untrusted TLS certificate accepted")
	}
	l.Close()
	if r := check(trusted); r.Status != "failed" || requests.Load() != before {
		t.Fatal("closed upstream fell back")
	}
}

func TestThirdPartyLiveSupplierProbe(t *testing.T) {
	if os.Getenv("XNV_THIRD_PARTY_LIVE") != "1" {
		t.Skip("explicit private supplier proxy acceptance only")
	}
	b, err := os.ReadFile(os.Getenv("XNV_THIRD_PARTY_FILE"))
	if err != nil {
		t.Fatal("private supplier file unavailable")
	}
	var p ThirdPartyProxy
	if json.Unmarshal(b, &p) != nil || validateThirdParty(p) != nil {
		t.Fatal("private supplier input invalid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	r := probeThirdParty(ctx, p, t.TempDir())
	if r.Status != "passed" {
		t.Fatal("supplier probe failed", r.Code)
	}
	wrong := p
	wrong.Password += "-invalid"
	r2 := probeThirdParty(ctx, wrong, t.TempDir())
	if r2.Status != "failed" {
		t.Fatal("supplier wrong password unexpectedly accepted")
	}
	safe := map[string]any{"status": r.Status, "actual_ip_observed": r.ActualIP != "", "https_latency_observed": r.LatencyMS > 0, "wrong_password_rejected": r2.Status == "failed", "protocol": "socks5"}
	if path := os.Getenv("XNV_THIRD_PARTY_REPORT"); path != "" {
		out, _ := json.Marshal(safe)
		if err := os.WriteFile(path, out, 0600); err != nil {
			t.Fatal("private test report write failed")
		}
	}
	t.Log("supplier SOCKS5, two HTTPS echoes, HTTPS content, and wrong-password rejection passed")
}
