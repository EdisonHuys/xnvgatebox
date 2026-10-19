package main

import "fmt"

// Persist binding intent even while its stable Slot is waiting/stopped. The
// generated route remains blackhole until that exact Slot becomes healthy.
func (n *Native) BindSlot(tag string, slot int, tunnels []*Tunnel) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	finish := n.beginChange()
	defer finish()
	var target *tunnelView
	if slot > 0 {
		for _, t := range tunnels {
			v := t.snapshot()
			if v.Slot == slot {
				target = &v
				break
			}
		}
		if target == nil {
			return fmt.Errorf("选择的出口不存在")
		}
	}
	for _, ib := range n.store.Inbounds {
		if ib.tag() != tag {
			continue
		}
		if ib.HostDirect || ib.SocksSlot > 0 || ib.ThirdParty != nil {
			return fmt.Errorf("此自动入口的出口固定，请新增手动入口选择其他出口")
		}
		if ib.ManagedSlot {
			for _, t := range tunnels {
				if t.Slot == ib.BoundSlot {
					return fmt.Errorf("自动入口的出口固定，请新增手动入口")
				}
			}
			ib.ManagedSlot, ib.Manual = false, true
		}
		ib.BoundSlot, ib.BoundTo = slot, ""
		if target != nil {
			ib.BoundTo = sanitizeTag(target.Node.HostName)
		}
		return n.apply(tunnels)
	}
	return fmt.Errorf("入站不存在")
}
