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
	names     []string
	permanent bool
	locations []string // Snapshot for safely updating other alias listings.
}

func (v *view) removeSelected(gtx layout.Context, permanent bool) {
	p := &v.panels[v.active]
	if !v.interaction.idle() || v.loader == nil || p.loading || p.drives || !v.actionAllowed(actionMutation) {
		return
	}
	var names []string
	hasDirectory := false
	if len(p.marks) != 0 {
		for row, item := range p.items {
			if p.marks[row] && p.markable(row) {
				names = append(names, item.file.Name)
				hasDirectory = hasDirectory || item.file.Directory
			}
		}
	} else if p.markable(p.selected) {
		names = append(names, p.items[p.selected].file.Name)
		hasDirectory = p.items[p.selected].file.Directory
	}
	if len(names) == 0 {
		return
	}
	request := &removalRequest{names: names, permanent: permanent, locations: make([]string, len(v.panels))}
	for pi := range v.panels {
		if !v.panels[pi].drives {
			request.locations[pi] = v.panels[pi].path
		}
	}
	panel, path := v.active, p.path
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
	if !v.interaction.idle() || panel >= len(v.panels) || !samePath(v.panels[panel].path, path) || !v.actionAllowed(actionMutation) {
		return
	}
	p := &v.panels[panel]
	p.loading, p.prefix = true, ""
	progress := "Recycling"
	if request.permanent {
		progress = "Deleting"
	}
	p.feedback.Progress(progress)
	if !v.loader.submit(loadRequest{panel: panel, path: path, removal: request}) {
		p.loading = false
		p.feedback.Complete()
	}
}

func (v *view) acceptRemoval(result loadResult) {
	p := &v.panels[result.request.panel]
	p.loading = false
	for pi := range v.panels {
		other := &v.panels[pi]
		if other.drives {
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
