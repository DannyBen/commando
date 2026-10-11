package app

import (
	"path/filepath"
	"runtime"
	"strings"

	"gioui.org/widget"
	"github.com/dannyben/commando/internal/browser"
)

type transferPanelUpdate struct {
	needed    bool
	path      string
	directory browser.Directory
	err       error
}

// Read affected open folders on the directory worker, including descendants of
// partial merges. UI reconciliation retains their surviving marks and current row.
func (l *directoryLoader) refreshTransferPanels(result *loadResult) {
	result.panelUpdates = make([]transferPanelUpdate, len(result.locations))
	for pi, original := range result.locations {
		if original == "" {
			continue
		}
		path := original
		needed := samePath(original, result.transferred.Directory)
		for _, change := range result.transferred.Changes {
			if samePath(original, filepath.Dir(change.Destination)) || change.Moved && samePath(original, filepath.Dir(change.Source)) {
				needed = true
			}
			if change.Moved && change.Directory {
				if relative, ok := movedLocationSuffix(change.Source, original); ok {
					path, needed = filepath.Join(change.Destination, relative), true
				}
			}
		}
		if result.transferMove {
			for _, parent := range result.sourceParents {
				needed = needed || samePath(original, parent)
			}
		}
		if !needed {
			continue
		}
		update := transferPanelUpdate{needed: true}
		if !samePath(original, path) {
			update.path = path
		}
		update.directory, update.err = l.readDirectory(path)
		result.panelUpdates[pi] = update
	}
}

func (p *panel) reconcileTransferListing(directory browser.Directory) {
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
	p.items = make([]entry, len(directory.Entries))
	p.marks = make(map[int]bool)
	p.selected = -1
	if len(p.items) > 0 {
		p.selected = max(0, min(previousRow, len(p.items)-1))
	}
	selectedSurvives := false
	for row, file := range directory.Entries {
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

func transferNameKey(name string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(name)
	}
	return name
}
