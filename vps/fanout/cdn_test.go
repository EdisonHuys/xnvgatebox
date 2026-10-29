package main

import (
	"os"
	"testing"
)

func TestCDNDomainDetection(t *testing.T) {
	oldDomain := os.Getenv("FANOUT_CDN_DOMAIN")
	oldHost := os.Getenv("FANOUT_PUBLIC_HOST")
	oldIP := os.Getenv("FANOUT_PUBLIC_IP")
	defer func() {
		os.Setenv("FANOUT_CDN_DOMAIN", oldDomain)
		os.Setenv("FANOUT_PUBLIC_HOST", oldHost)
		os.Setenv("FANOUT_PUBLIC_IP", oldIP)
	}()

	os.Unsetenv("FANOUT_CDN_DOMAIN")
	os.Unsetenv("FANOUT_PUBLIC_HOST")
	os.Unsetenv("FANOUT_PUBLIC_IP")
	if cdnDomain() != "" {
		t.Fatalf("未设置时应返回空，实际为 %q", cdnDomain())
	}

	os.Setenv("FANOUT_PUBLIC_IP", "43.108.86.141")
	if cdnDomain() != "" {
		t.Fatalf("纯 IP 不应被识别为 CDN 域名，实际为 %q", cdnDomain())
	}

	os.Setenv("FANOUT_PUBLIC_IP", "ali.edisonhu.xyz")
	if cdnDomain() != "ali.edisonhu.xyz" {
		t.Fatalf("FANOUT_PUBLIC_IP 为域名时应返回，实际为 %q", cdnDomain())
	}

	os.Setenv("FANOUT_CDN_DOMAIN", "cf.example.com")
	if cdnDomain() != "cf.example.com" {
		t.Fatalf("FANOUT_CDN_DOMAIN 优先级最高，实际为 %q", cdnDomain())
	}
}

func TestCDNPathAndPort(t *testing.T) {
	oldDomain := os.Getenv("FANOUT_CDN_DOMAIN")
	oldPort := os.Getenv("FANOUT_CDN_PORT")
	oldPath := os.Getenv("FANOUT_CDN_PATH")
	defer func() {
		os.Setenv("FANOUT_CDN_DOMAIN", oldDomain)
		os.Setenv("FANOUT_CDN_PORT", oldPort)
		os.Setenv("FANOUT_CDN_PATH", oldPath)
	}()

	os.Setenv("FANOUT_CDN_DOMAIN", "ali.edisonhu.xyz")
	os.Unsetenv("FANOUT_CDN_PORT")
	os.Unsetenv("FANOUT_CDN_PATH")

	if cdnPort() != 443 {
		t.Fatalf("默认 CDN 端口应为 443，实际为 %d", cdnPort())
	}
	if cdnPathPrefix() != "/ali-ws" {
		t.Fatalf("ali 域名默认路径应为 /ali-ws，实际为 %q", cdnPathPrefix())
	}

	os.Setenv("FANOUT_CDN_PORT", "8443")
	os.Setenv("FANOUT_CDN_PATH", "custom-ray")
	if cdnPort() != 8443 {
		t.Fatalf("显式配置端口 8443 生效，实际为 %d", cdnPort())
	}
	if cdnPathPrefix() != "/custom-ray" {
		t.Fatalf("自定义路径应自动补全斜杠，实际为 %q", cdnPathPrefix())
	}
}
