package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

func apiRollbackSlotPolicy(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		slot, err := strconv.Atoi(r.URL.Query().Get("slot"))
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "slot 参数无效"})
			return
		}
		if err := m.RollbackSlotPolicy(slot); err != nil {
			writeJSON(w, 409, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"ok": "已恢复上一次策略；运行中的 Slot 将重新验活"})
	}
}

func apiRollbackNative(m *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		p, err := openPanel()
		if err != nil {
			writeJSON(w, 503, map[string]string{"error": "内置配置后端不可用"})
			return
		}
		n, ok := p.(*Native)
		if !ok {
			writeJSON(w, 400, map[string]string{"error": "仅支持内置 Xray 配置回滚"})
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodGet {
			_, err := os.Stat(filepath.Join(n.dir, "native.previous.json"))
			writeJSON(w, 200, map[string]bool{"available": err == nil})
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		if err := n.Rollback(m.Tunnels()); err != nil {
			writeJSON(w, 409, map[string]string{"error": fmt.Sprint(err)})
			return
		}
		invalidateInbounds()
		writeJSON(w, 200, map[string]string{"ok": "已回滚上一次内置入站、客户端及绑定配置；路由按当前 Slot 状态生成"})
	}
}
