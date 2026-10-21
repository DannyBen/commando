package app

import (
	"image"
	"time"

	"gioui.org/layout"
	"github.com/dannyben/commando/internal/windowshell"
)

func (v *view) startBackgroundContextMenu(gtx layout.Context, index int) {
	if v.shellExecutor == nil || !v.canStartInteraction(index, actionMutation) {
		return
	}
	p := &v.panels[index]
	if p.path == "" || !p.nativeFolder() {
		return
	}
	s := &nativeSession{panel: index, request: shellRequest{
		action: shellMenu, paths: []string{p.path},
		options: windowshell.MenuOptions{Background: true}, queued: time.Now(),
	}}
	if !v.startInteraction(s) {
		return
	}
	v.dismissFeedback()
	v.focusPanel(index, false)
	p.prefix = ""
	v.focusInteraction(gtx)
}

// Mouse, keyboard and command search join here. Right-clicking a marked item
// uses the shared marked selection; an unmarked item is targeted alone.
func (v *view) startContextMenu(gtx layout.Context, index, row int, mouse bool) {
	v.startItemShellAction(gtx, index, row, mouse, shellMenu)
}

func (v *view) startItemShellAction(gtx layout.Context, index, row int, mouse bool, action shellAction) {
	if !v.canStartInteraction(index, actionMutation) || !v.nativeItemsAvailable(index, row, mouse, action) {
		return
	}
	p := &v.panels[index]
	paths := p.selectedPaths()
	if p.drives && !mouse && len(paths) == 0 {
		paths = []string{p.items[row].file.Path}
	} else if mouse && !p.marks[row] {
		paths = []string{p.entryPath(row)}
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
