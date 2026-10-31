package app

import (
	"fmt"
	"image/color"

	"gioui.org/layout"
	"gioui.org/text"
	"github.com/dannyben/commando/internal/settings"
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
	preview := false
	if s := v.settingsInteraction(); s != nil && len(s.fields) > 0 {
		preview = s.fields[0].Key == settingLocationScrollEnabled
	}
	animate := index == v.active && (v.interaction.idle() && gtx.Focused(&v.focus) || preview) &&
		v.settingsValues.Get(settings.CatalogueDefault, settingLocationScrollEnabled).Bool
	if !animate {
		p.locationScroll = textMarquee{}
	}
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
		bg = state.background(bg, v.appearance.HoverStrength)
		return v.headerBand(gtx, height, bg, func(gtx layout.Context) layout.Dimensions {
			if !animate {
				return v.label(gtx, fmt.Sprintf("%d  %s", index+1, p.locationLabel()), v.appearance.TextSize, fg, text.Start)
			}
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return v.label(gtx, fmt.Sprintf("%d  ", index+1), v.appearance.TextSize, fg, text.Start)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return v.scrollingLocation(gtx, p, fg)
				}),
			)
		}, index == v.active)
	})
}

func (v *view) scrollingLocation(gtx layout.Context, p *panel, fg color.NRGBA) layout.Dimensions {
	return v.scrollingText(gtx, &p.locationScroll, p.locationLabel(), settingLocationScroll, func(gtx layout.Context, content string) layout.Dimensions {
		return v.label(gtx, content, v.appearance.TextSize, fg, text.Start)
	})
}
