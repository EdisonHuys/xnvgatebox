package main

import (
	"fmt"
	"net/http"
)

type SocksExposure struct {
	Port   int  `json:"port"`
	Enable bool `json:"enable"`
}

func (n *Native) socksExposures() map[int]*SocksExposure {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := map[int]*SocksExposure{}
	for _, ib := range n.store.Inbounds {
		if ib.SocksSlot > 0 {
			out[ib.SocksSlot] = &SocksExposure{Port: ib.Port, Enable: ib.Enable && !n.unavailable}
		}
	}
	return out
}

// Keep the internal hand-off private. External access is a separate authenticated
// Xray listener bound to the same stable Slot; its route never uses host-direct.
func (n *Native) setPublicSocks(t *Tunnel, enabled bool, tunnels []*Tunnel) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	finish := n.beginChange()
	defer finish()
	for _, ib := range n.store.Inbounds {
		if ib.SocksSlot == t.Slot {
			if ib.Enable == enabled {
				return nil
			}
			ib.Enable = enabled
			return n.apply(tunnels)
		}
	}
	if !enabled {
		return nil
	}
	cred := t.credential()
	if err := validateCred(cred); err != nil {
		return err
	}
	used := n.store.usedPorts()
	for _, tunnel := range tunnels {
		used[tunnel.Port] = true
	}
	port, err := freeRandomPort(used)
	if err != nil {
		return err
	}
	n.store.Inbounds = append(n.store.Inbounds, &nativeInbound{
		ID: n.store.NextID, Port: port, Protocol: "socks", Network: "tcp", Security: "none",
		Remark: fmt.Sprintf("SOCKS · %02d", t.Slot), NameMode: "custom", Enable: true,
		BoundSlot: t.Slot, SocksSlot: t.Slot,
		Clients: []nativeClient{{Email: cred.User, Password: cred.Pass, Enable: true}},
	})
	n.store.NextID++
	return n.apply(tunnels)
}

// A direct entry is created only through this explicit operation. Old unbound
// entries are never migrated into direct entries, and enabling it is idempotent.
func (n *Native) setHostDirect(enabled bool, serverName string, tunnels []*Tunnel) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	finish := n.beginChange()
	defer finish()
	for _, ib := range n.store.Inbounds {
		if ib.HostDirect {
			if ib.Enable == enabled {
				return nil
			}
			ib.Enable = enabled
			return n.apply(tunnels)
		}
	}
	if !enabled {
		return nil
	}
	used := n.store.usedPorts()
	for _, tunnel := range tunnels {
		used[tunnel.Port] = true
	}
	ib, err := n.prepareInbound(NewInboundSpec{
		Protocol: "vless", Network: "tcp", Security: "reality", Remark: "VPS 本机直出",
		Dest: serverName + ":443", ServerNames: serverName, Fingerprint: "chrome",
	}, used)
	if err != nil {
		return err
	}
	ib.HostDirect, ib.NameMode = true, "custom"
	n.store.Inbounds = append(n.store.Inbounds, ib)
	n.store.NextID++
	return n.apply(tunnels)
}

func (c *Console) apiHostDirect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := readConsoleJSON(w, r, &req); err != nil || req.Enabled == nil {
		writeJSON(w, 400, map[string]string{"error": "需要明确设置是否启用本机直出"})
		return
	}
	p, err := openPanel()
	n, ok := p.(*Native)
	if err != nil || !ok {
		writeJSON(w, 409, map[string]string{"error": "本机直出仅用于内置模式"})
		return
	}
	c.targetMu.Lock()
	defer c.targetMu.Unlock()
	if c.ctx.Err() != nil {
		w.WriteHeader(503)
		return
	}
	host := "www.tesla.com"
	if c.plan != nil {
		host = c.plan.ServerName
	}
	if err := n.setHostDirect(*req.Enabled, host, c.m.Tunnels()); err != nil {
		writeJSON(w, 409, map[string]string{"error": firstLine(err.Error())})
		return
	}
	invalidateInbounds()
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (c *Console) apiPublicSocks(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Only authenticated administration exposes credentials. Never publish
		// them in candidate observations, GitHub files or public subscriptions.
		view := c.m.ExitsOf()
		links := []map[string]any{}
		for _, e := range view.Exits {
			if e.Status == "up" && e.PublicSocks != nil && e.PublicSocks.Enable {
				links = append(links, map[string]any{"slot": e.Slot, "name": subscriptionExitName(e),
					"url": socksURL(publicHost(r), e.PublicSocks.Port, SocksCred{User: e.SocksUser, Pass: e.SocksPass})})
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]any{"links": links})
		return
	}
	var req struct {
		Slot    int   `json:"slot"`
		Enabled *bool `json:"enabled"`
	}
	if err := readConsoleJSON(w, r, &req); err != nil || req.Slot < 1 || req.Enabled == nil {
		writeJSON(w, 400, map[string]string{"error": "需要出口编号及外部访问开关"})
		return
	}
	p, err := openPanel()
	n, ok := p.(*Native)
	if err != nil || !ok {
		writeJSON(w, 409, map[string]string{"error": "外部 SOCKS 仅用于内置模式"})
		return
	}
	c.m.mu.RLock()
	t := c.m.tunnels[req.Slot]
	c.m.mu.RUnlock()
	if t == nil {
		writeJSON(w, 404, map[string]string{"error": "出口不存在"})
		return
	}
	t.changeMu.Lock()
	defer t.changeMu.Unlock()
	if !c.m.ownsTunnel(t) || c.ctx.Err() != nil {
		writeJSON(w, 409, map[string]string{"error": "出口已删除或服务正在停止"})
		return
	}
	t.opMu.Lock()
	defer t.opMu.Unlock()
	if err := n.setPublicSocks(t, *req.Enabled, c.m.Tunnels()); err != nil {
		writeJSON(w, 409, map[string]string{"error": firstLine(err.Error())})
		return
	}
	invalidateInbounds()
	writeJSON(w, 200, map[string]bool{"ok": true})
}
