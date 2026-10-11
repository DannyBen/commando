package app

import (
	"errors"
	"io/fs"
	"slices"
	"sort"

	"gioui.org/widget"
	"github.com/dannyben/commando/internal/browser"
)

type createRequest struct {
	name   string
	folder bool
}

// Add the entry to panels displaying this directory without losing their marks
// or moving another panel's current item.
func (v *view) applyCreatedEntry(source int, directory string, file browser.Entry) {
	size := "—"
	if !file.Directory {
		size = formatSize(file.Bytes)
	}
	for pi := range v.panels {
		p := &v.panels[pi]
		if p.drives || !samePath(p.path, directory) {
			continue
		}
		selectedName := ""
		if p.selected >= 0 && p.selected < len(p.items) {
			selectedName = p.items[p.selected].file.Name
		}
		// A stale listing can already contain the name of an externally removed
		// entry. Replace that cached row rather than inserting a duplicate.
		for row, item := range p.items {
			if samePath(item.file.Name, file.Name) {
				p.items = slices.Delete(p.items, row, row+1)
				p.shiftRows(row, -1)
				break
			}
		}
		row := sort.Search(len(p.items), func(i int) bool {
			return browser.CompareEntries(p.items[i].file, file) >= 0
		})
		p.items = slices.Insert(p.items, row, entry{name: file.Name, size: size, file: file})
		p.shiftRows(row, 1)
		if pi == source || p.selected < 0 || samePath(selectedName, file.Name) {
			p.selected, p.reveal = row, true
		}
		p.markRange = nil
		p.rows = make(map[int]*widget.Clickable)
	}
}

func (p *panel) shiftRows(row, delta int) {
	marks := make(map[int]bool, len(p.marks))
	for index, marked := range p.marks {
		if index < row {
			marks[index] = marked
		} else if delta > 0 || index > row {
			marks[index+delta] = marked
		}
	}
	p.marks = marks
	if p.selected >= row {
		p.selected += delta
	}
}

func createError(err error, noun string) string {
	switch {
	case errors.Is(err, browser.ErrInvalidName):
		return "Invalid " + noun + " name"
	case errors.Is(err, fs.ErrExist):
		return "Name already exists"
	case errors.Is(err, fs.ErrPermission):
		return "Access denied"
	case errors.Is(err, fs.ErrNotExist):
		return "Folder not found"
	default:
		return "Cannot create " + noun
	}
}
