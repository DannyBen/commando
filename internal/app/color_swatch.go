package app

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
)

// A passive swatch occupies the trailing end of a value field. The content
// retains its normal focus handling and progress line in the remaining width.
func colorValueControl(gtx layout.Context, height int, color, border color.NRGBA, content layout.Widget) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, content),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(min(2*height, gtx.Constraints.Max.X), min(height, gtx.Constraints.Max.Y))
				gtx.Constraints = layout.Exact(size)
				return widget.Border{Color: border, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					paint.FillShape(gtx.Ops, color, clip.Rect{Max: gtx.Constraints.Max}.Op())
					return layout.Dimensions{Size: gtx.Constraints.Max}
				})
			})
		}),
	)
}
