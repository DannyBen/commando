package app

import (
	"image"
	"os"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func TestBandContentCannotCoverBorderOrAdjacentRows(t *testing.T) {
	if os.Getenv("COMMANDO_PREVIEW_SETTINGS") == "" {
		t.Skip("optional rendered border regression")
	}
	for _, scale := range []float32{1, 1.5, 2} {
		w, err := headless.NewWindow(120, 100)
		if err != nil {
			t.Fatal(err)
		}
		v := newPreviewView()
		v.appearance.Colors.Border = rgb(0x00ff00)
		var ops op.Ops
		paint.Fill(&ops, rgb(0x0000ff))
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}}
		height, border := gtx.Dp(20), gtx.Dp(1)
		gtx.Constraints = layout.Exact(image.Pt(80, height))
		at := op.Offset(image.Pt(10, 10)).Push(&ops)
		v.band(gtx, height, rgb(0), func(gtx layout.Context) layout.Dimensions {
			// A full-row sample must remain inside its content area even if
			// its renderer attempts to paint beyond its reported dimensions.
			paint.Fill(gtx.Ops, rgb(0xff0000))
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}, false)
		at.Pop()
		if err := w.Frame(&ops); err != nil {
			w.Release()
			t.Fatal(err)
		}
		img := image.NewRGBA(image.Rect(0, 0, 120, 100))
		err = w.Screenshot(img)
		w.Release()
		if err != nil {
			t.Fatal(err)
		}
		for y := 10 + height - border; y < 10+height; y++ {
			c := img.RGBAAt(40, y)
			if c.R != 0 || c.G != 255 || c.B != 0 {
				t.Fatalf("scale %g: content covered border at y=%d: %v", scale, y, c)
			}
		}
		for _, point := range []image.Point{{40, 9}, {40, 10 + height}, {9, 12}, {90, 12}} {
			c := img.RGBAAt(point.X, point.Y)
			if c.R != 0 || c.G != 0 || c.B != 255 {
				t.Fatalf("scale %g: content escaped band at %v: %v", scale, point, c)
			}
		}
	}
}
