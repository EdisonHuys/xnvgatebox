package main

import (
	"strings"
	"testing"
)

func TestNativeInboundTagMatchesXUIFormat(t *testing.T) {
	// tag 格式必须和 3x-ui 一致，否则两种后端的绑定语义会对不上
	cases := []struct {
		ib   nativeInbound
		want string
	}{
		{nativeInbound{Port: 443, Network: "tcp"}, "in-443-tcp"},
		{nativeInbound{Port: 8080, Network: "ws"}, "in-8080-ws"},
		{nativeInbound{Port: 1234}, "in-1234-tcp"}, // 缺省按 tcp
	}
	for _, c := range cases {
		if got := c.ib.tag(); got != c.want {
			t.Errorf("tag() = %q, want %q", got, c.want)
		}
	}
}

func TestBuildXrayConfigBlocksUnavailableAndUnbound(t *testing.T) {
	up := &Tunnel{Slot: 1, Port: 17928, Status: "up", Node: Node{HostName: "jp1"}}
	down := &Tunnel{Slot: 2, Port: 17929, Status: "failed", Node: Node{HostName: "jp2"}}
	inbounds := []*nativeInbound{
		{ID: 1, Port: 100, Protocol: "vless", Enable: true, BoundTo: "jp1"},
		{ID: 2, Port: 200, Protocol: "vless", Enable: true, BoundTo: "jp2"},
		{ID: 3, Port: 300, Protocol: "vless", Enable: true},
		{ID: 4, Port: 400, Protocol: "vless", Enable: false, BoundTo: "jp1"},
	}
	cfg := buildXrayConfig(inbounds, []*Tunnel{up, down})
	outs := cfg["outbounds"].([]any)
	if outs[0].(map[string]any)["protocol"] != "blackhole" {
		t.Fatal("default outbound must block")
	}
	protocols := map[string]any{}
	for _, o := range outs {
		m := o.(map[string]any)
		protocols[m["tag"].(string)] = m["protocol"]
	}
	if protocols["slot-01"] != "socks" || protocols["slot-02"] != "blackhole" {
		t.Fatalf("unexpected outbounds: %v", protocols)
	}
	rules := cfg["routing"].(map[string]any)["rules"].([]any)
	if len(rules) != 3 {
		t.Fatalf("all enabled inbounds must have explicit routes: %v", rules)
	}
	for i, want := range []string{"slot-01", "slot-02", "block"} {
		if rules[i].(map[string]any)["outboundTag"] != want {
			t.Fatalf("route %d must use %s", i, want)
		}
	}
}

func TestSlotOutboundSurvivesNodeReplacement(t *testing.T) {
	for _, host := range []string{"node-a", "node-b"} {
		for _, status := range []string{"up", "starting", "failed"} {
			outs := slotOutbounds([]*Tunnel{{Slot: 3, Port: 17930, Status: status, Node: Node{HostName: host}}})
			if outs[1].(map[string]any)["tag"] != "slot-03" {
				t.Fatal("slot tag changed")
			}
			if status == "up" {
				server := outs[1].(map[string]any)["settings"].(map[string]any)["servers"].([]any)[0].(map[string]any)
				if server["port"] != 17930 || server["address"] != "127.0.0.1" {
					t.Fatalf("slot address changed: %v", server)
				}
			}
		}
	}
}

func TestShareLinkPerProtocol(t *testing.T) {
	c := nativeClient{ID: "uuid-1", Password: "pw-1", Email: "e", Enable: true}

	vless := shareLink(&nativeInbound{Port: 100, Protocol: "vless", Remark: "r"}, c, "1.2.3.4")
	if !strings.HasPrefix(vless, "vless://uuid-1@1.2.3.4:100?") {
		t.Errorf("vless 链接格式不对: %s", vless)
	}
	if !strings.Contains(vless, "encryption=none") {
		t.Errorf("vless 需要 encryption=none: %s", vless)
	}

	tro := shareLink(&nativeInbound{Port: 200, Protocol: "trojan", Network: "ws", Path: "/p"}, c, "h")
	if !strings.HasPrefix(tro, "trojan://pw-1@h:200?") {
		t.Errorf("trojan 应当用密码而不是 UUID: %s", tro)
	}
	if !strings.Contains(tro, "path=%2Fp") {
		t.Errorf("ws 链接要带 path: %s", tro)
	}
}

// 复制出来的入站直接叫出口的名字，不再拼模板备注。
// 以前拼接会在"拿复制品当模板再复制"时叠成 fanout-JP-243-VN-165 这种。
func TestUniqueRemark(t *testing.T) {
	taken := map[string]bool{}
	if got := uniqueRemark("🇯🇵 日本 54", taken); got != "🇯🇵 日本 54" {
		t.Errorf("没撞名时应原样返回，实际 %q", got)
	}
	taken["🇯🇵 日本 54"] = true
	// 同一条出口挂第二个节点时要加序号，mihomo 那边名字重了会丢节点
	if got := uniqueRemark("🇯🇵 日本 54", taken); got != "🇯🇵 日本 54 2" {
		t.Errorf("撞名应加序号，实际 %q", got)
	}
	taken["🇯🇵 日本 54 2"] = true
	if got := uniqueRemark("🇯🇵 日本 54", taken); got != "🇯🇵 日本 54 3" {
		t.Errorf("第三个应是 3，实际 %q", got)
	}
}

// 换节点只改 fanout 自己起的名字，用户手工改过的不碰。
func TestRenameExitLabel(t *testing.T) {
	if got := renameExitLabel("🇯🇵 日本 54", "🇰🇷 韩国 15"); got != "🇰🇷 韩国 15" {
		t.Errorf("自动名应整体换掉，实际 %q", got)
	}
	for _, mine := range []string{"线路A", "给老王的", "JP-244", ""} {
		if got := renameExitLabel(mine, "🇰🇷 韩国 15"); got != mine {
			t.Errorf("手工备注 %q 不该被改成 %q", mine, got)
		}
	}
	// 新名字算不出来时别把原来的抹掉
	if got := renameExitLabel("🇯🇵 日本 54", ""); got != "🇯🇵 日本 54" {
		t.Errorf("新标签为空时应保持原样，实际 %q", got)
	}
}

func TestIsGeneratedLabel(t *testing.T) {
	for _, s := range []string{"🇯🇵 日本 54", "🇺🇸 美国 1", " 🇰🇷 韩国 15"} {
		if !isGeneratedLabel(s) {
			t.Errorf("%q 是自动生成的名字", s)
		}
	}
	for _, s := range []string{"", "线路A", "JP-244", "fanout-JP-243", "日本 54"} {
		if isGeneratedLabel(s) {
			t.Errorf("%q 不是自动生成的名字", s)
		}
	}
}

func TestVisionCapable(t *testing.T) {
	// Vision 只在 VLESS + 裸 TCP + TLS/REALITY 下有效，其他组合 Xray 会拒绝启动
	if !visionCapable("vless", "tcp", "reality") {
		t.Error("vless/tcp/reality 应当支持 vision")
	}
	if !visionCapable("vless", "tcp", "tls") {
		t.Error("vless/tcp/tls 应当支持 vision")
	}
	if visionCapable("vless", "ws", "tls") {
		t.Error("ws 不该支持 vision")
	}
	if visionCapable("vless", "tcp", "none") {
		t.Error("没有安全层时不该支持 vision")
	}
	if visionCapable("trojan", "tcp", "tls") {
		t.Error("vision 是 VLESS 专属")
	}
}

func TestStreamSettingsPerNetwork(t *testing.T) {
	cases := []struct {
		ib      nativeInbound
		key     string
		wantKey string
		want    any
	}{
		{nativeInbound{Network: "ws", Path: "/p"}, "wsSettings", "path", "/p"},
		{nativeInbound{Network: "httpupgrade", Path: "/h"}, "httpupgradeSettings", "path", "/h"},
		{nativeInbound{Network: "xhttp", Path: "/x"}, "xhttpSettings", "path", "/x"},
		// gRPC 没有 path，Path 字段复用为 serviceName，且不带前导斜杠
		{nativeInbound{Network: "grpc", Path: "/svc"}, "grpcSettings", "serviceName", "svc"},
	}
	for _, c := range cases {
		st := streamSettingsJSON(&c.ib)
		sub, ok := st[c.key].(map[string]any)
		if !ok {
			t.Errorf("%s 缺少 %s", c.ib.Network, c.key)
			continue
		}
		if got := sub[c.wantKey]; got != c.want {
			t.Errorf("%s 的 %s = %v, want %v", c.ib.Network, c.wantKey, got, c.want)
		}
	}
}

func TestStreamSettingsReality(t *testing.T) {
	ib := nativeInbound{
		Network: "tcp", Security: "reality",
		Reality: &realityConfig{
			Dest: "www.cloudflare.com:443", ServerNames: []string{"www.cloudflare.com"},
			PrivateKey: "priv", PublicKey: "pub", ShortIDs: []string{"abcd1234"},
		},
	}
	st := streamSettingsJSON(&ib)
	if st["security"] != "reality" {
		t.Fatalf("security = %v", st["security"])
	}
	r, ok := st["realitySettings"].(map[string]any)
	if !ok {
		t.Fatal("缺少 realitySettings")
	}
	if r["privateKey"] != "priv" {
		t.Errorf("服务端要写私钥，实际 %v", r["privateKey"])
	}
	// 公钥只有客户端用，写进服务端配置会被 Xray 拒绝
	if _, leaked := r["publicKey"]; leaked {
		t.Error("服务端配置不该出现 publicKey")
	}
}

func TestShareLinkCarriesSecurityParams(t *testing.T) {
	c := nativeClient{ID: "uuid-1", Enable: true, Flow: "xtls-rprx-vision"}

	re := shareLink(&nativeInbound{
		Port: 100, Protocol: "vless", Network: "tcp", Security: "reality", Remark: "r",
		Reality: &realityConfig{
			ServerNames: []string{"www.cloudflare.com"}, PublicKey: "PBK",
			ShortIDs: []string{"sid1"}, Fingerprint: "chrome",
		},
	}, c, "h")
	for _, want := range []string{"pbk=PBK", "sid=sid1", "fp=chrome",
		"sni=www.cloudflare.com", "flow=xtls-rprx-vision"} {
		if !strings.Contains(re, want) {
			t.Errorf("REALITY 链接缺少 %s: %s", want, re)
		}
	}

	// 自签证书验不过 CA，链接必须带指纹，否则客户端连不上
	tl := shareLink(&nativeInbound{
		Port: 200, Protocol: "vless", Network: "tcp", Security: "tls", Remark: "t",
		TLS: &tlsConfig{ServerName: "demo.local", SelfSigned: true, CertSha256: "AABB"},
	}, nativeClient{ID: "u", Enable: true}, "h")
	if !strings.Contains(tl, "pinSHA256=AABB") {
		t.Errorf("自签 TLS 链接要带证书指纹: %s", tl)
	}
}
