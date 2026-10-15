package app

import (
	"image"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestHueSwatchTracksAllColorComponents(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	v.startSettingsWidget(colorPrefix + "edge")
	before := v.appearance.Colors.Edge
	send, _ := panelInputFrames(v)
	send(pressed(key.NameDownArrow)) // Element -> Lightness.
	for i, component := range []string{"lightness", "saturation", "hue"} {
		previous := v.appearance.Colors.Edge
		send(pressed(key.NameRightArrow))
		swatch, ok := v.settingSwatch(colorPrefix + "edge.hue")
		if !ok || swatch != v.appearance.Colors.Edge || swatch == previous {
			t.Fatalf("swatch did not follow %s", component)
		}
		if component != "hue" {
			if _, ok := v.settingSwatch(colorPrefix + "edge." + component); ok {
				t.Fatal("swatch appeared outside Hue")
			}
		}
		if i < 2 {
			send(pressed(key.NameDownArrow))
		}
	}
	send(pressed(key.NameTab))
	if !v.settingsInteraction().actionsFocused {
		t.Fatal("swatch added a keyboard focus stop")
	}
	v.finishSettingsWidget(layout.Context{}, false)
	if swatch, _ := v.settingSwatch(colorPrefix + "edge.hue"); swatch != before {
		t.Fatal("Cancel did not restore the swatch")
	}
}

func TestSwatchMatchesFieldHeightAndReservesItsWidth(t *testing.T) {
	for _, scale := range []float32{1, 1.5, 2} {
		v := newPreviewView()
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}}
		height := v.textHeight(gtx, v.appearance.TextSize)
		gtx.Constraints = layout.Exact(image.Pt(240, height))
		width := 0
		dims := colorValueControl(gtx, height, v.appearance.Colors.Edge, v.appearance.Colors.Border, func(gtx layout.Context) layout.Dimensions {
			width = gtx.Constraints.Max.X
			return layout.Dimensions{Size: image.Pt(width, height)}
		})
		if dims.Size != image.Pt(240, height) || width != 240-2*height-gtx.Dp(4) {
			t.Fatalf("scale %g: field %v, remaining width %d, swatch height %d", scale, dims.Size, width, height)
		}
	}
}

func TestSettingsSwatchPreviews(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional color swatch previews")
	}
	for _, role := range []string{"background", "edge", "error"} {
		v := newPreviewView()
		v.applySettings()
		v.startSettingsWidget(colorPrefix + role)
		v.settingsInteraction().selected = 3
		if err := renderViewPNG(filepath.Join(dir, "theme-swatch-"+role+".png"), v); err != nil {
			t.Fatal(err)
		}
	}
}
