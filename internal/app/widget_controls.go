package app

import (
	"image"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
)

// Help can take priority while its control owns focus. Elsewhere, validation
// explains why the widget cannot be applied before showing help or key hints.
type widgetFooter struct {
	hint, help, errorText string
	rightHint             string
	helpFocused           bool
}

func (f widgetFooter) message() (string, bool) {
	if f.helpFocused && f.help != "" {
		return f.help, false
	}
	if f.errorText != "" {
		return f.errorText, true
	}
	if f.help != "" {
		return f.help, false
	}
	return f.hint, false
}

func (v *view) layoutWidgetFooter(gtx layout.Context, height int, footer widgetFooter) layout.Dimensions {
	return v.band(gtx, height, v.appearance.Colors.Footer, func(gtx layout.Context) layout.Dimensions {
		message, invalid := footer.message()
		fg := v.appearance.Colors.Foreground
		if invalid {
			fg = v.appearance.Colors.Error
		}
		content := func(gtx layout.Context) layout.Dimensions {
			return v.label(gtx, message, v.appearance.SmallTextSize, fg, text.Start)
		}
		if footer.rightHint == "" {
			return content(gtx)
		}
		return v.trailingLayout(gtx, content, func(gtx layout.Context) layout.Dimensions {
			return v.label(gtx, footer.rightHint, v.appearance.SmallTextSize, v.appearance.Colors.Foreground, text.End)
		})
	}, false)
}

// Value fields and steppers share their full-width focus, hover and text inset.
// Passing nil retains the same inset for an active native editor.
func (v *view) widgetValueContent(gtx layout.Context, target *valueTarget, focused bool, height int, content layout.Widget) layout.Dimensions {
	bg := v.appearance.Colors.Surface
	if target != nil {
		state := target.state(gtx, focused)
		if state.selectedAppearance() {
			bg = v.appearance.Colors.Selected
		}
		bg = state.background(bg, v.appearance.HoverStrength)
		paint.FillShape(gtx.Ops, bg, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, height)}.Op())
	}
	return (layout.Inset{Left: 4, Right: 4}).Layout(gtx, v.textOn(bg, content))
}

// Single-line editors participate in row navigation unless their owner uses
// these keys for suggestions. Modified keys remain native editor input.
func widgetRowDirection(ke key.Event) int {
	if ke.State != key.Press || ke.Modifiers != 0 {
		return 0
	}
	switch ke.Name {
	case key.NameUpArrow:
		return -1
	case key.NameDownArrow:
		return 1
	}
	return 0
}

func widgetRowNext(row, count, direction int) int {
	return max(0, min(count-1, row+direction))
}
