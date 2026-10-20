package app

import "slices"

var panelSizeSteps = []float64{0.6, 0.8, 1, 1.2, 1.5, 2}

// Missing or unsupported saved values use the normal panel size.
func panelSizeRatio(ratio float64) float64 {
	if slices.Contains(panelSizeSteps, ratio) {
		return ratio
	}
	return 1
}

func panelSizingReady(v *view) bool {
	return v.canOfferInteraction(v.active, actionBrowse)
}

// Ratios divide the available workspace; resizing never changes window geometry
// or the base width used when adding and removing panels.
func (v *view) resizePanel(direction int) {
	if !v.canStartInteraction(v.active, actionBrowse) {
		return
	}
	p := &v.panels[v.active]
	previous := panelSizeRatio(p.sizeRatio)
	ratio := 1.0
	if direction != 0 {
		index := slices.Index(panelSizeSteps, previous)
		ratio = panelSizeSteps[max(0, min(len(panelSizeSteps)-1, index+direction))]
	}
	p.sizeRatio = ratio
	p.prefix = ""
	if ratio != previous {
		v.announcePanelResize(ratio)
	}
}
