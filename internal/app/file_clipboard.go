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
	p := &v.panels[v.active]
	names := p.selectedNames()
	paths := p.selectedPaths()
	if p.archiveFolder != "" {
		paths = make([]string, len(names))
		for i, name := range names {
			paths[i] = p.location().Child(name).String()
		}
	}
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(strings.Join(paths, "\n")))})
	message := "Path copied"
	if len(paths) > 1 {
		message = fmt.Sprintf("%d paths copied", len(paths))
	}
	v.showFeedback(v.active, feedback.Success, message)
}

type fileClipboardRequest struct {
	archive   *archiveSelection
	paths     []string
	move      bool
	locations []string
}

type archiveSelection struct {
	location  browser.Location
	names     []string
	reference *browser.ArchiveClipboard
}

type transferRequest struct {
	target           *panelTransferTarget
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
	if !v.interaction.idle() || move && !clipboardReady(v) || !move && !copyReady(v) {
		return
	}
	paths := p.selectedPaths()
	var archive *archiveSelection
	if p.archiveFolder != "" {
		archive = &archiveSelection{location: p.location(), names: p.selectedNames()}
	}
	locations := v.fileOperationLocations()
	for i := range v.panels {
		if v.panels[i].drives {
			locations[i] = v.favoritesDirectory()
		}
	}
	if archive != nil {
		for i, panel := range v.panels {
			if !panel.drives {
				locations[i] = panel.path
			}
		}
	}
	p.loading, p.prefix = true, ""
	if !v.loader.submit(loadRequest{panel: v.active, path: v.entryDirectory(v.active), action: actionClipboard, clipboard: &fileClipboardRequest{paths: paths, archive: archive, move: move, locations: locations}}) {
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
	if !v.interaction.idle() || !pasteReady(v) || shortcuts && !p.nativeFolder() {
		return
	}
	p.loading, p.prefix, v.transferPending = true, "", true
	// Capture the open destination, including archive subfolders.
	archiveLocations := v.archiveOperationLocations()
	// Windows owns the native transfer UI, including progress and cancellation.
	if !v.loader.submit(loadRequest{panel: v.active, path: p.path, archiveFolder: p.archiveFolder, transfer: &transferRequest{archiveLocations: archiveLocations, locations: v.fileOperationLocations(), owner: v.ownerWindow, shortcuts: shortcuts}}) {
		p.loading, v.transferPending = false, false
	} else if p.archiveFolder != "" {
		v.operationProgress(v.active, "Updating archive")
	} else if shortcuts {
		v.operationProgress(v.active, "Creating shortcuts")
	}
}

func (selection *archiveSelection) stage() (*browser.ArchiveStage, error) {
	if selection.reference != nil {
		return selection.reference.Stage()
	}
	return browser.StageArchive(selection.location, selection.names)
}
