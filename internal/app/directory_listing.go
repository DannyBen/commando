package app

import (
	"path/filepath"
	"slices"

	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/dannyben/commando/internal/browser"
)

// Reconcile refreshed entries by identity only while the location stays the
// same. A recovered ancestor is a new listing, even when names happen to match.
func (p *panel) reconcileDirectoryListing(directory browser.Directory) {
	if p.drives != directory.Drives || !directory.Drives && !samePath(filepath.Clean(p.path), filepath.Clean(directory.Path)) {
		p.clearMarks()
		p.items, p.entries, p.selected = nil, nil, -1
		p.list = widget.List{List: layout.List{Axis: layout.Vertical}}
		p.refreshNeeded = false
	}
	p.drives = directory.Drives
	if !p.drives {
		p.path = directory.Path
	}
	previousRow, first := p.selected, p.list.Position.First
	selectedName, firstName := "", ""
	if previousRow >= 0 && previousRow < len(p.items) {
		selectedName = p.items[previousRow].file.Name
	}
	if first >= 0 && first < len(p.items) {
		firstName = p.items[first].file.Name
	}
	marked := make(map[string]bool)
	for row := range p.marks {
		if p.markable(row) {
			marked[transferNameKey(p.items[row].file.Name)] = true
		}
	}
	p.entries = slices.Clone(directory.Entries)
	files := p.visibleEntries()
	p.items = make([]entry, len(files))
	p.marks = make(map[int]bool)
	p.selected = -1
	if len(p.items) > 0 {
		p.selected = max(0, min(previousRow, len(p.items)-1))
	}
	p.list.Position.First = max(0, min(first, len(p.items)-1))
	selectedSurvives := false
	for row, file := range files {
		size := "?"
		if file.Directory {
			size = "—"
		} else if file.SizeKnown {
			size = formatSize(file.Bytes)
		}
		p.items[row] = entry{name: file.Name, size: size, file: file}
		if marked[transferNameKey(file.Name)] && !file.Parent {
			p.marks[row] = true
		}
		if samePath(file.Name, selectedName) {
			p.selected, selectedSurvives = row, true
		}
		if samePath(file.Name, firstName) {
			p.list.Position.First = row
		}
	}
	if !selectedSurvives {
		p.reveal = p.selected >= 0
	}
	p.prefix, p.markRange, p.err = "", nil, nil
	p.rows = make(map[int]*widget.Clickable)
}
