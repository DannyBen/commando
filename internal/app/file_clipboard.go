package app

import (
	"fmt"
	"io"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func (v *view) copySelectedPaths(gtx layout.Context) {
	if !v.interaction.idle() || !textClipboardReady(v) {
		return
	}
	paths := v.panels[v.active].selectedPaths()
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(strings.Join(paths, "\n")))})
	message := "Path copied"
	if len(paths) > 1 {
		message = fmt.Sprintf("%d paths copied", len(paths))
	}
	v.showFeedback(v.active, feedback.Success, message)
}

type fileClipboardRequest struct {
	paths     []string
	move      bool
	locations []string
}

type archiveSelection struct {
	location browser.Location
	names    []string
}

type transferRequest struct {
	archiveLocations []browser.Location
	archive          *archiveSelection
	locations        []string
	duplicate        bool                   // Direct duplication does not read or change the clipboard.
	shortcuts        bool                   // Create shell links instead of transferring clipboard sources.
	snapshot         *browser.FileClipboard // Captured selection; bypasses system clipboard reads and writes.
	owner            uintptr
}

func (v *view) fileOperationLocations() []string {
	paths := make([]string, len(v.panels))
	for i := range v.panels {
		if v.panels[i].nativeFolder() {
			paths[i] = v.panels[i].path
		}
	}
	return paths
}

func (v *view) clipboardSelected(move bool) {
	p := &v.panels[v.active]
	if !v.interaction.idle() || v.loader == nil || p.loading || !p.nativeFolder() || p.path == "" || !v.actionAllowed(actionClipboard) {
		return
	}
	paths := p.selectedPaths()
	if len(paths) == 0 {
		return
	}
	p.loading, p.prefix = true, ""
	if !v.loader.submit(loadRequest{panel: v.active, path: p.path, action: actionClipboard, clipboard: &fileClipboardRequest{paths: paths, move: move, locations: v.fileOperationLocations()}}) {
		p.loading = false
	}
}

func (v *view) pasteFiles() {
	v.pasteClipboard(false)
}

func (v *view) pasteShortcuts() {
	v.pasteClipboard(true)
}

func (v *view) pasteClipboard(shortcuts bool) {
	p := &v.panels[v.active]
	if !v.interaction.idle() || v.loader == nil || p.loading || !p.nativeFolder() || p.path == "" || !v.actionAllowed(actionMutation) {
		return
	}
	p.loading, p.prefix, v.transferPending = true, "", true
	// Windows owns the transfer UI, including progress and cancellation.
	if !v.loader.submit(loadRequest{panel: v.active, path: p.path, transfer: &transferRequest{locations: v.fileOperationLocations(), owner: v.ownerWindow, shortcuts: shortcuts}}) {
		p.loading, v.transferPending = false, false
	} else if shortcuts {
		v.operationProgress(v.active, "Creating shortcuts")
	}
}
