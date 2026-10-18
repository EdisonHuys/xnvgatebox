package main

// The veth is only a control-plane path to the OpenVPN server.  Once tun0 is
// gone, packets must be rejected at both sides of that path; otherwise the
// namespace's ordinary default route plus host MASQUERADE would silently turn
// the SOCKS port into a VPS-direct proxy.

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

type vpnRemote struct {
	ip    string
	port  string
	proto string
}

func parseVPNRemotes(config string, fallbackIP string) []vpnRemote {
	proto := "tcp"
	var out []vpnRemote
	for _, raw := range strings.Split(config, "\n") {
		fields := strings.Fields(strings.TrimSpace(raw))
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") || strings.HasPrefix(fields[0], ";") {
			continue
		}
		switch fields[0] {
		case "proto":
			if len(fields) > 1 {
				proto = strings.TrimSuffix(strings.ToLower(fields[1]), "4")
			}
		case "remote":
			if len(fields) < 2 {
				continue
			}
			host := strings.TrimSpace(fields[1])
			port := "443"
			if len(fields) > 2 {
				port = fields[2]
			}
			p := proto
			for _, f := range fields[3:] {
				if f == "tcp" || f == "udp" || f == "tcp4" || f == "udp4" {
					p = strings.TrimSuffix(f, "4")
				}
			}
			for _, ip := range resolveIPv4(host) {
				out = append(out, vpnRemote{ip: ip, port: port, proto: p})
			}
		}
	}
	if len(out) == 0 && net.ParseIP(strings.TrimSpace(fallbackIP)) != nil {
		out = append(out, vpnRemote{ip: strings.TrimSpace(fallbackIP), port: "443", proto: proto})
	}
	return dedupeRemotes(out)
}

func resolveIPv4(host string) []string {
	if ip := net.ParseIP(host); ip != nil && ip.To4() != nil {
		return []string{ip.To4().String()}
	}
	// Resolve in the host namespace.  Do not let a goroutine that is currently
	// probing a tunnel inherit the target namespace's resolver.
	out, err := cmdOutput(exec.Command("getent", "ahostsv4", host))
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var ips []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		ip := net.ParseIP(fields[0])
		if ip != nil && ip.To4() != nil && !seen[ip.String()] {
			seen[ip.String()] = true
			ips = append(ips, ip.To4().String())
		}
	}
	return ips
}

func dedupeRemotes(in []vpnRemote) []vpnRemote {
	seen := map[string]bool{}
	out := make([]vpnRemote, 0, len(in))
	for _, r := range in {
		if net.ParseIP(r.ip) == nil {
			continue
		}
		key := r.ip + ":" + r.port + "/" + r.proto
		if !seen[key] {
			seen[key] = true
			out = append(out, r)
		}
	}
	return out
}

func installKillSwitch(t *Tunnel) error {
	veth, peer := t.vethNames()
	remotes := parseVPNRemotes(t.Node.Config, t.Node.IP)
	if len(remotes) == 0 {
		return fmt.Errorf("OpenVPN 配置没有可验证的 remote endpoint")
	}
	cidr := t.linkCIDR()
	// Put the reject in place first; endpoint allows are inserted at chain
	// position 1 and therefore end up before it.
	ensureRuleInsert("filter", "FORWARD", "-i", veth, "-s", cidr, "-j", "REJECT", "--reject-with", "icmp-admin-prohibited")
	for _, r := range remotes {
		args := []string{"-i", veth, "-s", cidr, "-d", r.ip, "-p", r.proto, "--dport", r.port, "-j", "ACCEPT"}
		ensureRuleInsert("filter", "FORWARD", args...)
		if err := run("ip", "netns", "exec", t.nsName(), "iptables", "-w", "5", "-I", "OUTPUT", "1", "-o", peer, "-d", r.ip, "-p", r.proto, "--dport", r.port, "-j", "ACCEPT"); err != nil {
			return fmt.Errorf("namespace control-plane allow 失败: %w", err)
		}
	}
	if err := run("ip", "netns", "exec", t.nsName(), "iptables", "-w", "5", "-A", "OUTPUT", "-o", peer, "-j", "REJECT", "--reject-with", "icmp-admin-prohibited"); err != nil {
		return fmt.Errorf("namespace kill switch 失败: %w", err)
	}
	// Return packets for the explicitly permitted OpenVPN control connection.
	ensureRuleInsert("filter", "FORWARD", "-o", veth, "-d", cidr, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT")
	if err := run("ip", "netns", "exec", t.nsName(), "sysctl", "-q", "-w", "net.ipv6.conf.all.disable_ipv6=1"); err != nil {
		return fmt.Errorf("禁用 namespace IPv6 失败: %w", err)
	}
	return nil
}

func removeKillSwitch(t *Tunnel) {
	veth := ""
	peer := ""
	veth, peer = t.vethNames()
	cidr := t.linkCIDR()
	remotes := parseVPNRemotes(t.Node.Config, t.Node.IP)
	for _, r := range remotes {
		removeRule("filter", "FORWARD", "-i", veth, "-s", cidr, "-d", r.ip, "-p", r.proto, "--dport", r.port, "-j", "ACCEPT")
		_ = run("ip", "netns", "exec", t.nsName(), "iptables", "-w", "5", "-D", "OUTPUT", "-o", peer, "-d", r.ip, "-p", r.proto, "--dport", r.port, "-j", "ACCEPT")
	}
	removeRule("filter", "FORWARD", "-i", veth, "-s", cidr, "-j", "REJECT", "--reject-with", "icmp-admin-prohibited")
	removeRule("filter", "FORWARD", "-o", veth, "-d", cidr, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT")
}

func removeRule(table, chain string, spec ...string) {
	args := append([]string{"-w", "5", "-t", table, "-D", chain}, spec...)
	runQuiet("iptables", args...)
}
