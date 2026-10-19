package app

import (
	"path/filepath"
	"slices"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"github.com/dannyben/commando/internal/browser"
)

func (v *view) rememberCutClipboard(snapshot browser.FileClipboard, paths, locations []string) {
	if snapshot.Move && (len(snapshot.Paths) > 0 || snapshot.Archive != nil) {
		v.cutClipboard, v.cutPanelPaths, v.cutLocations = snapshot, paths, locations
	} else {
		v.cutClipboard, v.cutPanelPaths, v.cutLocations = browser.FileClipboard{}, nil, nil
	}
}

func (v *view) updateCutClipboard(gtx layout.Context) {
	if len(v.cutClipboard.Paths) == 0 && v.cutClipboard.Archive == nil {
		return
	}
	if v.clipboardSequence() != v.cutClipboard.Sequence {
		v.rememberCutClipboard(browser.FileClipboard{}, nil, nil)
		return
	}
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(500 * time.Millisecond)})
}

func (v *view) cutEntry(panel, row int) bool {
	p := &v.panels[panel]
	if !p.markable(row) || !v.cutClipboard.Move {
		return false
	}
	if reference := v.cutClipboard.Archive; reference != nil {
		source := p.path
		if panel < len(v.cutPanelPaths) && samePath(p.path, v.cutPanelPaths[panel]) && panel < len(v.cutLocations) && v.cutLocations[panel] != "" {
			source = v.cutLocations[panel]
		}
		return p.archiveFolder == reference.Location.Folder && samePath(source, reference.Location.Path) && slices.Contains(reference.Names, p.items[row].file.Name)
	}
	if !p.nativeFolder() {
		return false
	}
	path := p.path
	if panel < len(v.cutPanelPaths) && samePath(p.path, v.cutPanelPaths[panel]) && panel < len(v.cutLocations) && v.cutLocations[panel] != "" {
		path = v.cutLocations[panel]
	}
	path = filepath.Join(path, p.items[row].file.Name)
	for _, cut := range v.cutClipboard.Paths {
		if samePath(path, cut) {
			return true
		}
	}
	return false
}
