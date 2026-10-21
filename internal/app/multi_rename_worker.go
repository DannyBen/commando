package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

type multiRenameRequest struct {
	session *multiRenameSession
	entries []browser.Entry
	changes []renameChange
}

func (l *directoryLoader) multiRenameOperation(request loadRequest) loadResult {
	result := loadResult{request: request, locations: resolveFileLocations(request.locations)}
	parent, err := resolveFileLocation(request.path)
	result.mutationDirectory = parent
	if err == nil {
		err = l.runMultiRename(&result)
	}
	result.err = err
	// Reconcile even failed preflights and partial batches. No rollback or retry
	// of already successful entries; later expressions start from actual names.
	result.panelUpdates = make([]transferPanelUpdate, len(request.locations))
	for pi, path := range request.locations {
		if path == "" || pi >= len(result.locations) || result.locations[pi] == "" {
			continue
		}
		for _, change := range result.renamed {
			oldPath := filepath.Join(parent, change.oldName)
			if suffix, ok := movedLocationSuffix(oldPath, result.locations[pi]); ok {
				if lexical, ok := movedLocationSuffix(filepath.Join(request.path, change.oldName), path); ok {
					path = filepath.Join(request.path, change.newName, lexical)
				} else {
					path = filepath.Join(parent, change.newName, suffix)
				}
				break
			}
		}
		update := transferPanelUpdate{needed: true}
		update.directory, update.err = l.readExistingParent(path)
		result.panelUpdates[pi] = update
	}
	return result
}

func (l *directoryLoader) runMultiRename(result *loadResult) error {
	request := result.request
	batch := request.multiRename
	parent := result.mutationDirectory
	directory, err := l.readDirectory(parent)
	if err != nil {
		return err
	}
	names := make([]string, len(batch.entries))
	changes := make(map[string]string, len(batch.changes))
	for _, change := range batch.changes {
		changes[change.oldName] = change.newName
	}
	available := make(map[string]browser.Entry, len(directory.Entries))
	for _, entry := range directory.Entries {
		available[entry.Name] = entry
	}
	for i, entry := range batch.entries {
		current, exists := available[entry.Name]
		if !exists || current.Parent || current.Directory != entry.Directory {
			return fs.ErrNotExist
		}
		names[i] = entry.Name
		if name, changed := changes[entry.Name]; changed {
			names[i] = name
		}
	}
	if _, err := validateMultiRename(batch.entries, names, directory.Entries); err != nil {
		return err
	}
	parentInfo, err := os.Stat(parent)
	if err != nil {
		return err
	}
	sources := make([]fs.FileInfo, len(batch.changes))
	for i, change := range batch.changes {
		if browser.ValidateName(change.oldName) != nil || browser.ValidateName(change.newName) != nil {
			return browser.ErrInvalidName
		}
		sources[i], err = os.Lstat(filepath.Join(parent, change.oldName))
		if err != nil {
			return err
		}
	}
	for i, change := range batch.changes {
		// Detect parent alias changes and replaced sources before each mutation.
		currentParent, err := os.Stat(request.path)
		if err != nil {
			return err
		}
		if !os.SameFile(parentInfo, currentParent) {
			return browser.ErrPathIdentity
		}
		current, err := os.Lstat(filepath.Join(parent, change.oldName))
		if err != nil {
			return err
		}
		if !os.SameFile(sources[i], current) {
			return browser.ErrPathIdentity
		}
		if err := l.renameEntry(parent, change.oldName, change.newName); err != nil {
			return err
		}
		result.renamed = append(result.renamed, change)
	}
	return nil
}

func (v *view) acceptMultiRename(result loadResult) {
	eligible := make([]bool, len(v.panels))
	for pi := range v.panels {
		eligible[pi] = v.nameResultApplies(result, pi)
	}
	for _, change := range result.renamed {
		v.applyRename(result, change.oldName, change.newName)
	}
	refreshFailed := false
	for pi, update := range result.panelUpdates {
		if pi >= len(v.panels) || !eligible[pi] || !update.needed {
			continue
		}
		if update.err != nil {
			v.panels[pi].refreshNeeded = true
			refreshFailed = true
		} else {
			v.panels[pi].reconcileDirectoryListing(update.directory)
		}
	}
	s := result.request.multiRename.session
	if s == nil || v.multiRenameInteraction() != s {
		return
	}
	s.pending, s.editor.ReadOnly = false, false
	if len(result.renamed) > 0 || result.err == nil {
		v.finishInteraction(s)
		v.interaction.focusPending = true
	} else {
		v.previewMultiRename(s)
		v.interaction.focusPending = true
	}
	if result.err != nil {
		message := multiRenameError(result.err)
		if len(result.renamed) == 0 {
			s.message = message
			return
		}
		if len(result.renamed) > 0 {
			message = fmt.Sprintf("%d renamed; %s", len(result.renamed), message)
		}
		v.showFeedback(s.panel, feedback.Error, message)
	} else if refreshFailed {
		v.showFeedback(s.panel, feedback.Warning, "Renamed; refresh failed")
	} else if len(result.renamed) > 0 {
		v.showFeedback(s.panel, feedback.Success, fmt.Sprintf("%d renamed", len(result.renamed)))
	}
}
