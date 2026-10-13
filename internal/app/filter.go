package app

import (
	"strings"
	"unicode/utf8"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/text"
	"github.com/dannyben/commando/internal/browser"
)

// The query belongs to the panel; only editing it owns a foreground session.
type filterSession struct {
	panel int
}

func (s *filterSession) interactionPanel() int { return s.panel }

func (v *view) filterInteraction() *filterSession {
	s, _ := v.interaction.current.(*filterSession)
	return s
}

func (p *panel) shows(file browser.Entry) bool {
	return p.visibility.shows(file) && (file.Parent || p.filter == "" ||
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
	if !folderReady(v) || !v.interaction.begin(&filterSession{panel: v.active}) {
		return
	}
	p := &v.panels[v.active]
	p.prefix = ""
	v.filterEditor.SetText(p.filter)
	v.filterEditor.SetCaret(0, utf8.RuneCountInString(p.filter))
	v.focusInteraction(gtx)
}

func (v *view) stopFilter(gtx layout.Context) {
	if s := v.filterInteraction(); s != nil {
		v.interaction.finish(s)
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

	v.panels[s.panel].setFilter(query)
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
			key.Filter{Focus: &v.filterEditor, Name: "1", Required: key.ModAlt},
			key.Filter{Focus: &v.filterEditor, Name: "2", Required: key.ModAlt},
			key.Filter{Focus: &v.filterEditor, Name: "3", Required: key.ModAlt},
			key.Filter{Focus: &v.filterEditor, Name: "4", Required: key.ModAlt},
			key.Filter{Focus: &v.filterEditor, Name: "5", Required: key.ModAlt},
			key.Filter{Focus: &v.filterEditor, Name: "6", Required: key.ModAlt},
			key.Filter{Focus: &v.filterEditor, Name: "7", Required: key.ModAlt},
			key.Filter{Focus: &v.filterEditor, Name: "8", Required: key.ModAlt},
			key.Filter{Focus: &v.filterEditor, Name: "9", Required: key.ModAlt},
		)
		if !ok {
			break
		}
		if ke, ok := e.(key.Event); ok && ke.State == key.Press {
			keys = append(keys, ke)
		}
	}
	v.updateFilterText(gtx)
	for _, ke := range keys {
		v.dismissFeedback()
		switch ke.Name {
		case key.NameEscape:
			v.escape(gtx)
			return
		case key.NameReturn, key.NameEnter:
			v.stopFilter(gtx)
			return
		case key.NameUpArrow, key.NameDownArrow:
			delta := 1
			if ke.Name == key.NameUpArrow {
				delta = -1
			}
			v.selectBrowsingRow(ke, v.panels[v.active].selected+delta)
		default:
			v.stopFilter(gtx)
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
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.label(gtx, "/ ", v.appearance.SmallTextSize, fg, text.Start)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if s := v.filterInteraction(); s != nil && s.panel == index {
				editor := v.textEditor(&v.filterEditor, "Filter files")
				editor.TextSize = v.appearance.SmallTextSize
				return editor.Layout(gtx)
			}
			return v.label(gtx, p.filter, v.appearance.SmallTextSize, fg, text.Start)
		}),
	)
}
