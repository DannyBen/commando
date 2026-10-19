package app

import (
	"image/color"
	"math"
)

// Presses preview the existing selected appearance without changing selection.
func (s targetState) pressedInside() bool { return s.Enabled && s.Hovered && s.Pressed }

func (s targetState) selectedAppearance() bool { return s.Selected || s.pressedInside() }

// Shift the actual background, including selection colors. Blending neutral
// channels keeps the effect visible even on pure black and white surfaces.
func (s targetState) background(base color.NRGBA, strength float64) color.NRGBA {
	if !s.Enabled || !s.Hovered || s.Pressed {
		return base
	}
	tone := 255.0
	if oklchFromRGB(base).l >= 0.65 {
		tone = 0
	}
	shift := func(channel uint8) uint8 {
		return uint8(math.Round(float64(channel)*(1-strength) + tone*strength))
	}
	return color.NRGBA{R: shift(base.R), G: shift(base.G), B: shift(base.B), A: base.A}
}
