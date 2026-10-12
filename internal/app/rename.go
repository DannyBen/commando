package app

import (
	"errors"
	"io/fs"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"gioui.org/widget"
	"github.com/dannyben/commando/internal/browser"
)

type renameRequest struct{ oldName, newName string }

// Update cached listings in place, retaining marks and selection by identity.
// The single filesystem worker orders pending directory reads before/after rename.
func (v *view) applyRename(result loadResult, oldName, newName string) {
	oldPath, newPath := filepath.Join(result.mutationDirectory, oldName), filepath.Join(result.mutationDirectory, newName)
	for pi := range v.panels {
		p := &v.panels[pi]
		if !v.nameResultApplies(result, pi) {
			continue
		}
		if samePath(result.locations[pi], result.mutationDirectory) {
			firstName := ""
			if first := p.list.Position.First; first >= 0 && first < len(p.items) {
				firstName = p.items[first].name
				if firstName == oldName {
					firstName = newName
				}
			}
			selectedName := ""
			if p.selected >= 0 && p.selected < len(p.items) {
				selectedName = p.items[p.selected].name
			}
			marked := make(map[string]bool, len(p.marks))
			for row := range p.marks {
				name := p.items[row].name
				if name == oldName {
					name = newName
				}
				marked[name] = true
			}
			for i := range p.items {
				if p.items[i].file.Name == oldName {
					p.items[i].name, p.items[i].file.Name = newName, newName
				}
			}
			if selectedName == oldName {
				selectedName = newName
			}
			slices.SortFunc(p.items, func(a, b entry) int { return browser.CompareEntries(a.file, b.file) })
			p.marks, p.markRange = make(map[int]bool, len(marked)), nil
			for row, item := range p.items {
				if item.name == firstName {
					p.list.Position.First = row
				}
				if item.name == selectedName {
					p.selected = row
				}
				if marked[item.name] {
					p.marks[row] = true
				}
			}
			p.rows = make(map[int]*widget.Clickable)
			if pi == v.active {
				p.reveal = true
			}
		} else if relative, ok := movedLocationSuffix(oldPath, result.locations[pi]); ok {
			// Retain a usable lexical alias when it follows the renamed child.
			// Aliases bound to the old child itself instead follow its backing path.
			lexicalOld := filepath.Join(result.request.path, oldName)
			if suffix, lexical := movedLocationSuffix(lexicalOld, p.path); lexical {
				p.path = filepath.Join(result.request.path, newName, suffix)
			} else {
				p.path = filepath.Join(newPath, relative)
			}
		}
	}
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func renameError(err error) string {
	switch {
	case errors.Is(err, browser.ErrInvalidName):
		return "Invalid filename"
	case errors.Is(err, fs.ErrExist):
		return "Name already exists"
	case errors.Is(err, fs.ErrPermission):
		return "Access denied"
	case errors.Is(err, fs.ErrNotExist):
		return "Item not found"
	default:
		return "Cannot rename"
	}
}
