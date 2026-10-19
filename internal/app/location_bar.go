package app

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
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
	if index != v.active || v.locationEditing() {
		p.locationScroll = locationMarquee{}
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
			if index != v.active {
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

type locationMarquee struct {
	path             string
	width, fullWidth int
	start            time.Time
}

func (m *locationMarquee) offset(gtx layout.Context, path string, width, fullWidth int) float32 {
	if m.path != path || m.width != width || m.fullWidth != fullWidth {
		*m = locationMarquee{path: path, width: width, fullWidth: fullWidth, start: gtx.Now}
	}
	if width <= 0 || fullWidth <= width {
		return 0
	}
	due := m.start.Add(locationScrollDelay)
	if gtx.Now.Before(due) {
		gtx.Execute(op.InvalidateCmd{At: due})
		return 0
	}
	end := float32(fullWidth - width)
	travel := time.Duration(float64(fullWidth-width) * float64(time.Second) / float64(gtx.Dp(locationScrollSpeed)))
	cycle := 2*travel + locationScrollEndPause + locationScrollStartPause
	phase := gtx.Now.Sub(due) % cycle
	next := gtx.Now.Add(time.Second / 60)
	var offset float32
	switch {
	case phase < travel:
		offset = end * float32(phase) / float32(travel)
	case phase < travel+locationScrollEndPause:
		offset = end
		next = gtx.Now.Add(travel + locationScrollEndPause - phase)
	case phase < 2*travel+locationScrollEndPause:
		offset = end * (1 - float32(phase-travel-locationScrollEndPause)/float32(travel))
	default:
		next = gtx.Now.Add(cycle - phase)
	}
	// Pauses need just one future wake-up, rather than continuous redraws.
	gtx.Execute(op.InvalidateCmd{At: next})
	return offset
}

func (v *view) scrollingLocation(gtx layout.Context, p *panel, fg color.NRGBA) layout.Dimensions {
	path := p.locationLabel()
	full := gtx
	full.Constraints = layout.Constraints{Max: image.Pt(1_000_000, gtx.Constraints.Max.Y)}
	// Shape the complete string once, then translate its drawing operations.
	macro := op.Record(gtx.Ops)
	dims := v.label(full, path, v.appearance.TextSize, fg, text.Start)
	drawing := macro.Stop()
	width := gtx.Constraints.Max.X
	offset := p.locationScroll.offset(gtx, path, width, dims.Size.X)
	defer clip.Rect{Max: image.Pt(width, dims.Size.Y)}.Push(gtx.Ops).Pop()
	defer op.Affine(f32.Affine2D{}.Offset(f32.Pt(-offset, 0))).Push(gtx.Ops).Pop()
	drawing.Add(gtx.Ops)
	dims.Size.X = width
	return dims
}
