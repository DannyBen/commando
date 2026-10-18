package app

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/text"
)

func (v *view) updateLocationBar(gtx layout.Context, index int) {
	p := &v.panels[index]
	if p.locationBar == nil {
		return
	}
	input := gtx
	if v.interaction.panelsBlocked() || v.locationEditing() && v.active == index {
		input = gtx.Disabled()
	}
	for {
		click, ok := p.locationBar.Update(input)
		if !ok {
			return
		}
		if !v.cancelInteraction(gtx) {
			continue
		}
		v.dismissFeedback()
		v.focusPanel(index, false)
		v.focusInteraction(gtx)
		if click.NumClicks == 2 && click.Modifiers == 0 {
			v.startLocation(gtx)
			return
		}
	}
}

func (v *view) layoutLocationBar(gtx layout.Context, index, height int) layout.Dimensions {
	p := &v.panels[index]
	if v.locationEditing() && index == v.active {
		s := v.interaction.current.(*locationSession)
		return v.headerBand(gtx, height, v.appearance.Colors.Header, func(gtx layout.Context) layout.Dimensions {
			return v.completionEditor(gtx, &v.location, &s.completion, "Path")
		}, true)
	}
	if p.locationBar == nil {
		p.locationBar = new(clickTarget)
	}
	return p.locationBar.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		state := p.locationBar.state(gtx, false)
		bg, fg := v.appearance.Colors.Header, v.appearance.Colors.Foreground
		if index == v.active {
			fg = v.appearance.Colors.ActiveLocation
		}
		if state.pressedInside() {
			bg, fg = v.appearance.Colors.Selected, v.appearance.Colors.SelectedName
		}
		bg = state.background(bg)
		return v.headerBand(gtx, height, bg, func(gtx layout.Context) layout.Dimensions {
			return v.label(gtx, fmt.Sprintf("%d  %s", index+1, p.locationLabel()), v.appearance.TextSize, fg, text.Start)
		}, index == v.active)
	})
}
