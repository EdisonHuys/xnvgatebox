package main

import "fmt"

// A per-instance /16 contains this many /30 links. This is an address-space
// constraint, not a machine-size or product tier. Diagnostics share free links.
const networkLinkCount = 1 << 14
const maximumSlots = networkLinkCount
const diagnosticFirstSlot = -1
const defaultCheckParallelism = 2

// Preserve every legacy positive Slot's /24-based link. Larger identities use
// the previously unused /30 links without overlapping any legacy address.
func slotNetworkIndex(slot int) int {
	if slot >= 1 && slot <= 255 {
		return slot * 64
	}
	if slot == 256 {
		return 0
	}
	if slot > 256 && slot <= maximumSlots {
		rank := slot - 257
		return (rank/63)*64 + rank%63 + 1
	}
	return -1
}

func (t *Tunnel) linkIndex() int {
	if t.networkIndex != nil {
		return *t.networkIndex
	}
	return slotNetworkIndex(t.Slot)
}

func (t *Tunnel) linkAddress(offset int) string {
	n := t.linkIndex()*4 + offset
	return fmt.Sprintf("10.%d.%d.%d", instBase, n/256, n%256)
}

func (t *Tunnel) linkCIDR() string { return t.linkAddress(0) + "/30" }
