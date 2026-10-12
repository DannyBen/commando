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
	duplicate bool                   // Direct duplication does not read or change the clipboard.
	snapshot  *browser.FileClipboard // Captured selection; bypasses system clipboard reads and writes.
	owner     uintptr
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
	if v.activeEditor() != nil || v.loader == nil || p.loading || p.drives || p.path == "" || !v.actionAllowed(actionClipboard) {
		return
	}
	paths := p.selectedPaths()
	if len(paths) == 0 {
		return
	}
	p.loading, p.prefix = true, ""
	v.loader.submit(loadRequest{panel: v.active, path: p.path, action: actionClipboard, clipboard: &fileClipboardRequest{paths: paths, move: move, locations: v.fileOperationLocations()}})
}

func (v *view) pasteFiles() {
	p := &v.panels[v.active]
	if v.activeEditor() != nil || v.loader == nil || p.loading || p.drives || p.path == "" || !v.actionAllowed(actionMutation) {
		return
	}
	p.loading, p.prefix, v.transferPending = true, "", true
	// Windows owns the transfer UI, including progress and cancellation.
	v.loader.submit(loadRequest{panel: v.active, path: p.path, transfer: &transferRequest{locations: v.fileOperationLocations(), owner: v.ownerWindow}})
}
