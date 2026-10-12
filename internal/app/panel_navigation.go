package app

// Browsing and inline interactions share direction, edge and exclusion rules.
// Tab cycles; spatial arrows stop at the edge. Transfers exclude their source.
func (v *view) panelNeighbor(direction int, cycle bool, excluded int) int {
	target := v.active
	for range len(v.panels) {
		target += direction
		if cycle {
			target = (target + len(v.panels)) % len(v.panels)
		} else if target < 0 || target >= len(v.panels) {
			return v.active
		}
		if target != excluded {
			return target
		}
	}
	return v.active
}

func (v *view) switchPanel(direction int) {
	v.switchPanelTo(v.panelNeighbor(direction, false, -1))
}

func (v *view) switchPanelTo(target int) {
	if target != v.active && v.actions != nil {
		v.cancelInlineActions()
	}
	v.panels[v.active].prefix = ""
	v.active = target
}
