package app

import (
	"slices"

	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/dannyben/commando/internal/feedback"
)

type closedPanel struct {
	panel                panel
	index                int
	cutPath, cutLocation string
	clipboardSequence    uint32
}

func panelsCanChange(v *view) bool {
	return panelsCanSwap(v) && v.pendingNameDraft == nil
}

func (p panel) clone() panel {
	p.items, p.entries = slices.Clone(p.items), slices.Clone(p.entries)
	p.rows = make(map[int]*fileRow)
	p.emptySpace = nil
	p.locationBar = nil
	p.marks, p.markRange = nil, nil
	p.list = widget.List{List: layout.List{Axis: layout.Vertical, Position: p.list.Position}}
	p.feedback = feedback.Channel{}
	p.pendingFeedback = nil
	p.prefix = ""
	p.err = nil
	p.reveal, p.loading, p.refreshing, p.refreshNeeded = false, false, false, false
	return p
}

func (v *view) insertPanel(index int, p panel, cutPath, cutLocation string) {
	p.folderSizes = v.folderSizes
	// Alias caches have one slot per live panel. Keep the clipboard itself
	// untouched while inserting, removing, or restoring pane models.
	for len(v.cutPanelPaths) < len(v.panels) {
		v.cutPanelPaths = append(v.cutPanelPaths, "")
	}
	for len(v.cutLocations) < len(v.panels) {
		v.cutLocations = append(v.cutLocations, "")
	}
	v.panels = slices.Insert(v.panels, index, p)
	v.cutPanelPaths = slices.Insert(v.cutPanelPaths, index, cutPath)
	v.cutLocations = slices.Insert(v.cutLocations, index, cutLocation)
	v.active = index
	v.announceInteraction(eventSelectionChanged)
}

func (v *view) addPanel(direction int) bool {
	if !v.interaction.idle() || !panelsCanChange(v) {
		return false
	}
	source := v.active
	v.panels[source].prefix = ""
	p := v.panels[source].clone()
	cutPath, cutLocation := "", ""
	if source < len(v.cutPanelPaths) {
		cutPath = v.cutPanelPaths[source]
	}
	if source < len(v.cutLocations) {
		cutLocation = v.cutLocations[source]
	}
	index := source
	if direction > 0 {
		index++
	}
	v.insertPanel(index, p, cutPath, cutLocation)
	return true
}

func (v *view) closePanel() {
	if len(v.panels) <= 1 || !v.interaction.idle() || !panelsCanChange(v) {
		return
	}
	index := v.active
	closed := closedPanel{panel: v.panels[index], index: index, clipboardSequence: v.cutClipboard.Sequence}
	if index < len(v.cutPanelPaths) {
		closed.cutPath = v.cutPanelPaths[index]
		v.cutPanelPaths = slices.Delete(v.cutPanelPaths, index, index+1)
	}
	if index < len(v.cutLocations) {
		closed.cutLocation = v.cutLocations[index]
		v.cutLocations = slices.Delete(v.cutLocations, index, index+1)
	}
	v.closedPanels = append(v.closedPanels, closed)
	v.panels = slices.Delete(v.panels, index, index+1)
	v.active = min(index, len(v.panels)-1)
	v.announceInteraction(eventSelectionChanged)
}

func (v *view) reopenPanel() {
	if len(v.closedPanels) == 0 || !v.interaction.idle() || !panelsCanChange(v) {
		return
	}
	last := len(v.closedPanels) - 1
	closed := v.closedPanels[last]
	v.closedPanels = slices.Delete(v.closedPanels, last, last+1)
	if closed.clipboardSequence != v.cutClipboard.Sequence {
		closed.cutPath, closed.cutLocation = "", ""
	}
	v.insertPanel(min(closed.index, len(v.panels)), closed.panel, closed.cutPath, closed.cutLocation)
	// Reconcile changes made while closed without losing surviving marks or
	// viewport anchors. Drive refresh retains selection through its usual read.
	if v.loader != nil {
		if closed.panel.drives {
			v.queueLoad(loadRequest{panel: v.active, path: "", refresh: true})
		} else {
			v.panels[v.active].refreshNeeded = true
		}
	}
}
