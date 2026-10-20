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
	if !v.interaction.idle() || !panelSourceReady(v, move) {
		return
	}
	snapshot, archive := p.panelTransferSelection(move)
	if len(snapshot.Paths) == 0 && archive == nil {
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
	v.choosePanelDestination(source, target, source, []string{verb + " here"}, func(target int) string {
		return v.panelTransferError(target, snapshot.Move)
	}, func(target, _ int) { v.submitPanelTransfer(target, target, snapshot, archive) })
}

// The same destination floater serves transfers and archive creation. Excluding
// -1 includes every panel, allowing Pack to target its source or a single panel.
func (v *view) choosePanelDestination(source, target, excluded int, options []string, check func(int) string, choose func(int, int)) {
	selected := 0
	var offer func(int)
	offer = func(target int) {
		p := &v.panels[target]
		v.startInlineActions(target, p.selected+1, "", "", options, func(choice int) {
			choose(target, choice)
		}, func() { v.active = source })
		v.inlineInteraction().selected = selected
		v.inlineInteraction().canChoose = func() bool {
			message := check(target)
			if message != "" {
				v.showFeedback(target, feedback.Info, message)
				return false
			}
			return true
		}
		v.inlineInteraction().navigate = func(direction int, cycle bool) {
			next := v.panelNeighbor(direction, cycle, excluded)
			if next == target {
				return
			}
			selected = v.inlineInteraction().selected
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
	if !v.interaction.idle() || !panelTransferReady(v, direction, move) {
		return
	}
	source := v.active
	p := &v.panels[source]
	snapshot, archive := p.panelTransferSelection(move)
	p.prefix = ""
	v.submitPanelTransfer(source+direction, source, snapshot, archive)
}

func panelTransferReady(v *view, direction int, move bool) bool {
	target := v.active + direction
	return target >= 0 && target < len(v.panels) && panelSourceReady(v, move) && v.panelTransferError(target, move) == ""
}

func (v *view) panelTransferError(target int, move bool) string {
	p := &v.panels[target]
	switch {
	case p.archiveFolder != "" && !p.location().Capabilities().Add:
		return "Cannot transfer into archive"
	case p.drives || p.path == "":
		return "Select a folder"
	case p.loading:
		return "Folder is busy"
	case !v.actionAllowed(actionMutation):
		return "File operation in progress"
	}
	return ""
}

func (v *view) submitPanelTransfer(target, resultPanel int, snapshot browser.FileClipboard, archive *archiveSelection) {
	if message := v.panelTransferError(target, snapshot.Move); message != "" {
		v.showFeedback(target, feedback.Info, message)
		return
	}
	destination := v.panels[target].location()
	var archiveLocations []browser.Location
	if destination.InArchive() || archive != nil {
		archiveLocations = v.archiveOperationLocations()
	}
	p := &v.panels[resultPanel]
	p.loading, p.prefix, v.transferPending = true, "", true
	if !v.loader.submit(loadRequest{panel: resultPanel, path: destination.Path, archiveFolder: destination.Folder, transfer: &transferRequest{
		locations: v.fileOperationLocations(), snapshot: &snapshot, archive: archive, archiveLocations: archiveLocations, owner: v.ownerWindow}}) {
		p.loading, v.transferPending = false, false
		v.showFeedback(resultPanel, feedback.Info, "File operation in progress")
	} else if destination.InArchive() || archive != nil {
		v.operationProgress(resultPanel, "Updating archive")
	} else if archive != nil {
		v.operationProgress(resultPanel, "Extracting archive")
	}
}

func panelSourceReady(v *view, move bool) bool {
	p := &v.panels[v.active]
	if !panelReady(v) || v.loader == nil || !v.actionAllowed(actionMutation) || len(p.selectedNames()) == 0 {
		return false
	}
	if p.nativeFolder() || p.drives {
		return true
	}
	return p.location().Capabilities().Read && (!move || p.location().Capabilities().Delete)
}

func (p *panel) panelTransferSelection(move bool) (browser.FileClipboard, *archiveSelection) {
	if p.archiveFolder != "" {
		return browser.FileClipboard{Move: move}, &archiveSelection{location: p.location(), names: p.selectedNames()}
	}
	return browser.FileClipboard{Paths: p.selectedPaths(), Move: move}, nil
}
