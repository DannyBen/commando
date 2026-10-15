package app

import "math"

// LSH is Commando's editable representation of OKLCH: lightness and saturation
// use 0–100 points; hue uses degrees. Saturation is a fraction of the sRGB gamut
// available at that lightness and hue, rather than an absolute chroma value.
type lshColor struct{ l, s, h float64 }

func maximumChroma(lightness, hue float64) float64 {
	if lightness <= 0 || lightness >= 1 {
		return 0
	}
	// 0.5 exceeds sRGB's maximum OKLCH chroma at every lightness and hue.
	return (oklchColor{lightness, 0.5, wrapHue(hue)}).gamutMapped().c
}

func (value lshColor) oklch() oklchColor {
	l, h := clampLightness(value.l/100), wrapHue(value.h)
	return oklchColor{l, max(0, min(100, value.s)) / 100 * maximumChroma(l, h), h}
}

func lshFromOKLCH(value oklchColor) lshColor {
	l, h := clampLightness(value.l), wrapHue(value.h)
	s := 0.0
	if maximum := maximumChroma(l, h); maximum > 0 {
		s = max(0, min(100, value.c/maximum*100))
	}
	return lshColor{l * 100, s, h}
}

// Round only at the settings boundary; generation and conversion keep precision.
func colorSettingNumber(value float64) float64 { return math.Round(value*10000) / 10000 }
