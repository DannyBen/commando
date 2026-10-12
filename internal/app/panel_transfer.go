package app

import (
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

// Panel transfers capture the source once and carry the shared inline action
// between destination panes, initially choosing the right neighbor or the left
// neighbor at the outer edge. Completion never changes panel focus.
func (v *view) startPanelTransfer(move bool) {
	source := v.active
	p := &v.panels[source]
	if v.activeEditor() != nil || v.actions != nil || v.loader == nil || p.loading || p.drives || p.path == "" || !v.actionAllowed(actionMutation) {
		return
	}
	snapshot := browser.FileClipboard{Paths: p.selectedPaths(), Move: move}
	if len(snapshot.Paths) == 0 {
		return
	}
	target := source + 1
	if target >= len(v.panels) {
		target = source - 1
	}
	if target < 0 {
		return
	}
	p.prefix = ""
	verb := "Copy"
	if move {
		verb = "Move"
	}
	var offer func(int)
	offer = func(target int) {
		p := &v.panels[target]
		v.startInlineActions(target, p.selected+1, "", "", []string{verb + " here"}, func(int) {
			p := &v.panels[target]
			p.loading, p.prefix, v.transferPending = true, "", true
			if !v.loader.submit(loadRequest{panel: target, path: p.path, transfer: &transferRequest{
				locations: v.fileOperationLocations(), snapshot: &snapshot, owner: v.ownerWindow}}) {
				p.loading, v.transferPending = false, false
				v.showFeedback(target, feedback.Info, "File operation in progress")
			}
		}, func() { v.active = source })
		v.actions.canChoose = func() bool {
			p := &v.panels[target]
			message := ""
			switch {
			case p.drives || p.path == "":
				message = "Select a folder"
			case p.loading:
				message = "Folder is busy"
			case !v.actionAllowed(actionMutation):
				message = "File operation in progress"
			}
			if message != "" {
				v.showFeedback(target, feedback.Info, message)
				return false
			}
			return true
		}
		v.actions.navigate = func(direction int, cycle bool) {
			next := v.panelNeighbor(direction, cycle, source)
			if next == target {
				return
			}
			v.stopInlineActions()
			v.active = next
			offer(next)
		}
	}
	v.active = target
	offer(target)
}
