package app

import (
	"slices"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/dannyben/commando/internal/fuzzy"
)

type commandMatch struct {
	command command
	match   fuzzy.Result
}

// Search and its results belong to one foreground session. The originating
// pane stays active and protected until dismissal or command dispatch.
type paletteSession struct {
	panel           int
	query           string
	selected        int
	results         []commandMatch
	list            widget.List
	rows            map[int]*widget.Clickable
	surface         overlay
	reveal          bool
	openingBacktick bool
}

func (s *paletteSession) interactionPanel() int { return s.panel }

func (v *view) paletteInteraction() *paletteSession {
	s, _ := v.interaction.current.(*paletteSession)
	return s
}

func (v *view) startPalette(gtx layout.Context) {
	s := &paletteSession{panel: v.active, rows: make(map[int]*widget.Clickable)}
	if !v.interaction.begin(s) {
		return
	}
	s.list.Axis = layout.Vertical
	v.commandSearch.SetText("")
	s.filter("")
	v.focusInteraction(gtx)
}

func (s *paletteSession) filter(query string) {
	s.query, s.selected, s.reveal = query, 0, true
	s.results = nil
	needle := strings.TrimSpace(query)
	for _, command := range commands {
		if match, ok := fuzzy.Match(needle, command.label()); ok {
			s.results = append(s.results, commandMatch{command: command, match: match})
		}
	}
	if needle != "" {
		slices.SortStableFunc(s.results, func(a, b commandMatch) int {
			if a.match.Score > b.match.Score {
				return -1
			}
			if a.match.Score < b.match.Score {
				return 1
			}
			return strings.Compare(a.command.label(), b.command.label())
		})
	}
	s.rows = make(map[int]*widget.Clickable)
	s.list.ScrollTo(0)
}

func (v *view) stopPalette(gtx layout.Context) {
	if s := v.paletteInteraction(); s != nil {
		v.interaction.finish(s)
		v.focusInteraction(gtx)
	}
}

func (v *view) choosePalette(gtx layout.Context) {
	s := v.paletteInteraction()
	if s == nil || s.selected < 0 || s.selected >= len(s.results) {
		return
	}
	command := s.results[s.selected].command
	if !command.allowed(v) {
		return
	}
	v.stopPalette(gtx)
	v.browseKey(gtx, command.binding)
	v.focusInteraction(gtx)
}

func (v *view) updatePaletteSearch(gtx layout.Context) {
	s := v.paletteInteraction()
	for {
		_, ok := v.commandSearch.Update(gtx)
		if !ok {
			break
		}
	}
	// Editor.Layout can consume events on the opening frame, before the new
	// session's update runs. Always reconcile its current text, not just changes.
	query := v.commandSearch.Text()
	if s.openingBacktick && query != "" {
		s.openingBacktick = false
		if strings.HasPrefix(query, "`") {
			query = strings.TrimPrefix(query, "`")
			v.commandSearch.SetText(query)
		}
	}
	if query != s.query {
		s.filter(query)
	}
}

func (v *view) updatePalette(gtx layout.Context) {
	s := v.paletteInteraction()
	if s.surface.outsidePress(gtx) {
		v.stopPalette(gtx)
		return
	}
	for row, clickable := range s.rows {
		for clickable.Clicked(gtx) {
			s.selected, s.reveal = row, true
			v.focusInteraction(gtx)
		}
	}
	var navigation []key.Event
	for {
		e, ok := gtx.Event(
			key.Filter{Focus: &v.commandSearch, Name: key.NameEscape},
			key.Filter{Focus: &v.commandSearch, Name: key.NameUpArrow},
			key.Filter{Focus: &v.commandSearch, Name: key.NameDownArrow},
			key.Filter{Focus: &v.commandSearch, Name: key.NameHome},
			key.Filter{Focus: &v.commandSearch, Name: key.NameEnd},
			key.Filter{Focus: &v.commandSearch, Name: key.NamePageUp},
			key.Filter{Focus: &v.commandSearch, Name: key.NamePageDown},
			key.Filter{Focus: &v.commandSearch, Name: key.NameReturn},
			key.Filter{Focus: &v.commandSearch, Name: key.NameEnter},
			key.Filter{Focus: &v.commandSearch, Name: "P", Required: key.ModCtrl | key.ModShift},
		)
		if !ok {
			break
		}
		ke, ok := e.(key.Event)
		if ok && ke.State == key.Press {
			navigation = append(navigation, ke)
		}
	}
	// Claim navigation before Editor.Update can consume those same keys. Native
	// text edits update the result set before the ordered selection/run keys.
	v.updatePaletteSearch(gtx)
	for _, ke := range navigation {
		switch ke.Name {
		case key.NameEscape, "P":
			v.stopPalette(gtx)
			return
		case key.NameReturn, key.NameEnter:
			v.choosePalette(gtx)
			return
		case key.NameUpArrow:
			s.selected--
		case key.NameDownArrow:
			s.selected++
		case key.NameHome:
			s.selected = 0
		case key.NameEnd:
			s.selected = len(s.results) - 1
		case key.NamePageUp:
			s.selected -= max(1, s.list.Position.Count-1)
		case key.NamePageDown:
			s.selected += max(1, s.list.Position.Count-1)
		}
		s.selected = max(0, min(s.selected, len(s.results)-1))
		s.reveal = true
	}
}
