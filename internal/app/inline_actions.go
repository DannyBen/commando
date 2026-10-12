package app

import (
	"image"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/widget"
)

// Inline actions are temporary file-list rows. The list owns their placement,
// focus, navigation and styling; callers provide labels and decision callbacks.
// They never enter the underlying listing, selection, marks or persisted state.
type inlineActionSession struct {
	panel, row, selected int
	subjectRow           int // Optional preceding file row, kept visible with its actions.
	headers              []string
	options              []string
	choose               func(int)
	canChoose            func() bool
	navigate             func(direction int, cycle bool) // Optional navigation that carries the inline interaction between panels.
	cancel               func()
	previousPosition     layout.Position
	previousReveal       bool
}

func (v *view) startInlineActions(panel, row int, title, detail string, options []string, choose func(int), cancel func()) {
	if !v.interaction.idle() {
		return
	}
	var headers []string
	for _, value := range []string{title, detail} {
		if value != "" {
			headers = append(headers, value)
		}
	}
	if len(headers) == 0 && len(options) == 0 {
		return
	}
	p := &v.panels[panel]
	v.interaction.begin(&inlineActionSession{panel: panel, row: max(0, min(row, len(p.items))), subjectRow: -1, headers: headers,
		options: options, choose: choose, cancel: cancel, previousPosition: p.list.Position, previousReveal: p.reveal})
	p.rows = make(map[int]*widget.Clickable)
	p.prefix, p.reveal = "", true
}

func (a *inlineActionSession) count() int { return len(a.headers) + len(a.options) }

func (v *view) stopInlineActions() *inlineActionSession {
	a := v.inlineInteraction()
	p := &v.panels[a.panel]
	p.list.Position, p.reveal = a.previousPosition, a.previousReveal
	if a.subjectRow >= 0 && a.subjectRow < len(p.items) {
		p.selected, p.reveal = a.subjectRow, true
		p.markRange = nil
	}
	p.rows = make(map[int]*widget.Clickable)
	v.interaction.finish(a)
	return a
}

func (v *view) cancelInlineActions() {
	a := v.stopInlineActions()
	if a.cancel != nil {
		a.cancel()
	}
}

func (v *view) chooseInlineAction() {
	if len(v.inlineInteraction().options) == 0 || v.inlineInteraction().canChoose != nil && !v.inlineInteraction().canChoose() {
		return
	}
	a := v.stopInlineActions()
	if a.choose != nil {
		a.choose(a.selected)
	}
}

func (v *view) inlineActionKey(ke key.Event) {
	a := v.inlineInteraction()
	if ke.State != key.Press {
		return
	}
	if ke.Name == key.NameTab && ke.Modifiers&^key.ModShift == 0 {
		direction := 1
		if ke.Modifiers&key.ModShift != 0 {
			direction = -1
		}
		v.navigateInlineActions(direction, true)
		return
	}
	if ke.Modifiers != 0 {
		return
	}
	switch ke.Name {
	case key.NameUpArrow:
		a.selected = max(0, a.selected-1)
	case key.NameDownArrow:
		a.selected = max(0, min(len(a.options)-1, a.selected+1))
	case key.NameHome:
		a.selected = 0
	case key.NameEnd:
		a.selected = max(0, len(a.options)-1)
	case key.NameReturn, key.NameEnter:
		v.chooseInlineAction()
	case key.NameEscape:
		v.cancelInlineActions()
	case key.NameLeftArrow:
		v.navigateInlineActions(-1, false)
	case key.NameRightArrow:
		v.navigateInlineActions(1, false)
	}
}

func (v *view) navigateInlineActions(direction int, cycle bool) {
	if navigate := v.inlineInteraction().navigate; navigate != nil {
		navigate(direction, cycle)
	} else {
		v.switchPanelTo(v.panelNeighbor(direction, cycle, -1))
	}
}

func (v *view) transientRows(panel int) (row, count int) {
	if a := v.inlineInteraction(); a != nil && a.panel == panel {
		return a.row, a.count()
	}
	if row := v.draftRow(panel); row >= 0 {
		return row, 1
	}
	return -1, 0
}

func (v *view) inlineActionRow(panel, row int) bool {
	a := v.inlineInteraction()
	return a != nil && a.panel == panel && row >= a.row && row < a.row+a.count()
}

func (v *view) clickInlineAction(gtx layout.Context, panel, row int) {
	a := v.inlineInteraction()
	v.active = panel
	if option := row - a.row - len(a.headers); option >= 0 && option < len(a.options) {
		a.selected = option
	}
	gtx.Execute(key.FocusCmd{Tag: &v.focus})
}

func (v *view) inlineActionContents(gtx layout.Context, row, rowHeight int) layout.Dimensions {
	a := v.inlineInteraction()
	index := row - a.row
	bg, fg := v.appearance.Colors.InlineSurface, v.appearance.Colors.InlineText
	value := ""
	if index < len(a.headers) {
		value = a.headers[index]
	} else {
		option := index - len(a.headers)
		value = "  " + a.options[option]
		if option == a.selected {
			value = "> " + a.options[option]
			if v.active == a.panel {
				bg, fg = v.appearance.Colors.Selected, v.appearance.Colors.SelectedName
			} else {
				bg = v.appearance.Colors.InlineInactiveSelected
			}
		}
	}
	size := image.Pt(gtx.Constraints.Max.X, rowHeight)
	paint.FillShape(gtx.Ops, bg, clip.Rect{Max: size}.Op())
	paint.FillShape(gtx.Ops, v.appearance.Colors.InlineEdge, clip.Rect{Max: image.Pt(gtx.Dp(3), rowHeight)}.Op())
	gtx.Constraints = layout.Exact(size)
	padding := max(0, v.appearance.RowPaddingHorizontal)
	return layout.Inset{Left: padding, Right: padding}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			return v.label(gtx, value, v.appearance.TextSize, fg, text.Start)
		})
	})
}
