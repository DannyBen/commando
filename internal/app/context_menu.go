package app

import (
	"image"
	"path/filepath"
	"time"

	"gioui.org/layout"
)

// Mouse, keyboard and command search join here. Mouse invocation targets only
// the clicked item; keyboard invocation uses the shared marked selection.
func (v *view) startContextMenu(gtx layout.Context, index, row int, mouse bool) {
	v.startItemShellAction(gtx, index, row, mouse, shellMenu)
}

func (v *view) startItemShellAction(gtx layout.Context, index, row int, mouse bool, action shellAction) {
	if !v.canStartInteraction(index, actionMutation) || !v.nativeItemsAvailable(index, row, mouse) {
		return
	}
	p := &v.panels[index]
	paths := p.selectedPaths()
	if mouse {
		paths = []string{filepath.Join(p.path, p.items[row].file.Name)}
	}
	if len(paths) == 0 {
		return
	}
	s := &nativeSession{panel: index, request: shellRequest{action: action, paths: paths, queued: time.Now()}}
	if !mouse {
		s.request.options.ClientPoint = new(image.Point)
	}
	if !v.startInteraction(s) {
		return
	}
	v.dismissFeedback()
	if mouse {
		v.panels[v.active].prefix = ""
		v.active = index
		p.selected, p.markRange = row, nil
	}
	p.prefix, p.reveal = "", true
	v.focusInteraction(gtx)
}
