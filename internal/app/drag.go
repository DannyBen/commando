package app

import (
	"time"

	"gioui.org/layout"
	"github.com/dannyben/commando/internal/feedback"
)

func (v *view) dragAvailable(index, row int, armed bool) bool {
	return v.nativeItemsAvailable(index, row, !armed, shellDrag)
}

func (v *view) startDrag(gtx layout.Context, index, row int, armed bool) {
	if !v.canStartInteraction(index, actionMutation) || !v.dragAvailable(index, row, armed) {
		return
	}
	p := &v.panels[index]
	paths := p.selectedPaths()
	if !armed && !p.marks[row] {
		paths = []string{p.entryPath(row)}
	}
	s := &nativeSession{panel: index, request: shellRequest{action: shellDrag, armed: armed, paths: paths, queued: time.Now()}}
	if !v.startInteraction(s) {
		return
	}
	v.dismissFeedback()
	if !armed {
		v.panels[v.active].prefix = ""
		v.active = index
		p.selected, p.markRange = row, nil
	}
	p.prefix, p.reveal = "", true
	if armed {
		p.feedback.ShowWithStyle(feedback.Info, "Drag armed", feedback.Immediate)
	}
	v.focusInteraction(gtx)
}
