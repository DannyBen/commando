package app

import (
	"image/color"
	"math"
)

// APCA 0.0.98G, ported from Color.js src/contrast/APCA.js (MIT).
// Copyright (c) 2021 Lea Verou, Chris Lilley. See LICENSE-COLORJS.
// https://github.com/color-js/color.js/blob/main/src/contrast/APCA.js
//
// Inputs are opaque sRGB colors, text first. Positive scores mean dark text on
// a lighter background; negative scores mean light text on a darker background.
// Magnitude is perceptual contrast (Lc), not a WCAG 2 contrast ratio.
func perceptualContrast(text, background color.NRGBA) float64 {
	textY, bgY := apcaLuminance(text), apcaLuminance(background)
	if math.Abs(bgY-textY) < 0.0005 {
		return 0
	}

	if bgY > textY {
		contrast := (math.Pow(bgY, 0.56) - math.Pow(textY, 0.57)) * 1.14
		if contrast < 0.1 {
			return 0
		}
		return (contrast - 0.027) * 100
	}
	contrast := (math.Pow(bgY, 0.65) - math.Pow(textY, 0.62)) * 1.14
	if contrast > -0.1 {
		return 0
	}
	return (contrast + 0.027) * 100
}

func apcaLuminance(value color.NRGBA) float64 {
	linear := func(n uint8) float64 { return math.Pow(float64(n)/255, 2.4) }
	y := 0.2126729*linear(value.R) + 0.7151522*linear(value.G) + 0.072175*linear(value.B)
	// Soft-clamp near black to account for display flare.
	if y < 0.022 {
		y += math.Pow(0.022-y, 1.414)
	}
	return y
}
