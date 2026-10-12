package app

import (
	"github.com/dannyben/commando/internal/browser"
)

type fileClipboardRequest struct {
	paths     []string
	move      bool
	locations []string
}

type transferRequest struct {
	locations []string
	duplicate bool // Direct duplication does not read or change the clipboard.
	snapshot  *browser.FileClipboard
	plan      *browser.TransferPlan
	policy    browser.TransferPolicy
}

func (v *view) fileOperationLocations() []string {
	paths := make([]string, len(v.panels))
	for i := range v.panels {
		if !v.panels[i].drives {
			paths[i] = v.panels[i].path
		}
	}
	return paths
}

func (v *view) clipboardSelected(move bool) {
	p := &v.panels[v.active]
	if v.activeEditor() != nil || v.loader == nil || p.loading || p.drives || p.path == "" || v.transferPending {
		return
	}
	paths := p.selectedPaths()
	if len(paths) == 0 {
		return
	}
	p.loading, p.prefix = true, ""
	v.loader.requests <- loadRequest{panel: v.active, path: p.path, clipboard: &fileClipboardRequest{paths: paths, move: move, locations: v.fileOperationLocations()}}
}

func (v *view) pasteFiles() {
	p := &v.panels[v.active]
	if v.activeEditor() != nil || v.loader == nil || p.loading || p.drives || p.path == "" || v.transferPending || v.recyclingInProgress() {
		return
	}
	p.loading, p.prefix, v.transferPending = true, "", true
	// No transfer-progress UI until its interaction model has been agreed.
	v.loader.requests <- loadRequest{panel: v.active, path: p.path, transfer: &transferRequest{locations: v.fileOperationLocations()}}
}
