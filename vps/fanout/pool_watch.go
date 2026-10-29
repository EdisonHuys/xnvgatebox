package main

import (
	"log"
	"os"
	"strings"
	"time"
)

// Local systemd builds the same pool; the manager only reads atomic snapshots.
func (m *Manager) WatchNodePool() {
	path := strings.TrimSpace(os.Getenv("FANOUT_VALIDATED_POOL_FILE"))
	if path == "" {
		path = "/var/lib/xnvgatebox/pool/node_pool.json"
	}
	var loaded time.Time
	for {
		if info, err := os.Stat(path); err == nil && !info.ModTime().Equal(loaded) {
			if count, err := m.RefreshNodes(); err != nil {
				log.Printf("共享池更新失败，保留当前池: %v", err)
			} else {
				loaded = info.ModTime()
				log.Printf("共享池已更新: %d 个候选", count)
			}
		}
		time.Sleep(30 * time.Second)
	}
}
