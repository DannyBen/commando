package app

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

// actionBar is a keyboard-controlled row of equal-width, clickable action cells.
// Its owner supplies focus, labels and activation, keeping session policy outside.
type actionBar struct {
	buttons  []clickTarget
	disabled []bool
}

func (b *actionBar) isDisabled(index int) bool {
	return index < len(b.disabled) && b.disabled[index]
}

func (b *actionBar) update(gtx layout.Context) (int, bool) {
	for i := range b.buttons {
		if b.buttons[i].Clicked(gtx) && !b.isDisabled(i) {
			return i, true
		}
	}
	return 0, false
}
func (b *actionBar) layout(gtx layout.Context, labels []string, selected, height int, theme Theme, label func(layout.Context, string, color.NRGBA, color.NRGBA) layout.Dimensions) layout.Dimensions {
	colors := theme.Colors
	if len(b.buttons) != len(labels) {
		b.buttons = make([]clickTarget, len(labels))
	}
	children := make([]layout.FlexChild, 0, len(labels))
	for i, value := range labels {
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			disabled := b.isDisabled(i)
			if disabled {
				gtx = gtx.Disabled()
			}
			size := image.Pt(gtx.Constraints.Max.X, height)
			gtx.Constraints = layout.Exact(size)
			return b.buttons[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				state := b.buttons[i].state(gtx, i == selected && !disabled)
				bg, fg := colors.Surface, colors.Foreground
				if state.selectedAppearance() {
					bg, fg = colors.Selected, colors.SelectedName
				}
				if disabled {
					fg = colors.Muted
				}
				bg = state.background(bg, theme.HoverStrength)
				paint.FillShape(gtx.Ops, bg, clip.Rect{Max: size}.Op())
				if i > 0 {
					paint.FillShape(gtx.Ops, colors.Border, clip.Rect{Max: image.Pt(gtx.Dp(1), height)}.Op())
				}
				paint.FillShape(gtx.Ops, colors.Border, clip.Rect{Min: image.Pt(0, height-gtx.Dp(1)), Max: size}.Op())
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return label(gtx, value, fg, bg) })
			})
		}))
	}
	return layout.Flex{}.Layout(gtx, children...)
}
