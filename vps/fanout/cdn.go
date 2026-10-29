package main

import (
	"net"
	"os"
	"strconv"
	"strings"
)

// cdnDomain 返回配置的 Cloudflare / CDN 接入域名；未配置时返回空串。
// 优先读 FANOUT_CDN_DOMAIN，其次读 FANOUT_PUBLIC_HOST，再次读 FANOUT_PUBLIC_IP（若为域名格式）。
func cdnDomain() string {
	if d := strings.TrimSpace(os.Getenv("FANOUT_CDN_DOMAIN")); d != "" {
		return d
	}
	if d := strings.TrimSpace(os.Getenv("FANOUT_PUBLIC_HOST")); d != "" {
		return d
	}
	if d := strings.TrimSpace(os.Getenv("FANOUT_PUBLIC_IP")); d != "" && !isIPAddress(d) {
		return d
	}
	return ""
}

// cdnPort 返回外部客户端连接 CDN 的端口，默认 443。
func cdnPort() int {
	if p := strings.TrimSpace(os.Getenv("FANOUT_CDN_PORT")); p != "" {
		if val, err := strconv.Atoi(p); err == nil && val > 0 && val <= 65535 {
			return val
		}
	}
	return 443
}

// cdnPathPrefix 返回 CDN WebSocket 的伪装路径前缀，默认 "/ali-ws"（若域名包含 ali）或 "/vless-ws"。
func cdnPathPrefix() string {
	if p := strings.TrimSpace(os.Getenv("FANOUT_CDN_PATH")); p != "" {
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		return strings.TrimRight(p, "/")
	}
	d := cdnDomain()
	if strings.Contains(strings.ToLower(d), "ali") {
		return "/ali-ws"
	}
	return "/vless-ws"
}

// isIPAddress 检查一个字符串是否为 IPv4 或 IPv6 地址。
func isIPAddress(s string) bool {
	return net.ParseIP(s) != nil
}
