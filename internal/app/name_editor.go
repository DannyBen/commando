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
)

// One inline file-list editor handles existing names and temporary draft rows.
type nameSession struct {
	kind              nameOperation
	panel, row        int
	path, oldName     string
	pending, finished bool
	focused           bool
	err               error
	previousPosition  layout.Position
	previousReveal    bool
}

func (n *nameSession) draft() bool { return n.kind != nameRename && !n.finished }

func (v *view) startName(gtx layout.Context, kind nameOperation) {
	p := &v.panels[v.active]
	if p.loading || v.activeEditor() != nil || v.recyclingInProgress() {
		return
	}
	r := &nameSession{kind: kind, panel: v.active, path: p.path, row: p.selected,
		previousPosition: p.list.Position, previousReveal: p.reveal}
	selectionEnd := 0
	if kind == nameRename {
		if !p.markable(p.selected) || p.items[p.selected].file.Name == "" {
			return
		}
		item := p.items[p.selected].file
		r.oldName = item.Name
		selectionEnd = len(item.Name)
		if !item.Directory {
			if ext := filepath.Ext(item.Name); ext != item.Name {
				selectionEnd -= len(ext)
			}
		}
		p.markRange = nil
	} else {
		if p.drives || p.path == "" {
			return
		}
		// Keep the draft next to the current row until successful creation.
		r.row = max(0, min(p.selected+1, len(p.items)))
		p.rows = make(map[int]*widget.Clickable)
	}
	p.prefix, p.reveal = "", true
	v.naming = r
	v.nameEditor.SetText(r.oldName)
	v.nameEditor.ReadOnly = false
	v.nameEditor.SetCaret(0, utf8.RuneCountInString(r.oldName[:selectionEnd]))
	gtx.Execute(key.FocusCmd{Tag: &v.nameEditor})
}

func (v *view) stopName(gtx layout.Context) {
	r := v.naming
	if r.draft() {
		p := &v.panels[r.panel]
		p.list.Position, p.reveal = r.previousPosition, r.previousReveal
		p.rows = make(map[int]*widget.Clickable)
	}
	v.naming = nil
	v.nameEditor.ReadOnly = false
	gtx.Execute(key.FocusCmd{Tag: &v.focus})
}

func (v *view) updateName(gtx layout.Context) {
	r := v.naming
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
		gtx.Execute(key.FocusCmd{Tag: &v.nameEditor})
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
				v.stopName(gtx)
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
			request := loadRequest{panel: r.panel, path: r.path}
			progress := "Renaming"
			if r.kind == nameRename {
				request.rename = &renameRequest{oldName: r.oldName, newName: e.Text}
			} else {
				request.create = &createRequest{name: e.Text, folder: r.kind == nameCreateFolder}
				progress = "Creating " + r.noun()
			}
			r.err, r.pending, v.nameEditor.ReadOnly = nil, true, true
			v.panels[r.panel].feedback.Progress(progress)
			v.loader.requests <- request
		}
	}
}

func (v *view) acceptName(result loadResult) {
	r := v.naming
	if r == nil {
		return
	}
	r.pending, r.err, v.nameEditor.ReadOnly = false, result.err, false
	if result.err != nil {
		v.showFeedback(r.panel, feedback.Error, r.errorText(result.err))
		v.panels[r.panel].reveal = true
		r.focused = false
		return
	}
	success := "Renamed"
	if request := result.request.rename; request != nil {
		v.applyRename(result.request.path, request.oldName, request.newName)
	} else {
		v.applyCreatedEntry(result.request.panel, result.request.path, result.created)
		success = "File created"
		if result.created.Directory {
			success = "Folder created"
		}
	}
	r.finished = true
	v.showFeedback(r.panel, feedback.Success, success)
}

func (n *nameSession) noun() string {
	if n.kind == nameCreateFolder {
		return "folder"
	}
	return "file"
}

func (n *nameSession) errorText(err error) string {
	if n.kind == nameRename {
		return renameError(err)
	}
	return createError(err, n.noun())
}

// Convert display rows to stored rows. A draft is visual only: real entries,
// marks, selection and persisted state keep their ordinary indices.
func (v *view) draftRow(panel int) int {
	if n := v.naming; n != nil && n.panel == panel && n.draft() {
		return n.row
	}
	return -1
}

func (v *view) storedRow(panel, row int) int {
	if draft := v.draftRow(panel); draft >= 0 {
		if row == draft {
			return -1
		}
		if row > draft {
			return row - 1
		}
	}
	return row
}

func (v *view) nameContents(gtx layout.Context, metadata string, metaColor color.NRGBA, mode metadataColumn) layout.Dimensions {
	hint := ""
	if v.naming.draft() {
		hint = "File name"
		if v.naming.kind == nameCreateFolder {
			hint = "Folder name"
		}
	}
	padding := max(0, v.appearance.RowPaddingHorizontal)
	return layout.Inset{Left: padding, Right: padding}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			return v.columnLayout(gtx, v.metadataWidth(gtx, mode), func(gtx layout.Context) layout.Dimensions {
				return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					paint.FillShape(gtx.Ops, v.appearance.Colors.Surface, clip.Rect{Max: gtx.Constraints.Min}.Op())
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}, func(gtx layout.Context) layout.Dimensions {
					return v.textEditor(&v.nameEditor, hint).Layout(gtx)
				})
			}, func(gtx layout.Context) layout.Dimensions {
				return v.label(gtx, metadata, v.appearance.TextSize, metaColor, text.End)
			})
		})
	})
}
