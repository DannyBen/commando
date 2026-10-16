package app

import (
	"image/color"
	"math"
)

// Resolve one edge using APCA alone, adjusting the member named by the rule.
// Prefer the nearest
// passing lightness along the original hue/chroma; RGB conversion reduces chroma
// only where needed to fit the gamut. If no candidate can pass, keep the strongest
// contrast found, including black and white. Results are cached by the caller.
func contrastColor(text, background color.NRGBA, rule contrastRule) color.NRGBA {
	score := func(value color.NRGBA) float64 { return math.Abs(perceptualContrast(value, background)) }
	visible := compositeText(text, background)
	original := text
	if rule.adjust == adjustBackground {
		original, visible = background, background
		score = func(value color.NRGBA) float64 {
			return math.Abs(perceptualContrast(compositeText(text, value), value))
		}
	}
	if score(visible) >= rule.targetLc {
		return original
	}
	base := oklchFromRGB(visible)
	best, bestScore := original, score(visible)
	nearest, distance := original, math.Inf(1)
	for _, endpoint := range []float64{0, 1} {
		extreme := rgb(0)
		if endpoint == 1 {
			extreme = rgb(0xffffff)
		}
		extremeScore := score(extreme)
		if extremeScore > bestScore {
			best, bestScore = extreme, extremeScore
		}
		// Black and white bound APCA contrast in each lightness direction.
		if extremeScore < rule.targetLc {
			continue
		}
		low, high := 0.0, 1.0
		passing := extreme
		for range 16 {
			fraction := (low + high) / 2
			candidate := base
			candidate.l += (endpoint - base.l) * fraction
			value := candidate.rgb()
			contrast := score(value)
			if contrast > bestScore {
				best, bestScore = value, contrast
			}
			if contrast >= rule.targetLc {
				high, passing = fraction, value
			} else {
				low = fraction
			}
		}
		if change := high * math.Abs(endpoint-base.l); change < distance {
			nearest, distance = passing, change
		}
	}
	if !math.IsInf(distance, 1) {
		return nearest
	}
	return best
}
