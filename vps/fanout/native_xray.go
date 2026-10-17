package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// xrayCandidates 是自建模式查找 xray 二进制的位置，按优先级排列。
// 优先用 fanout 自己装的那份，避免和别人的 xray 抢版本。
func xrayCandidates(workDir string) []string {
	return []string{
		filepath.Join(workDir, "bin", "xray"),
		"/usr/local/bin/xray",
		"/usr/bin/xray",
		// 接管 3x-ui 时机器上通常只有面板自带的这份，文件名带平台后缀
		fmt.Sprintf("/usr/local/x-ui/bin/xray-%s-%s", runtime.GOOS, xuiArchSuffix()),
	}
}

// xuiArchSuffix 把 Go 的 GOARCH 映射成 3x-ui 给 xray 二进制起名用的后缀。
func xuiArchSuffix() string {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "arm":
		return "arm32"
	case "s390x":
		return "s390x"
	default:
		return runtime.GOARCH
	}
}

// findXray 定位可执行的 xray。
func findXray(workDir string) (string, error) {
	for _, p := range xrayCandidates(workDir) {
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0111 != 0 {
			return p, nil
		}
	}
	if p, err := exec.LookPath("xray"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("找不到 xray 可执行文件，装一份到 %s 或 /usr/local/bin/xray",
		filepath.Join(workDir, "bin", "xray"))
}

// buildXrayConfig 由入站列表和当前隧道生成完整的 Xray 运行配置。
//
// Every slot keeps its outbound identity when the backing node changes.
// Unbound, unavailable and stale bindings are blocked, never sent via the host.
func buildXrayConfig(inbounds []*nativeInbound, tunnels []*Tunnel) map[string]any {
	bound := map[string]string{}
	slots := map[int]bool{}
	for _, t := range tunnels {
		if t.Slot > 0 {
			slots[t.Slot] = true
			if host := t.snapshot().Node.HostName; host != "" {
				bound[sanitizeTag(host)] = slotOutboundTag(t.Slot)
			}
		}
	}

	ins := make([]any, 0, len(inbounds))
	for _, ib := range inbounds {
		if !ib.Enable {
			continue
		}
		ins = append(ins, nativeInboundJSON(ib))
	}

	outs := slotOutbounds(tunnels)

	rules := []any{}
	for _, ib := range inbounds {
		if !ib.Enable {
			continue
		}
		tag := bound[ib.BoundTo]
		if ib.BoundSlot > 0 {
			tag = ""
			if slots[ib.BoundSlot] {
				tag = slotOutboundTag(ib.BoundSlot)
			}
		}
		if tag == "" {
			tag = "block"
		}
		rules = append(rules, map[string]any{
			"type":        "field",
			"inboundTag":  []any{ib.tag()},
			"outboundTag": tag,
		})
	}

	return map[string]any{
		"log":       map[string]any{"loglevel": "warning"},
		"inbounds":  ins,
		"outbounds": outs,
		"routing": map[string]any{
			"domainStrategy": "AsIs",
			"rules":          rules,
		},
	}
}

// nativeInboundJSON 把一个入站转成 Xray 的 inbound 配置。
func nativeInboundJSON(ib *nativeInbound) map[string]any {
	settings := map[string]any{}
	clients := make([]any, 0, len(ib.Clients))
	for _, c := range ib.Clients {
		if !c.Enable {
			continue
		}
		switch ib.Protocol {
		case "trojan":
			clients = append(clients, map[string]any{"password": c.Password, "email": c.Email})
		case "vmess":
			clients = append(clients, map[string]any{"id": c.ID, "email": c.Email})
		default: // vless
			clients = append(clients, map[string]any{"id": c.ID, "email": c.Email, "flow": c.Flow})
		}
	}
	settings["clients"] = clients
	if ib.Protocol == "vless" {
		settings["decryption"] = "none"
	}

	return map[string]any{
		"tag":            ib.tag(),
		"listen":         "0.0.0.0",
		"port":           ib.Port,
		"protocol":       ib.Protocol,
		"settings":       settings,
		"streamSettings": streamSettingsJSON(ib),
		"sniffing":       map[string]any{"enabled": true, "destOverride": []any{"http", "tls"}},
	}
}

// streamSettingsJSON 生成传输层配置：网络类型 + 安全层。
func streamSettingsJSON(ib *nativeInbound) map[string]any {
	network := ib.netOrTCP()
	stream := map[string]any{"network": network, "security": ib.securityOrNone()}

	path := ib.Path
	if path == "" {
		path = "/"
	}
	switch network {
	case "ws":
		ws := map[string]any{"path": path}
		if ib.Host != "" {
			ws["host"] = ib.Host
		}
		stream["wsSettings"] = ws
	case "httpupgrade":
		hu := map[string]any{"path": path}
		if ib.Host != "" {
			hu["host"] = ib.Host
		}
		stream["httpupgradeSettings"] = hu
	case "xhttp":
		xh := map[string]any{"path": path, "mode": "auto"}
		if ib.Host != "" {
			xh["host"] = ib.Host
		}
		stream["xhttpSettings"] = xh
	case "grpc":
		// gRPC 没有 path，用 serviceName 区分；沿用 Path 字段少一个概念
		name := strings.TrimPrefix(ib.Path, "/")
		stream["grpcSettings"] = map[string]any{"serviceName": name}
	}

	switch ib.securityOrNone() {
	case "tls":
		if ib.TLS != nil {
			t := map[string]any{
				"certificates": []any{map[string]any{
					"certificateFile": ib.TLS.CertFile,
					"keyFile":         ib.TLS.KeyFile,
				}},
			}
			if ib.TLS.ServerName != "" {
				t["serverName"] = ib.TLS.ServerName
			}
			stream["tlsSettings"] = t
		}
	case "reality":
		if ib.Reality != nil {
			r := map[string]any{
				"dest":        ib.Reality.Dest,
				"serverNames": toAnySlice(ib.Reality.ServerNames),
				"privateKey":  ib.Reality.PrivateKey,
				"shortIds":    toAnySlice(ib.Reality.ShortIDs),
			}
			stream["realitySettings"] = r
		}
	}
	return stream
}

func toAnySlice(in []string) []any {
	out := make([]any, 0, len(in))
	for _, s := range in {
		out = append(out, s)
	}
	return out
}

// writeXrayConfig 把配置写到磁盘，返回配置路径。
func writeXrayConfig(dir string, cfg map[string]any) (string, error) {
	blob, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "xray.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, blob, 0600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}

// verifyXrayConfig 用 xray 自己的校验器检查配置。
//
// 先校验再重启：配置写坏时进程会起不来，而那时旧进程已经被杀掉，
// 所有节点链接会一起断掉。校验能把这类错误挡在重启之前。
func verifyXrayConfig(bin, cfgPath string) error {
	_, err := cmdCombined(exec.Command(bin, "run", "-test", "-c", cfgPath))
	if err != nil {
		return fmt.Errorf("Xray 配置校验失败: %w", err)
	}
	return nil
}

func trimOutput(b []byte) string {
	s := string(b)
	if len(s) > 400 {
		s = s[:400] + "..."
	}
	return s
}

// xrayProc 管理自建模式下的 xray 进程。
type xrayProc struct {
	bin  string
	dir  string
	cmd  *exec.Cmd
	logf *os.File
	done chan error
}

func (p *xrayProc) verify(path string) error { return verifyXrayConfig(p.bin, path) }

// restart 用当前配置重启 xray。配置已在调用前写好并校验过。
func (p *xrayProc) restart(cfgPath string) error {
	p.stop()

	logPath := filepath.Join(p.dir, "xray.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("打开 Xray 日志失败: %w", err)
	}

	cmd := exec.Command(p.bin, "run", "-c", cfgPath)
	cmd.Stdout = f
	cmd.Stderr = f
	// 必须从母机的网络命名空间拉起：直接 Start 有概率把 Xray 拉进某条隧道，
	// 那样入站端口只在隧道内部监听，母机上看不到，客户端全连不上
	// 而进程、配置、日志三处都显示正常（见 netnsguard.go）
	if err := cmdStart(cmd); err != nil {
		f.Close()
		return fmt.Errorf("启动 Xray 失败: %w", err)
	}
	p.cmd, p.logf = cmd, f
	p.done = make(chan error, 1)
	finished := p.done
	go func() { finished <- cmd.Wait() }()
	if err := p.writePID(cmd.Process.Pid); err != nil {
		p.stop()
		return fmt.Errorf("保存 Xray 进程标识失败: %w", err)
	}

	// 起得来但立刻退出的情况要能被发现，否则界面会显示成功而实际不通
	select {
	case <-p.done:
		p.cmd = nil
		p.done = nil
		return fmt.Errorf("Xray 启动后立刻退出，详见 %s", logPath)
	case <-time.After(400 * time.Millisecond):
		return nil
	}
}

func (p *xrayProc) stop() {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		if p.done != nil {
			select {
			case <-p.done:
			case <-time.After(2 * time.Second):
			}
		}
		p.cmd = nil
	}
	p.done = nil
	if p.logf != nil {
		p.logf.Close()
		p.logf = nil
	}
	_ = os.Remove(p.pidPath())
}

func (p *xrayProc) pidPath() string { return filepath.Join(p.dir, "xray.pid") }

func (p *xrayProc) writePID(pid int) error {
	return atomicConfigWrite(p.pidPath(), []byte(strconv.Itoa(pid)))
}

// reapOrphan 清掉上次遗留的 Xray。
//
// fanout 被 SIGKILL 时来不及停子进程，遗留的 Xray 仍占着入站端口，
// 下次启动会因端口冲突起不来。这里按 pidfile 精确定位，
// 并核对可执行文件确实是我们启动的那个，避免误杀同名进程。
func (p *xrayProc) reapOrphan() {
	blob, err := os.ReadFile(p.pidPath())
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(blob)))
	if err != nil || pid <= 1 {
		_ = os.Remove(p.pidPath())
		return
	}
	// pid 会被系统回收给别的进程，只有确认可执行文件一致才动手
	if exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil {
		if exe == p.bin {
			if proc, err := os.FindProcess(pid); err == nil {
				_ = proc.Kill()
			}
		}
	}
	_ = os.Remove(p.pidPath())
}
