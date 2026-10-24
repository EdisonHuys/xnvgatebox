package main

import "fmt"

const defaultExitTarget = 16

// Caller holds targetMu. Run after state restore, and only when a candidate
// pool exists. Existing plans and legacy Slots represent user intent.
func (c *Console) initializeDefaultTarget() error {
	if c.plan != nil || len(c.m.Tunnels()) != 0 || currentPanelMode() != "native" {
		return nil
	}
	nodes, _ := c.m.Nodes()
	if len(nodes) == 0 {
		return nil
	}
	p := ExitPlan{Count: defaultExitTarget, Policy: defaultSlotPolicyForMode("", false)}
	if panel, err := openPanel(); err == nil {
		if n, ok := panel.(*Native); ok {
			n.mu.Lock()
			for _, ib := range n.store.Inbounds {
				if ib.Reality != nil && len(ib.Reality.ServerNames) > 0 {
					p.ServerName = ib.Reality.ServerNames[0]
					break
				}
			}
			n.mu.Unlock()
		}
	}
	if err := validateExitPlan(&p, c.m.maxSlots); err != nil {
		return fmt.Errorf("默认 16 个出口无法初始化: %w", err)
	}
	return c.savePlan(p)
}
