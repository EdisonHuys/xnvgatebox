package main

import (
	"fmt"
	"net/http"
)

func slotOutboundTag(slot int) string { return fmt.Sprintf("slot-%02d", slot) }

func slotOutbounds(tunnels []*Tunnel) []any {
	// Xray's first outbound is the default for requests without a matching rule.
	outs := []any{map[string]any{"tag": "block", "protocol": "blackhole"}}
	for _, t := range tunnels {
		if t.Slot < 1 {
			continue
		}
		out := map[string]any{"tag": slotOutboundTag(t.Slot), "protocol": "blackhole"}
		if t.snapshot().Status == "up" {
			out["protocol"] = "socks"
			out["settings"] = map[string]any{"servers": []any{socksServerJSON(t)}}
		}
		outs = append(outs, out)
	}
	return outs
}

// Export a standard fragment for manual integration without writing 3x-ui's DB.
func apiExportXray(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		tunnels := m.exportTunnels()
		rules := []any{}
		for _, t := range tunnels {
			if t.Slot > 0 {
				rules = append(rules, map[string]any{"type": "field",
					"inboundTag":  []string{"inbound-" + slotOutboundTag(t.Slot)},
					"outboundTag": slotOutboundTag(t.Slot)})
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Disposition", `attachment; filename="xnvgatebox-xray.json"`)
		writeJSON(w, http.StatusOK, map[string]any{"outbounds": slotOutbounds(tunnels),
			"routing": map[string]any{"rules": rules}})
	}
}
