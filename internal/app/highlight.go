package app

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// Underline matching runs using the text color, preserving the theme's palette
// and selection contrast. Indices are runes, including for non-ASCII labels.
func (v *view) highlightedLabel(gtx layout.Context, value string, indices []int, size unit.Sp, fg color.NRGBA) layout.Dimensions {
	style := material.Label(v.theme, size, value)
	macro := op.Record(gtx.Ops)
	paint.ColorOp{Color: fg}.Add(gtx.Ops)
	material := macro.Stop()
	dims, info := (widget.Label{MaxLines: 1}).LayoutDetailed(gtx, style.Shaper, style.Font, size, value, material)
	runes := []rune(value)
	visible := len(runes) - info.Truncated
	for i := 0; i < len(indices); {
		start, end := indices[i], indices[i]+1
		i++
		for i < len(indices) && indices[i] == end {
			end++
			i++
		}
		if start >= visible {
			break
		}
		end = min(end, visible)
		left := v.textWidth(gtx, string(runes[:start]), size)
		right := min(dims.Size.X, v.textWidth(gtx, string(runes[:end]), size))
		y := dims.Size.Y - dims.Baseline + gtx.Dp(1)
		paint.FillShape(gtx.Ops, fg, clip.Rect{Min: image.Pt(left, y), Max: image.Pt(right, y+max(1, gtx.Dp(1)))}.Op())
	}
	return dims
}

func (v *view) textWidth(gtx layout.Context, value string, size unit.Sp) int {
	gtx.Constraints = layout.Constraints{Max: image.Pt(1_000_000, 1_000_000)}
	macro := op.Record(gtx.Ops)
	dims := v.label(gtx, value, size, v.appearance.Colors.Foreground, text.Start)
	macro.Stop()
	return dims.Size.X
}
