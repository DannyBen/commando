package app

import (
	"time"

	"gioui.org/layout"
)

func (v *view) dragAvailable(index, row int) bool {
	return v.nativeItemsAvailable(index, row, true, shellDrag)
}

func (v *view) startDrag(gtx layout.Context, index, row int) {
	if !v.canStartInteraction(index, actionMutation) || !v.dragAvailable(index, row) {
		return
	}
	p := &v.panels[index]
	paths := p.selectedPaths()
	if !p.marks[row] {
		paths = []string{p.entryPath(row)}
	}
	s := &nativeSession{panel: index, request: shellRequest{action: shellDrag, paths: paths, queued: time.Now()}}
	if !v.startInteraction(s) {
		return
	}
	v.dismissFeedback()
	v.panels[v.active].prefix = ""
	v.active = index
	p.selected, p.markRange = row, nil
	p.prefix, p.reveal = "", true
	v.focusInteraction(gtx)
}
