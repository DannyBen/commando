package app

import (
	"path/filepath"

	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

type duplicateRequest struct {
	plan      *browser.TransferPlan
	name      string
	locations []string
}

func (p *panel) selectedPaths() []string {
	var paths []string
	if len(p.marks) > 0 {
		for row, item := range p.items {
			if p.marks[row] && p.markable(row) {
				paths = append(paths, filepath.Join(p.path, item.file.Name))
			}
		}
	} else if p.markable(p.selected) {
		paths = append(paths, filepath.Join(p.path, p.items[p.selected].file.Name))
	}
	return paths
}

func (v *view) duplicateSelected() {
	p := &v.panels[v.active]
	if v.activeEditor() != nil || v.loader == nil || p.loading || p.drives || p.path == "" || v.transferPending || v.recyclingInProgress() {
		return
	}
	paths := p.selectedPaths()
	if len(paths) == 0 {
		return
	}
	p.loading, p.prefix, v.transferPending = true, "", true
	snapshot := browser.FileClipboard{Paths: paths}
	v.loader.requests <- loadRequest{panel: v.active, path: p.path, transfer: &transferRequest{
		locations: v.fileOperationLocations(), snapshot: &snapshot, duplicate: true}}
}

func (v *view) offerDuplicate(gtx layout.Context) {
	if v.pendingDuplicate == nil {
		return
	}
	result := v.pendingDuplicate
	panel := result.request.panel
	if panel != v.active {
		v.pendingDuplicate, v.transferPending = nil, false
		v.showFeedback(panel, feedback.Info, "Copy cancelled")
		return
	}
	if v.activeEditor() != nil || v.actions != nil {
		return
	}
	v.pendingDuplicate = nil
	p := &v.panels[panel]
	source, name := result.duplicatePlan.DuplicateSubject()
	// Anchor to the copied source even when a different row is highlighted.
	first := p.list.Position.First
	inserted := p.applyTransferredEntry(browser.TransferredItem{Entry: source, Replaced: true})
	row := p.selected + 1
	for index, item := range p.items {
		if samePath(item.file.Name, source.Name) {
			row = index + 1
			if inserted && first > 0 && index <= first {
				p.list.Position.First++
			}
			break
		}
	}
	r := &nameSession{kind: nameDuplicate, panel: panel, row: row, path: p.path, folder: source.Directory,
		duplicate: result.duplicatePlan, previousPosition: p.list.Position, previousReveal: p.reveal}
	v.beginName(gtx, r, name, source.Directory)
}

func (l *directoryLoader) duplicateOperation(request loadRequest) loadResult {
	r := request.duplicate
	result := loadResult{request: request}
	result.transferred, result.err = browser.DuplicateWithName(r.plan, r.name)
	if len(result.transferred.Items) > 0 {
		result.created = result.transferred.Items[0].Entry
		result.sameDirectory = make([]bool, len(r.locations))
		for pi, path := range r.locations {
			if path == "" {
				continue
			}
			resolved, err := resolveFileLocation(path)
			result.sameDirectory[pi] = err == nil && samePath(resolved, result.transferred.Directory)
		}
	}
	return result
}

func (v *view) applyDuplicate(result loadResult) {
	for pi := range v.panels {
		p := &v.panels[pi]
		locations := result.request.duplicate.locations
		if p.drives || pi >= len(locations) || !samePath(p.path, locations[pi]) || pi >= len(result.sameDirectory) || !result.sameDirectory[pi] {
			continue
		}
		first := p.list.Position.First
		p.insertCreatedEntry(result.created, pi == result.request.panel)
		if pi != result.request.panel && first > 0 {
			for row, item := range p.items {
				if item.file.Name == result.created.Name && row <= first {
					p.list.Position.First++
					break
				}
			}
		}
	}
}
