package main

import (
	"log"
	"os"
	"strings"
)

// startConfiguredSlot is deliberately opt-in.  It makes a first VPS install
// deterministic without opening a public admin API: the operator supplies a
// country (or an exact hostname), and the manager creates slot 1 using the
// fixed port base.  Later slots remain managed by the WebUI/API.
func startConfiguredSlot(m *Manager) {
	if len(m.Tunnels()) != 0 {
		return
	}
	region := strings.ToUpper(strings.TrimSpace(os.Getenv("FANOUT_INITIAL_REGION")))
	host := strings.TrimSpace(os.Getenv("FANOUT_INITIAL_NODE"))
	if region == "" && host == "" {
		return
	}
	var selected Node
	var found bool
	if host != "" {
		nodes, _ := m.Nodes()
		for _, n := range nodes {
			if n.HostName == host {
				selected, found = n, true
				break
			}
		}
	} else {
		picks := m.policyCandidates(defaultSlotPolicy(region), nil, 0)
		if len(picks) > 0 {
			selected = picks[0]
		} else {
			selected = Node{CountryCode: region, Country: region}
		}
		found = true
	}
	if !found {
		log.Printf("初始 Exit Slot 找不到节点: region=%s host=%s", region, host)
		return
	}
	t, err := m.Start(selected)
	if err != nil {
		log.Printf("初始 Exit Slot 启动失败: %v", err)
		return
	}
	log.Printf("初始 Exit Slot 已排队: slot=%d port=%d node=%s", t.Slot, t.Port, t.Node.HostName)
}
