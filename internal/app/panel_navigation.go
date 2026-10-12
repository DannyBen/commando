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
	if target != v.active && v.inlineInteraction() != nil {
		v.cancelInlineActions()
	}
	v.panels[v.active].prefix = ""
	v.active = target
}

// Swap entire pane models so their location, listing, selection and viewport
// travel together. Worker requests capture pane indices, so wait until all
// directory work and inline interactions have finished before rearranging them.
func (v *view) swapPanel(direction int) {
	if !v.interaction.idle() || !v.actionAllowed(actionMutation) {
		return
	}
	for i := range v.panels {
		if v.panels[i].loading || v.panels[i].refreshing {
			return
		}
	}
	source, target := v.active, v.panelNeighbor(direction, false, -1)
	if target == source {
		return
	}
	v.panels[source], v.panels[target] = v.panels[target], v.panels[source]
	// Cut appearance caches resolved aliases by pane index; move those cached
	// locations with their panes without touching the system clipboard.
	for _, paths := range [][]string{v.cutPanelPaths, v.cutLocations} {
		if source < len(paths) && target < len(paths) {
			paths[source], paths[target] = paths[target], paths[source]
		}
	}
	v.active = target
}
