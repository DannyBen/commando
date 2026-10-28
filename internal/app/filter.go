package app

import (
	"path/filepath"
	"strings"
	"unicode/utf8"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/text"
	"github.com/dannyben/commando/internal/browser"
)

// The query belongs to the panel; only editing it owns a foreground session.
type filterSession struct {
	panel int
	deep  bool
}

func (s *filterSession) interactionPanel() int { return s.panel }

func (v *view) filterInteraction() *filterSession {
	s, _ := v.interaction.current.(*filterSession)
	return s
}

func (p *panel) shows(file browser.Entry) bool {
	if file.Parent {
		return p.showParentEntry
	}
	visible := p.visibility.shows(file)
	if p.showProtectedSystem && file.Hidden && file.System {
		copy := file
		copy.System = false
		visible = p.visibility.shows(copy)
	}
	if p.deep != nil {
		return visible && p.deep.includes(filepath.Join(p.path, file.Name))
	}
	return visible && (file.Parent || p.filter == "" ||
		strings.Contains(strings.ToLower(file.Name), strings.ToLower(p.filter)))
}

func (p *panel) setFilter(query string) {
	if query == p.filter {
		return
	}
	p.ensureEntries()
	p.filter = query
	p.refilter()
}

func (v *view) startFilter(gtx layout.Context) {
	if !folderReady(v) || !v.startInteraction(&filterSession{panel: v.active}) {
		return
	}
	p := &v.panels[v.active]
	if p.deep != nil {
		v.clearDeepSearch(v.active)
	}
	p.prefix = ""
	v.filterEditor.SetText(p.filter)
	v.filterEditor.SetCaret(0, utf8.RuneCountInString(p.filter))
	v.focusInteraction(gtx)
}

func (v *view) startDeepFilter(gtx layout.Context) {
	if !folderReady(v) || v.loader == nil || !v.panels[v.active].nativeFolder() || !v.startInteraction(&filterSession{panel: v.active, deep: true}) {
		return
	}
	v.panels[v.active].prefix = ""
	v.filterEditor.SetText("")
	v.focusInteraction(gtx)
}

func (v *view) stopFilter(gtx layout.Context) {
	if s := v.filterInteraction(); s != nil {
		v.finishInteraction(s)
		v.focusInteraction(gtx)
	}
}

func (v *view) updateFilterText(gtx layout.Context) {
	s := v.filterInteraction()
	for {
		_, ok := v.filterEditor.Update(gtx)
		if !ok {
			break
		}
	}
	query := v.filterEditor.Text()

	if !s.deep {
		v.panels[s.panel].setFilter(query)
	}
}

func (v *view) updateFilter(gtx layout.Context) {
	var keys []key.Event
	for {
		e, ok := gtx.Event(
			key.Filter{Focus: &v.filterEditor, Name: key.NameEscape},
			key.Filter{Focus: &v.filterEditor, Name: key.NameReturn},
			key.Filter{Focus: &v.filterEditor, Name: key.NameEnter},
			key.Filter{Focus: &v.filterEditor, Name: key.NameUpArrow},
			key.Filter{Focus: &v.filterEditor, Name: key.NameDownArrow},
			key.Filter{Name: key.NameTab, Optional: key.ModShift},
		)
		if !ok {
			break
		}
		if ke, ok := e.(key.Event); ok && ke.State == key.Press {
			keys = append(keys, ke)
		}
	}
	v.updateFilterText(gtx)
	// Let native text editing consume its keys before offering configurable
	// panel-focus shortcuts to the filter session.
	var focusFilters []event.Filter
	for index := range v.panels {
		c := v.resolveCommandBindings(panelFocusCommand(index + 1))
		for slot := range c.bindings {
			if shortcut := c.effectiveShortcut(slot); shortcut.Name != "" {
				focusFilters = append(focusFilters, key.Filter{Focus: &v.filterEditor, Name: shortcut.Name, Required: shortcut.Modifiers})
				if shortcut.Name == key.NameReturn {
					focusFilters = append(focusFilters, key.Filter{Focus: &v.filterEditor, Name: key.NameEnter, Required: shortcut.Modifiers})
				}
			}
		}
	}
	for {
		e, ok := gtx.Event(focusFilters...)
		if !ok {
			break
		}
		if ke, ok := e.(key.Event); ok && ke.State == key.Press {
			keys = append(keys, ke)
		}
	}

	for _, ke := range keys {
		v.dismissFeedback()
		if ke.Modifiers != 0 && ke.Name != key.NameTab {
			v.interaction.finish(v.filterInteraction()) // Handoff; the next command owns audio.
			v.focusInteraction(gtx)
			v.browseKey(gtx, ke)
			return
		}
		switch ke.Name {
		case key.NameEscape:
			if s := v.filterInteraction(); s != nil && s.deep {
				v.clearDeepSearch(s.panel)
				v.stopFilter(gtx)
			} else {
				v.escape(gtx)
			}
			return
		case key.NameReturn, key.NameEnter:
			if s := v.filterInteraction(); s != nil && s.deep {
				query := strings.TrimSpace(v.filterEditor.Text())
				if query != "" {
					v.startDeepSearch(s.panel, query)
				}
			}
			v.stopFilter(gtx)
			return
		case key.NameUpArrow, key.NameDownArrow:
			delta := 1
			if ke.Name == key.NameUpArrow {
				delta = -1
			}
			v.selectBrowsingRow(v.panels[v.active].selected+delta, false, true)
		default:
			v.interaction.finish(v.filterInteraction())
			v.focusInteraction(gtx)
			v.browseKey(gtx, ke)
			return
		}
	}
}

func (v *view) filterContents(gtx layout.Context, index int) layout.Dimensions {
	p := &v.panels[index]
	fg := v.appearance.Colors.Filename
	if s := v.filterInteraction(); s != nil && s.panel == index {
		fg = v.appearance.Colors.EditorText
	}
	if v.followMe {
		_, fg, _ = v.metadataHeaderColors(index)
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			prompt := "/ "
			if s := v.filterInteraction(); s != nil && s.panel == index && s.deep || p.deep != nil {
				prompt = "* "
			}
			return v.label(gtx, prompt, v.appearance.SmallTextSize, fg, text.Start)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if s := v.filterInteraction(); s != nil && s.panel == index {
				hint := "Filter"
				if s.deep {
					hint = "Search"
				}
				editor := v.textEditor(&v.filterEditor, hint)
				editor.TextSize = v.appearance.SmallTextSize
				if v.followMe {
					editor.Color = v.resolvedText(fg)
					editor.HintColor = v.resolvedTextFor(contrastDimmedText, v.dimmedColor(fg), v.currentTextBackground())
					editor.SelectionColor = v.resolvedTextSelection(editor.Color, v.currentTextBackground())
				}
				return editor.Layout(gtx)
			}
			query := p.filter
			if p.deep != nil {
				query = p.deep.query
			}
			return v.label(gtx, query, v.appearance.SmallTextSize, fg, text.Start)
		}),
	)
}
