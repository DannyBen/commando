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
	if !v.interaction.idle() || v.loader == nil || p.loading || !p.nativeFolder() || p.path == "" || !v.actionAllowed(actionMutation) {
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
			v.submitPanelTransfer(target, target, snapshot)
		}, func() { v.active = source })
		v.inlineInteraction().canChoose = func() bool {
			message := v.panelTransferError(target)
			if message != "" {
				v.showFeedback(target, feedback.Info, message)
				return false
			}
			return true
		}
		v.inlineInteraction().navigate = func(direction int, cycle bool) {
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

// Direct transfers keep their source as the result context. In particular,
// same-folder copies can use the existing duplicate-name editor there.
func (v *view) transferToNeighbor(direction int, move bool) {
	if !v.interaction.idle() || !panelTransferReady(v, direction) {
		return
	}
	source := v.active
	p := &v.panels[source]
	snapshot := browser.FileClipboard{Paths: p.selectedPaths(), Move: move}
	p.prefix = ""
	v.submitPanelTransfer(source+direction, source, snapshot)
}

func panelTransferReady(v *view, direction int) bool {
	target := v.active + direction
	return target >= 0 && target < len(v.panels) && selectedMutationReady(v) && v.panelTransferError(target) == ""
}

func (v *view) panelTransferError(target int) string {
	p := &v.panels[target]
	switch {
	case p.archiveFolder != "":
		return "Archive is read-only"
	case p.drives || p.path == "":
		return "Select a folder"
	case p.loading:
		return "Folder is busy"
	case !v.actionAllowed(actionMutation):
		return "File operation in progress"
	}
	return ""
}

func (v *view) submitPanelTransfer(target, resultPanel int, snapshot browser.FileClipboard) {
	if message := v.panelTransferError(target); message != "" {
		v.showFeedback(target, feedback.Info, message)
		return
	}
	p := &v.panels[resultPanel]
	p.loading, p.prefix, v.transferPending = true, "", true
	if !v.loader.submit(loadRequest{panel: resultPanel, path: v.panels[target].path, transfer: &transferRequest{
		locations: v.fileOperationLocations(), snapshot: &snapshot, owner: v.ownerWindow}}) {
		p.loading, v.transferPending = false, false
		v.showFeedback(resultPanel, feedback.Info, "File operation in progress")
	}
}
