package app

import (
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func (l *directoryLoader) archiveCreateOperation(request loadRequest) loadResult {
	result := loadResult{request: request}
	locations := request.create.archiveLocations
	paths := make([]string, len(locations))
	for i, location := range locations {
		paths[i] = location.Path
	}
	result.locations = resolveFileLocations(paths)
	original, err := resolveFileLocation(request.path)
	if err != nil {
		result.err = err
		return result
	}
	result.mutationDirectory = original
	parent, created, err := browser.CreateArchive(browser.Location{Path: original, Folder: request.archiveFolder}, request.create.name, request.create.folder, request.create.data)
	result.created, result.err = created, err
	if err != nil {
		return result
	}
	l.refreshArchivePanels(&result, locations, original)
	if parent.Folder != request.archiveFolder {
		parent.Path = request.path // Preserve the panel's lexical alias.
		result.directory, result.err = browser.ReadArchive(parent)
	}
	return result
}

func (v *view) acceptArchiveCreate(result loadResult, session *nameSession) {
	failed := v.applyArchivePanels(result, result.request.create.archiveLocations)
	p := &v.panels[result.request.panel]
	if result.directory.Path != "" {
		p.reconcileDirectoryListing(result.directory)
	}
	for row, item := range p.items {
		if item.file.Name == result.created.Name {
			p.selected, p.reveal = row, true
			break
		}
	}
	session.finished = true
	if failed || result.err != nil {
		v.showFeedback(result.request.panel, feedback.Warning, "Created; refresh failed")
	} else {
		message := "File created"
		if result.created.Directory {
			message = "Folder created"
		}
		v.showFeedback(result.request.panel, feedback.Success, message)
	}
}
