package app

import (
	"errors"
	"image/color"
	"path/filepath"
	"unicode/utf8"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/widget"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

type nameOperation uint8

const (
	nameRename nameOperation = iota
	nameCreateFolder
	nameCreateFile
	nameDuplicate
)

// One inline file-list editor handles existing names and temporary draft rows.
type nameSession struct {
	kind              nameOperation
	panel, row        int
	path, oldName     string
	pending, finished bool
	focused           bool
	err               error
	duplicate         *browser.DuplicatePlan
	data              []byte // Captured contents for a new file.
	folder            bool
	previousPosition  layout.Position
	previousReveal    bool
}

func (n *nameSession) draft() bool { return n.kind != nameRename && !n.finished }

func (v *view) startName(gtx layout.Context, kind nameOperation) {
	p := &v.panels[v.active]
	if p.loading || !v.interaction.idle() || !v.actionAllowed(actionMutation) {
		return
	}
	r := &nameSession{kind: kind, panel: v.active, path: p.path, row: p.selected,
		previousPosition: p.list.Position, previousReveal: p.reveal}
	if kind == nameRename {
		if !p.markable(p.selected) || p.items[p.selected].file.Name == "" {
			return
		}
		item := p.items[p.selected].file
		r.oldName = item.Name
		r.folder = item.Directory
		p.markRange = nil
	} else {
		if p.drives || p.path == "" {
			return
		}
		// Keep the draft next to the current row until successful creation.
		r.row = max(0, min(p.selected+1, len(p.items)))
	}
	v.beginName(gtx, r, r.oldName, r.folder)
}

func (v *view) beginName(gtx layout.Context, r *nameSession, name string, folder bool) {
	if !v.interaction.begin(r) {
		return
	}
	p := &v.panels[r.panel]
	if r.draft() {
		p.rows = make(map[int]*fileRow)
	}
	p.prefix, p.reveal = "", true
	v.nameEditor.SetText(name)
	v.nameEditor.ReadOnly = false
	selectionEnd := len(name)
	if !folder {
		if ext := filepath.Ext(name); ext != name {
			selectionEnd -= len(ext)
		}
	}
	v.nameEditor.SetCaret(0, utf8.RuneCountInString(name[:selectionEnd]))
	v.focusInteraction(gtx)
}

func (v *view) stopName(gtx layout.Context) {
	r := v.nameInteraction()
	if r == nil || r.pending {
		return
	}
	if r.draft() {
		p := &v.panels[r.panel]
		p.list.Position, p.reveal = r.previousPosition, r.previousReveal
		p.rows = make(map[int]*fileRow)
	}
	v.interaction.finish(r)
	if r.kind == nameDuplicate {
		v.transferPending = false
		if !r.finished {
			v.showFeedback(r.panel, feedback.Info, "Copy cancelled")
		}
	}
	v.nameEditor.ReadOnly = false
	v.focusInteraction(gtx)
}

func (v *view) updateName(gtx layout.Context) {
	r := v.nameInteraction()
	if r.finished {
		v.stopName(gtx)
		return
	}
	if gtx.Focused(&v.nameEditor) {
		r.focused = true
	} else if r.focused && !r.pending {
		// As with rename, never leave an invisible draft after scrolling away.
		v.stopName(gtx)
		return
	} else if !r.focused {
		v.focusInteraction(gtx)
	}
	for {
		e, ok := gtx.Event(
			key.Filter{Focus: &v.nameEditor, Name: key.NameEscape},
			key.Filter{Focus: &v.nameEditor, Name: key.NameF2},
		)
		if !ok {
			break
		}
		if ke, ok := e.(key.Event); ok && ke.State == key.Press && !r.pending {
			v.dismissFeedback()
			if ke.Name == key.NameF2 {
				v.nameEditor.SetCaret(0, utf8.RuneCountInString(v.nameEditor.Text()))
			} else {
				v.escape(gtx)
				return
			}
		}
	}
	for {
		e, ok := v.nameEditor.Update(gtx)
		if !ok {
			return
		}
		switch e := e.(type) {
		case widget.ChangeEvent:
			v.dismissFeedback()
			r.err = nil
		case widget.SubmitEvent:
			v.dismissFeedback()
			if r.pending {
				continue
			}
			if r.kind == nameRename && e.Text == r.oldName {
				v.stopName(gtx)
				return
			}
			if err := browser.ValidateName(e.Text); err != nil {
				r.err = err
				v.showFeedback(r.panel, feedback.Error, r.errorText(err))
				continue
			}
			if v.loader == nil {
				r.err = errors.ErrUnsupported
				v.showFeedback(r.panel, feedback.Error, r.errorText(r.err))
				continue
			}
			request := loadRequest{panel: r.panel, path: r.path, locations: v.fileOperationLocations()}
			progress := "Renaming"
			if r.kind == nameRename {
				request.rename = &renameRequest{oldName: r.oldName, newName: e.Text}
			} else if r.kind == nameDuplicate {
				request.duplicate = &duplicateRequest{plan: r.duplicate, name: e.Text, locations: v.fileOperationLocations(), owner: v.ownerWindow}
				progress = "Copying"
			} else {
				request.create = &createRequest{name: e.Text, folder: r.kind == nameCreateFolder, data: r.data}
				progress = "Creating " + r.noun()
			}
			r.err, r.pending, v.nameEditor.ReadOnly = nil, true, true
			v.panels[r.panel].feedback.Progress(progress)
			if !v.loader.submit(request) {
				r.pending, v.nameEditor.ReadOnly = false, false
				v.panels[r.panel].loading = false
				v.showFeedback(r.panel, feedback.Info, "Folder is busy")
			}
		}
	}
}

func (v *view) acceptName(result loadResult) {
	r := v.nameInteraction()
	if r == nil {
		return
	}
	r.pending, r.err, v.nameEditor.ReadOnly = false, result.err, false
	if result.request.duplicate != nil && result.transferred.Attempted {
		refreshFailed := v.applyTransferPanels(result, result.request.duplicate.locations)
		r.finished = true
		if result.created.Name != "" {
			v.applyDuplicate(result)
		}
		v.showTransferResult(result, refreshFailed)
		return
	}
	if result.err != nil && result.created.Name == "" {
		v.showFeedback(r.panel, feedback.Error, r.errorText(result.err))
		v.panels[r.panel].reveal = true
		r.focused = false
		return
	}
	success := "Renamed"
	if request := result.request.rename; request != nil {
		v.applyRename(result, request.oldName, request.newName)
	} else if result.request.duplicate != nil {
		v.applyDuplicate(result)
		success = "Copied"
	} else {
		v.applyCreatedEntry(result)
		success = "File created"
		if result.created.Directory {
			success = "Folder created"
		}
	}
	r.finished = true
	if result.err != nil {
		v.showFeedback(r.panel, feedback.Warning, "Copied; cleanup failed")
	} else {
		v.showFeedback(r.panel, feedback.Success, success)
	}
}

func (n *nameSession) noun() string {
	if n.kind == nameCreateFolder || n.kind == nameDuplicate && n.folder {
		return "folder"
	}
	return "file"
}

func (n *nameSession) errorText(err error) string {
	if n.kind == nameRename {
		return renameError(err)
	}
	if n.kind == nameDuplicate {
		return fileTransferError(err)
	}
	return createError(err, n.noun())
}

// Convert display rows to stored rows. A draft is visual only: real entries,
// marks, selection and persisted state keep their ordinary indices.
func (v *view) draftRow(panel int) int {
	if n := v.nameInteraction(); n != nil && n.panel == panel && n.draft() {
		return n.row
	}
	return -1
}

func (v *view) storedRow(panel, row int) int {
	if start, count := v.transientRows(panel); start >= 0 {
		if row >= start && row < start+count {
			return -1
		}
		if row >= start+count {
			return row - count
		}
	}
	return row
}

func (v *view) nameContents(gtx layout.Context, metadata string, metaColor color.NRGBA) layout.Dimensions {
	hint := ""
	if v.nameInteraction().draft() {
		hint = "File name"
		if v.nameInteraction().noun() == "folder" {
			hint = "Folder name"
		}
	}
	padding := max(0, v.appearance.RowPaddingHorizontal)
	return layout.Inset{Left: padding, Right: padding}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			var trailing layout.Widget
			if metadata != "" {
				trailing = func(gtx layout.Context) layout.Dimensions {
					return v.label(gtx, metadata, v.appearance.TextSize, metaColor, text.End)
				}
			}
			return v.trailingLayout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					paint.FillShape(gtx.Ops, v.appearance.Colors.Surface, clip.Rect{Max: gtx.Constraints.Min}.Op())
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}, func(gtx layout.Context) layout.Dimensions {
					return v.textEditor(&v.nameEditor, hint).Layout(gtx)
				})
			}, trailing)
		})
	})
}
