package app

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"io"
	"path/filepath"

	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

type unpackRequest struct{ archive, name string }

func unpackReady(v *view) bool {
	if !selectedMutationReady(v) {
		return false
	}
	p := &v.panels[v.active]
	names := p.selectedNames()
	if len(names) != 1 || browser.UnpackExtension(names[0]) == "" {
		return false
	}
	for _, item := range p.items {
		if item.file.Name == names[0] {
			return !item.file.Directory
		}
	}
	return false
}

func (v *view) startUnpack() {
	if !v.interaction.idle() || !unpackReady(v) {
		return
	}
	source := v.active
	p := &v.panels[source]
	archive := p.selectedPaths()[0]
	name := filepath.Base(archive)
	name = name[:len(name)-len(browser.UnpackExtension(name))]
	if browser.ValidateName(name) != nil {
		name = "Archive"
	}
	target := source + 1
	if target >= len(v.panels) {
		target = max(0, source-1)
	}
	p.prefix = ""
	v.choosePanelDestination(source, target, -1, []string{"Extract here", "Extract to folder"}, func(target int) string {
		if !v.panels[target].nativeFolder() {
			return "Select a filesystem folder"
		}
		return v.panelTransferError(target, false)
	}, func(target, choice int) {
		if choice == 1 {
			v.transferPending = true
			v.pendingNameDraft = &loadResult{request: loadRequest{panel: target, fileDraft: &fileDraftRequest{name: name, unpackArchive: archive}}}
		} else {
			request := loadRequest{panel: target, path: v.panels[target].path, locations: v.fileOperationLocations(), unpack: &unpackRequest{archive: archive}}
			if v.loader.submit(request) {
				v.panels[target].loading = true
				v.transferPending = true
				v.operationProgress(target, "Extracting archive")
			}
		}
	})
}

func (l *directoryLoader) unpackOperation(request loadRequest) loadResult {
	result := loadResult{request: request, locations: resolveFileLocations(request.locations)}
	result.mutationDirectory, result.err = resolveFileLocation(request.path)
	if result.err == nil {
		result.created, result.err = browser.UnpackArchive(request.unpack.archive, result.mutationDirectory, request.unpack.name)
		l.refreshTransferPanels(&result)
	}
	return result
}

func (v *view) acceptUnpack(result loadResult) {
	panel := result.request.panel
	p := &v.panels[panel]
	p.loading = false
	p.feedback.Complete()
	v.transferPending = false
	n := v.nameInteraction()
	if n != nil && n.kind == nameUnpack {
		n.pending, n.err, v.nameEditor.ReadOnly = false, result.err, false
		if result.err != nil && result.created.Name == "" {
			n.focused = false
			v.showFeedback(panel, feedback.Error, unpackError(result.err))
			return
		}
		n.finished = true
	}
	refreshFailed := v.applyTransferPanels(result, result.request.locations)
	if result.created.Name != "" {
		v.applyCreatedEntry(result)
	}
	if result.err != nil {
		v.showFeedback(panel, feedback.Error, unpackError(result.err))
	} else if refreshFailed {
		v.showFeedback(panel, feedback.Warning, "Extracted; refresh failed")
	} else {
		v.showFeedback(panel, feedback.Success, "Extracted")
	}
}

func unpackError(err error) string {
	if errors.Is(err, zip.ErrFormat) || errors.Is(err, tar.ErrHeader) || errors.Is(err, gzip.ErrHeader) {
		return "Invalid archive"
	}
	if errors.Is(err, zip.ErrChecksum) || errors.Is(err, gzip.ErrChecksum) {
		return "Archive checksum failed"
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return "Archive is truncated"
	}
	if message := fileTransferError(err); message != "Cannot transfer" {
		return message
	}
	return "Cannot extract archive"
}
