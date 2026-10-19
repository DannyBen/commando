package app

import (
	"errors"
	"io/fs"
	"slices"
	"sort"

	"github.com/dannyben/commando/internal/browser"
)

type createRequest struct {
	archiveLocations []browser.Location
	name             string
	folder           bool
	data             []byte
}

// Add the entry to panels displaying this directory without losing their marks
// or moving another panel's current item.
func (v *view) applyCreatedEntry(result loadResult) {
	for pi := range v.panels {
		p := &v.panels[pi]
		if !v.nameResultApplies(result, pi) || !samePath(result.locations[pi], result.mutationDirectory) {
			continue
		}
		p.insertCreatedEntry(result.created, pi == result.request.panel)
	}
}

// Shared by inline creation and clipboard transfers.
func (p *panel) insertCreatedEntry(file browser.Entry, selectCreated bool) {
	p.cacheEntry(file)
	if !p.visibility.shows(file) {
		p.refilter()
		return
	}
	firstName := ""
	if first := p.list.Position.First; first >= 0 && first < len(p.items) {
		firstName = p.items[first].file.Name
	}
	size := "—"
	if !file.Directory {
		size = "?"
		if file.SizeKnown {
			size = formatSize(file.Bytes)
		}
	}
	selectedName := ""
	if p.selected >= 0 && p.selected < len(p.items) {
		selectedName = p.items[p.selected].file.Name
	}
	for row, item := range p.items {
		if samePath(item.file.Name, file.Name) {
			p.items = slices.Delete(p.items, row, row+1)
			p.shiftRows(row, -1)
			break
		}
	}
	row := sort.Search(len(p.items), func(i int) bool { return p.compareEntries(p.items[i].file, file) >= 0 })
	p.items = slices.Insert(p.items, row, entry{name: file.Name, size: size, file: file})
	p.shiftRows(row, 1)
	for index, item := range p.items {
		if item.file.Name == firstName {
			p.list.Position.First = index
			break
		}
	}
	if selectCreated || p.selected < 0 || samePath(selectedName, file.Name) {
		p.selected, p.reveal = row, true
	}
	p.markRange = nil
	p.rows = make(map[int]*fileRow)
}

// Refresh an overwritten file's metadata without treating its cached row as a
// removed entry. Marks, selection and viewport belong to its destination path.
func (p *panel) applyTransferredEntry(item browser.TransferredItem) {
	if item.Replaced {
		p.cacheEntry(item.Entry)
		if !p.visibility.shows(item.Entry) {
			p.refilter()
			return
		}
		for row, existing := range p.items {
			if samePath(existing.file.Name, item.Entry.Name) && existing.file.Directory == item.Entry.Directory {
				size := "—"
				if !item.Entry.Directory {
					size = formatSize(item.Entry.Bytes)
				}
				p.items[row] = entry{name: item.Entry.Name, size: size, file: item.Entry}
				if p.sortByMetadata && p.metadata != metadataNone {
					p.resort()
				}
				return
			}
		}
	}
	p.insertCreatedEntry(item.Entry, false)
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
	case errors.Is(err, browser.ErrArchiveChanged), errors.Is(err, browser.ErrArchiveRecovery), errors.Is(err, browser.ErrInvalidArchive):
		return deleteError(err)
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
