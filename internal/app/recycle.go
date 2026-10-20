package app

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"gioui.org/layout"

	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

type removalRequest struct {
	archiveFolder    string
	archiveLocations []browser.Location
	names            []string
	permanent        bool
	locations        []string // Snapshot for safely updating other alias listings.
	owner            uintptr
}

func (v *view) removeSelected(gtx layout.Context, permanent bool) {
	p := &v.panels[v.active]
	if !v.interaction.idle() || !removalReady(v) {
		return
	}
	names, hasDirectory := p.removalSelection()
	if len(names) == 0 {
		return
	}
	permanent = permanent || p.archiveFolder != ""
	request := &removalRequest{archiveFolder: p.archiveFolder, names: names, permanent: permanent, locations: make([]string, len(v.panels)), owner: v.ownerWindow}
	for pi := range v.panels {
		if v.panels[pi].nativeFolder() {
			request.locations[pi] = v.panels[pi].path
		}
	}
	if p.archiveFolder != "" {
		request.archiveLocations = v.archiveOperationLocations()
	}
	panel, path := v.active, v.entryDirectory(v.active)
	submit := func(layout.Context) { v.submitRemoval(panel, path, request) }
	guard := v.removalGuard(permanent, len(names), hasDirectory)
	if guard == guardOff {
		submit(gtx)
		return
	}
	action, verb := "Recycle", "Recycle"
	if permanent {
		action, verb = "Delete permanently", "Permanently delete"
	}
	if p.archiveFolder != "" {
		action = "Delete from archive"
	}
	entry := "entry"
	if len(names) != 1 {
		entry = "entries"
	}
	selected := 0
	if guard == guardCancel {
		selected = 1
	}
	v.startConfirmation(gtx, &confirmationSession{panel: panel, message: fmt.Sprintf("%s %d %s?", verb, len(names), entry), acceptLabel: action, selected: selected, accept: submit})
}

func (v *view) submitRemoval(panel int, path string, request *removalRequest) {
	// The confirmation owns the captured sources; browsing never retargets it.
	if !v.interaction.idle() || panel >= len(v.panels) || !samePath(v.entryDirectory(panel), path) || v.panels[panel].archiveFolder != request.archiveFolder || !v.actionAllowed(actionMutation) {
		return
	}
	p := &v.panels[panel]
	p.loading, p.prefix = true, ""
	progress := "Recycling"
	if request.permanent {
		progress = "Deleting"
	}
	v.operationProgress(panel, progress)
	if !v.loader.submit(loadRequest{panel: panel, path: path, archiveFolder: request.archiveFolder, removal: request}) {
		p.loading = false
		p.feedback.Complete()
	}
}

func (v *view) acceptRemoval(result loadResult) {
	p := &v.panels[result.request.panel]
	p.loading = false
	refreshFailed := false
	if result.request.removal.archiveFolder != "" {
		refreshFailed = v.applyArchivePanels(result, result.request.removal.archiveLocations)
	} else {
		for pi := range v.panels {
			other := &v.panels[pi]
			if other.drives {
				if samePath(result.request.path, v.favoritesDirectory()) {
					other.removeEntries(result.removed.Names)
				}
				continue
			}
			matches := samePath(other.path, result.request.path)
			if pi < len(result.sameDirectory) && result.sameDirectory[pi] &&
				pi < len(result.request.removal.locations) && samePath(other.path, result.request.removal.locations[pi]) {
				matches = true
			}
			if matches {
				other.removeEntries(result.removed.Names)
			}
			if pi < len(result.locations) && result.locations[pi] != "" &&
				pi < len(result.request.removal.locations) && samePath(other.path, result.request.removal.locations[pi]) {
				for _, name := range result.removed.Names {
					if _, inside := movedLocationSuffix(filepath.Join(result.removed.Directory, name), result.locations[pi]); inside {
						other.refreshNeeded = true
						break
					}
				}
			}
		}
		refreshFailed = v.applyTransferPanels(result, result.request.removal.locations)
	}
	count := len(result.removed.Names)
	outcome := "recycled"
	if result.request.removal.permanent {
		outcome = "deleted"
	}
	if result.err != nil {
		message := recycleError(result.err)
		if result.request.removal.permanent {
			message = deleteError(result.err)
		}
		if count > 0 {
			message = fmt.Sprintf("%d %s; %s", count, outcome, message)
		}
		v.showFeedback(result.request.panel, feedback.Error, message)
		return
	}
	if refreshFailed {
		v.showFeedback(result.request.panel, feedback.Warning, fmt.Sprintf("%d %s; refresh failed", count, outcome))
		return
	}
	if result.removed.Incomplete {
		v.showFeedback(result.request.panel, feedback.Warning, fmt.Sprintf("%d %s; %d remaining", count, outcome, len(result.request.removal.names)-count))
		return
	}
	message := "Item " + outcome
	if count != 1 {
		message = fmt.Sprintf("%d items %s", count, outcome)
	}
	v.showFeedback(result.request.panel, feedback.Success, message)
}

func (p *panel) removeEntries(names []string) {
	if len(names) == 0 {
		return
	}
	key := func(name string) string {
		if runtime.GOOS == "windows" {
			return strings.ToUpper(name)
		}
		return name
	}
	removed := make(map[string]bool, len(names))
	for _, name := range names {
		removed[key(name)] = true
	}
	p.ensureEntries()
	p.entries = slices.DeleteFunc(p.entries, func(file browser.Entry) bool {
		return !file.Parent && removed[key(file.Name)]
	})
	oldSelected, oldFirst := p.selected, p.list.Position.First
	items := make([]entry, 0, len(p.items))
	marks := make(map[int]bool)
	removedBefore, removedBeforeFirst := 0, 0
	selectedRemoved := false
	for row, item := range p.items {
		remove := !item.file.Parent && removed[key(item.file.Name)]
		if remove {
			if row < oldSelected {
				removedBefore++
			}
			if row < oldFirst {
				removedBeforeFirst++
			}
			if row == oldSelected {
				selectedRemoved = true
			}
			continue
		}
		if p.marks[row] {
			marks[len(items)] = true
		}
		items = append(items, item)
	}
	p.items, p.marks, p.markRange = items, marks, nil
	p.selected = min(oldSelected-removedBefore, len(items)-1)
	p.list.Position.First = max(0, min(oldFirst-removedBeforeFirst, len(items)-1))
	p.reveal = p.reveal || selectedRemoved
	p.rows = make(map[int]*fileRow)
}

func recycleError(err error) string {
	message := "Cannot recycle"
	switch {
	case errors.Is(err, browser.ErrRecycleUnavailable):
		message = "Recycle Bin unavailable"
	case errors.Is(err, browser.ErrRecycleUnconfirmed):
		message = "Cannot confirm recycling"
	case errors.Is(err, fs.ErrPermission):
		message = "Access denied"
	case errors.Is(err, fs.ErrNotExist):
		message = "Item not found"
	case errors.Is(err, browser.ErrDriveMapping), errors.Is(err, browser.ErrDriveMappingCycle), errors.Is(err, browser.ErrPathIdentity):
		message = "Backing path unavailable"
	}
	return message
}

func deleteError(err error) string {
	switch {
	case errors.Is(err, browser.ErrArchiveRecovery):
		return "Archive recovery needed"
	case errors.Is(err, browser.ErrArchiveChanged):
		return "Archive changed; retry"
	case errors.Is(err, browser.ErrInvalidArchive):
		return "Invalid archive"
	case errors.Is(err, errors.ErrUnsupported):
		return "Unsupported archive type"
	case errors.Is(err, browser.ErrDeleteUnavailable):
		return "Deletion unavailable"
	case errors.Is(err, browser.ErrDeleteUnconfirmed):
		return "Cannot confirm deletion"
	}
	message := recycleError(err)
	if message == "Cannot recycle" || message == "Recycle Bin unavailable" || message == "Cannot confirm recycling" {
		return "Cannot delete"
	}
	return message
}
