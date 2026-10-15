package app

import (
	"image/color"
	"math"
)

func themeLuminance(c color.NRGBA) float64 {
	linear := func(n uint8) float64 {
		x := float64(n) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(c.R) + 0.7152*linear(c.G) + 0.0722*linear(c.B)
}

func themeContrast(a, b color.NRGBA) float64 {
	x, y := themeLuminance(a), themeLuminance(b)
	return (max(x, y) + 0.05) / (min(x, y) + 0.05)
}

// Selection retains normal text at Lc 75, otherwise uses the stronger neutral.
// Lc 75 is the initial policy for Commando's small interface text. Rank absolute
// magnitudes while retaining the signed scores in the underlying APCA function.
func selectionTextColor(text, background color.NRGBA) color.NRGBA {
	if math.Abs(perceptualContrast(text, background)) >= 75 {
		return text
	}
	black, white := rgb(0), rgb(0xffffff)
	if math.Abs(perceptualContrast(white, background)) > math.Abs(perceptualContrast(black, background)) {
		return white
	}
	return black
}
