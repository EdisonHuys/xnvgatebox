package main

import (
	"context"
	"fmt"
	"math"
	"net"
	"os/exec"
	"strconv"
	"strings"
)

var exitIPEchoURLs = []string{
	"https://api.ipify.org",
	"https://ipv4.icanhazip.com",
}

func probeIPPair(command func(string) ([]byte, error)) (string, error) {
	var got string
	for _, url := range exitIPEchoURLs {
		out, err := command(url)
		if err != nil {
			return "", err
		}
		ip := strings.TrimSpace(string(out))
		parsed := net.ParseIP(ip)
		if parsed == nil || parsed.To4() == nil {
			return "", fmt.Errorf("出口 IP 返回异常: %q", ip)
		}
		if got == "" {
			got = parsed.To4().String()
		} else if got != parsed.To4().String() {
			return "", fmt.Errorf("两个出口 IP 查询不一致: %s / %s", got, parsed)
		}
	}
	return got, nil
}

func (t *Tunnel) probeExpectedExitIP() (string, error) {
	return probeIPPair(func(url string) ([]byte, error) {
		return cmdOutput(exec.CommandContext(t.workContext(), "ip", "netns", "exec", t.nsName(), "curl", "--disable", "-4fsS", "--max-time", "15", url))
	})
}

func (t *Tunnel) probeSocksExitIP() (string, error) {
	ip, _, err := t.probeSocksExitIPWithLatency()
	return ip, err
}

func (t *Tunnel) probeSocksExitIPWithLatency() (string, float64, error) {
	return t.probeSocksWithContext(t.workContext())
}

func (t *Tunnel) probeSocksWithContext(ctx context.Context) (string, float64, error) {
	cred := t.probeCredential()
	proxy := fmt.Sprintf("127.0.0.1:%d", t.Port)
	userpass := cred.User + ":" + cred.Pass
	return probeSocksPair(func(url string) ([]byte, error) {
		return cmdOutput(exec.CommandContext(ctx, "curl", "--disable", "-4fsS", "--noproxy", "", "--max-time", "15", "--socks5-hostname", proxy, "--proxy-user", userpass, "--write-out", "\n%{time_total}", url))
	})
}

func probeSocksPair(command func(string) ([]byte, error)) (string, float64, error) {
	maximum := 0.0
	ip, err := probeIPPair(func(url string) ([]byte, error) {
		out, err := command(url)
		if err != nil {
			return nil, err
		}
		// curl appends the timing after the response body. Echo services may
		// already end that body with a newline (for example icanhazip), so
		// only the final separator belongs to --write-out.
		response := strings.TrimSpace(string(out))
		separator := strings.LastIndexByte(response, '\n')
		if separator < 0 {
			return nil, fmt.Errorf("SOCKS 出口和延迟响应异常")
		}
		seconds, err := strconv.ParseFloat(strings.TrimSpace(response[separator+1:]), 64)
		if err != nil || seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
			return nil, fmt.Errorf("SOCKS 延迟响应异常")
		}
		maximum = math.Max(maximum, seconds*1000)
		return []byte(strings.TrimSpace(response[:separator])), nil
	})
	return ip, maximum, err
}
