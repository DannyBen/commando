package app

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"unicode/utf8"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/renameexpr"
)

type multiRenameSession struct {
	panel      int
	path       string
	entries    []browser.Entry // Captured in panel order, independent of marks and sorting.
	editor     widget.Editor
	surface    dockedSurface
	actions    actionBar
	focus      int // Expression, Apply, Cancel.
	pending    bool
	preview    map[string]string
	spans      map[string][]renameSpan
	expression string // Last validated text; validation follows editor contents, not key kinds.
	invalid    map[string]bool
	changes    []renameChange
	message    string
}

type renameChange struct{ oldName, newName string }

func (s *multiRenameSession) interactionPanel() int        { return s.panel }
func (*multiRenameSession) openingEvent() interactionEvent { return eventWidgetOpened }
func (*multiRenameSession) blocksMutations() bool          { return true }
func (s *multiRenameSession) holdsOperations() bool        { return s.pending }
func (s *multiRenameSession) blocksPanels() bool           { return s.pending }

func (v *view) multiRenameInteraction() *multiRenameSession {
	s, _ := v.interaction.current.(*multiRenameSession)
	return s
}

func renameReady(v *view) bool {
	p := &v.panels[v.active]
	if !selectedReady(v) || !v.actionAllowed(actionMutation) {
		return false
	}
	if len(p.selectedNames()) > 1 && p.nativeFolder() {
		return true
	}
	return (p.favoriteSelected() || !p.drives && p.location().Capabilities().Rename) && p.markable(p.selected)
}

func (v *view) startMultiRename(gtx layout.Context) {
	p := &v.panels[v.active]
	if !v.canStartInteraction(v.active, actionMutation) || !p.nativeFolder() {
		return
	}
	s := &multiRenameSession{panel: v.active, path: p.path,
		editor: widget.Editor{SingleLine: true, Submit: true}}
	for row, item := range p.items {
		if p.marks[row] && p.markable(row) {
			s.entries = append(s.entries, item.file)
		}
	}
	if len(s.entries) < 2 || !v.startInteraction(s) {
		return
	}
	p.ensureEntries()
	p.prefix, p.markRange = "", nil
	s.editor.SetText("<name>")
	s.editor.SetCaret(0, utf8.RuneCountInString(s.editor.Text()))
	v.previewMultiRename(s)
	v.focusInteraction(gtx)
}

// Collisions use Windows name semantics on development hosts too. Occupied
// targets remain conflicts even when another selected entry would vacate them.
func validateMultiRename(entries []browser.Entry, names []string, all []browser.Entry) (map[string]bool, error) {
	invalid := make(map[string]bool)
	occupied := make(map[string][]string)
	for _, entry := range all {
		if !entry.Parent {
			key := strings.ToUpper(entry.Name)
			occupied[key] = append(occupied[key], entry.Name)
		}
	}
	var err error
	targets := make(map[string]string)
	for i, entry := range entries {
		name := names[i]
		if browser.ValidateName(name) != nil {
			invalid[entry.Name] = true
			err = browser.ErrInvalidName
		}
		key := strings.ToUpper(name)
		if previous, exists := targets[key]; exists {
			invalid[previous], invalid[entry.Name] = true, true
			if err == nil {
				err = fs.ErrExist
			}
		}
		targets[key] = entry.Name
		for _, existing := range occupied[key] {
			if existing != entry.Name {
				invalid[entry.Name], invalid[existing] = true, true
				if err == nil {
					err = fs.ErrExist
				}
			}
		}
	}
	return invalid, err
}

func multiRenameError(err error) string {
	if errors.Is(err, fs.ErrExist) {
		return "Conflicting filenames"
	}
	if errors.Is(err, browser.ErrPathIdentity) {
		return "Item changed"
	}
	return renameError(err)
}

func (v *view) previewMultiRename(s *multiRenameSession) {
	s.preview, s.spans, s.invalid, s.changes = nil, nil, nil, nil
	s.expression = s.editor.Text()
	s.message = ""
	expr, err := renameexpr.Parse(s.editor.Text())
	var names []string
	if err == nil {
		entries := make([]renameexpr.Entry, len(s.entries))
		for i, entry := range s.entries {
			entries[i] = renameexpr.Entry{Name: entry.Name, IsDir: entry.Directory}
		}
		names, err = expr.Evaluate(entries)
	}
	if err != nil || strings.TrimSpace(s.expression) == "" {
		s.message = "Invalid expression"
	} else {
		p := &v.panels[s.panel]
		s.invalid, err = validateMultiRename(s.entries, names, p.entries)
		if err != nil {
			s.message = multiRenameError(err)
		}
		s.preview = make(map[string]string, len(names))
		s.spans = make(map[string][]renameSpan, len(names))
		for i, entry := range s.entries {
			if names[i] != entry.Name {
				s.preview[entry.Name] = names[i]
				s.spans[entry.Name] = renameSpans(entry.Name, names[i])
				s.changes = append(s.changes, renameChange{entry.Name, names[i]})
			}
		}
	}
	s.actions.disabled = []bool{s.message != "" || len(s.changes) == 0, false}
}

func (v *view) stopMultiRename(gtx layout.Context) {
	s := v.multiRenameInteraction()
	if s != nil && !s.pending {
		v.interaction.finish(s)
		v.focusInteraction(gtx)
	}
}

func (v *view) applyMultiRename(gtx layout.Context) {
	s := v.multiRenameInteraction()
	if s == nil || s.pending {
		return
	}
	v.previewMultiRename(s)
	if s.message != "" || len(s.changes) == 0 {
		return
	}
	if v.loader == nil {
		s.message = "Cannot rename"
		return
	}
	request := loadRequest{panel: s.panel, path: s.path, locations: v.fileOperationLocations(),
		multiRename: &multiRenameRequest{session: s, entries: append([]browser.Entry(nil), s.entries...), changes: append([]renameChange(nil), s.changes...)}}
	if !v.loader.submit(request) {
		s.message = "Folder is busy"
		return
	}
	s.pending, s.editor.ReadOnly = true, true
	v.operationProgress(s.panel, "Renaming")
	v.focusInteraction(gtx)
}

func (v *view) updateMultiRename(gtx layout.Context, s *multiRenameSession) {
	if s.pending {
		return
	}
	if action, ok := s.actions.update(gtx); ok {
		s.focus = action + 1
		if action == 0 {
			v.applyMultiRename(gtx)
		} else {
			v.escape(gtx)
		}
		v.focusInteraction(gtx)
		if v.multiRenameInteraction() != s || s.pending {
			return
		}
	}
	focus := any(&v.focus)
	if s.focus == 0 {
		focus = &s.editor
	}
	var keys []key.Event
	for {
		e, ok := gtx.Event(key.FocusFilter{Target: &v.focus}, key.Filter{Focus: focus, Name: key.NameEscape},
			key.Filter{Focus: &v.focus, Name: key.NameLeftArrow}, key.Filter{Focus: &v.focus, Name: key.NameRightArrow},
			key.Filter{Focus: &v.focus, Name: key.NameUpArrow}, key.Filter{Focus: &v.focus, Name: key.NameSpace},
			key.Filter{Focus: focus, Name: key.NameTab, Optional: key.ModShift},
			key.Filter{Focus: focus, Name: key.NameReturn, Optional: key.ModCtrl},
			key.Filter{Focus: focus, Name: key.NameEnter, Optional: key.ModCtrl})
		if !ok {
			break
		}
		if ke, ok := e.(key.Event); ok && ke.State == key.Press {
			keys = append(keys, ke)
		}
	}
	v.updateEditorInput(gtx, &s.editor)
	for {
		_, ok := s.editor.Update(gtx)
		if !ok {
			break
		}
	}
	if s.editor.Text() != s.expression {
		v.previewMultiRename(s)
	}
	if gtx.Focused(&s.editor) && s.focus != 0 {
		s.focus = 0
	}
	for _, ke := range keys {
		switch ke.Name {
		case key.NameLeftArrow, key.NameRightArrow, key.NameUpArrow:
			previous := s.focus
			if ke.Name == key.NameLeftArrow {
				s.focus = 1
			} else if ke.Name == key.NameRightArrow {
				s.focus = 2
			} else {
				s.focus = 0
			}
			if s.focus != previous {
				v.announceInteraction(eventSelectionChanged)
			}
			v.focusInteraction(gtx)
		case key.NameEscape:
			v.escape(gtx)
			return
		case key.NameTab:
			direction := 1
			if ke.Modifiers == key.ModShift {
				direction = -1
			}
			s.focus = (s.focus + direction + 3) % 3
			v.announceInteraction(eventSelectionChanged)
			v.focusInteraction(gtx)
		case key.NameReturn, key.NameEnter, key.NameSpace:
			if ke.Modifiers == key.ModCtrl || s.focus == 1 {
				v.applyMultiRename(gtx)
			} else if s.focus == 2 {
				v.escape(gtx)
			} else {
				v.previewMultiRename(s)
			}
			return
		}
	}
}

func (s *multiRenameSession) title() string { return fmt.Sprintf("Rename %d entries", len(s.entries)) }
