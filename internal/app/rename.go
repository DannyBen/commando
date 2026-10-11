package app

import (
	"errors"
	"image/color"
	"io/fs"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
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

type renameRequest struct{ oldName, newName string }

type renameSession struct {
	panel, row        int
	oldName           string
	pending, finished bool
	focused           bool
	err               error
}

func (v *view) startRename(gtx layout.Context) {
	p := &v.panels[v.active]
	if p.loading || !p.markable(p.selected) {
		return
	}
	item := p.items[p.selected].file
	if item.Name == "" {
		return
	}
	p.prefix, p.markRange = "", nil
	v.renaming = &renameSession{panel: v.active, row: p.selected, oldName: item.Name}
	v.renameEditor.SetText(item.Name)
	v.renameEditor.ReadOnly = false
	end := len(item.Name)
	if !item.Directory {
		if ext := filepath.Ext(item.Name); ext != item.Name {
			end -= len(ext)
		}
	}
	v.renameEditor.SetCaret(0, utf8.RuneCountInString(item.Name[:end]))
	p.reveal = true
	gtx.Execute(key.FocusCmd{Tag: &v.renameEditor})
}

func (v *view) stopRename(gtx layout.Context) {
	v.renaming = nil
	v.renameEditor.ReadOnly = false
	gtx.Execute(key.FocusCmd{Tag: &v.focus})
}

func (v *view) updateRename(gtx layout.Context) {
	r := v.renaming
	if r.finished {
		v.stopRename(gtx)
		return
	}
	// Scrolling an editor out of the viewport removes its focus target. Cancel
	// the draft once focus is lost, so browsing never has an invisible editor.
	if gtx.Focused(&v.renameEditor) {
		r.focused = true
	} else if r.focused && !r.pending {
		v.stopRename(gtx)
		return
	}
	for {
		e, ok := gtx.Event(
			key.Filter{Focus: &v.renameEditor, Name: key.NameEscape},
			key.Filter{Focus: &v.renameEditor, Name: key.NameF2},
		)
		if !ok {
			break
		}
		if ke, ok := e.(key.Event); ok && ke.State == key.Press && !r.pending {
			v.dismissFeedback()
			if ke.Name == key.NameF2 {
				v.renameEditor.SetCaret(0, utf8.RuneCountInString(v.renameEditor.Text()))
			} else {
				v.stopRename(gtx)
				return
			}
		}
	}
	for {
		e, ok := v.renameEditor.Update(gtx)
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
			if e.Text == r.oldName {
				v.stopRename(gtx)
				return
			}
			if err := browser.ValidateName(e.Text); err != nil {
				r.err = err
				v.showFeedback(r.panel, feedback.Error, renameError(err))
				continue
			}
			if v.loader == nil {
				r.err = errors.ErrUnsupported
				v.showFeedback(r.panel, feedback.Error, renameError(r.err))
				continue
			}
			r.err, r.pending, v.renameEditor.ReadOnly = nil, true, true
			v.panels[r.panel].feedback.Progress("Renaming")
			v.loader.requests <- loadRequest{panel: r.panel, path: v.panels[r.panel].path,
				rename: &renameRequest{oldName: r.oldName, newName: e.Text}}
		}
	}
}

func (v *view) acceptRename(result loadResult) {
	r := v.renaming
	if r == nil {
		return
	}
	r.pending, r.err, v.renameEditor.ReadOnly = false, result.err, false
	if result.err != nil {
		v.showFeedback(r.panel, feedback.Error, renameError(result.err))
		return
	}
	request := result.request.rename
	v.applyRename(result.request.path, request.oldName, request.newName)
	v.showFeedback(r.panel, feedback.Success, "Renamed")
	r.finished = true
}

// Update cached listings in place, retaining marks and selection by identity.
// The single filesystem worker orders pending directory reads before/after rename.
func (v *view) applyRename(directory, oldName, newName string) {
	oldPath, newPath := filepath.Join(directory, oldName), filepath.Join(directory, newName)
	for pi := range v.panels {
		p := &v.panels[pi]
		if p.drives {
			continue
		}
		if samePath(p.path, directory) {
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
		} else if samePath(p.path, oldPath) {
			p.path = newPath
		} else if len(p.path) > len(oldPath) && samePath(p.path[:len(oldPath)], oldPath) && p.path[len(oldPath)] == filepath.Separator {
			p.path = newPath + p.path[len(oldPath):]
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

func (v *view) renameContents(gtx layout.Context, item entry, metaColor color.NRGBA, mode metadataColumn) layout.Dimensions {
	padding := max(0, v.appearance.RowPaddingHorizontal)
	return layout.Inset{Left: padding, Right: padding}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			return v.columnLayout(gtx, v.metadataWidth(gtx, mode), func(gtx layout.Context) layout.Dimensions {
				return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					paint.FillShape(gtx.Ops, v.appearance.Colors.Surface, clip.Rect{Max: gtx.Constraints.Min}.Op())
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}, func(gtx layout.Context) layout.Dimensions {
					return v.textEditor(&v.renameEditor, "").Layout(gtx)
				})
			}, func(gtx layout.Context) layout.Dimensions {
				return v.label(gtx, mode.value(item), v.appearance.TextSize, metaColor, text.End)
			})
		})
	})
}
