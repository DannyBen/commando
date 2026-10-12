package app

import (
	"errors"
	"fmt"
	"io/fs"
	"runtime"
	"strings"

	"gioui.org/widget"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

type recycleRequest struct {
	names     []string
	locations []string // Snapshot for safely updating other alias listings.
}

func (v *view) recycleSelected() {
	p := &v.panels[v.active]
	if v.activeEditor() != nil || v.loader == nil || p.loading || p.drives || !v.actionAllowed(actionMutation) {
		return
	}
	var names []string
	if len(p.marks) != 0 {
		for row, item := range p.items {
			if p.marks[row] && p.markable(row) {
				names = append(names, item.file.Name)
			}
		}
	} else if p.markable(p.selected) {
		names = append(names, p.items[p.selected].file.Name)
	}
	if len(names) == 0 {
		return
	}
	request := &recycleRequest{names: names, locations: make([]string, len(v.panels))}
	for pi := range v.panels {
		if !v.panels[pi].drives {
			request.locations[pi] = v.panels[pi].path
		}
	}
	p.loading, p.prefix = true, ""
	p.feedback.Progress("Recycling")
	v.loader.submit(loadRequest{panel: v.active, path: p.path, recycle: request})
}

func (v *view) acceptRecycle(result loadResult) {
	p := &v.panels[result.request.panel]
	p.loading = false
	for pi := range v.panels {
		other := &v.panels[pi]
		if other.drives {
			continue
		}
		matches := samePath(other.path, result.request.path)
		if pi < len(result.sameDirectory) && result.sameDirectory[pi] &&
			pi < len(result.request.recycle.locations) && samePath(other.path, result.request.recycle.locations[pi]) {
			matches = true
		}
		if matches {
			other.removeEntries(result.recycled.Names)
		}
	}
	count := len(result.recycled.Names)
	if result.err != nil {
		message := recycleError(result.err)
		if count > 0 {
			message = fmt.Sprintf("%d recycled; %s", count, message)
		}
		v.showFeedback(result.request.panel, feedback.Error, message)
		return
	}
	message := "Item recycled"
	if count != 1 {
		message = fmt.Sprintf("%d items recycled", count)
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
	p.rows = make(map[int]*widget.Clickable)
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
