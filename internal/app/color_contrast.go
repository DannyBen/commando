package app

import (
	"image/color"
	"math"
)

// Resolve one edge using APCA alone, adjusting the member named by the rule.
// Prefer the nearest passing lightness along the original hue/chroma; RGB
// conversion reduces chroma only where needed to fit the gamut. If no candidate
// passes, keep the strongest permitted contrast. Results are cached by the caller.
func contrastColor(text, background color.NRGBA, rule contrastRule) color.NRGBA {
	return contrastColorOn(text, background, color.NRGBA{}, rule)
}

func contrastColorOn(text, background, surface color.NRGBA, rule contrastRule) color.NRGBA {
	score := func(value color.NRGBA) float64 { return math.Abs(perceptualContrast(value, background)) }
	visible := compositeText(text, background)
	original := text
	endpoints := []float64{0, 1}
	if rule.adjust == adjustBackground {
		original, visible = background, background
		if surface.A != 0 {
			visible = compositeText(background, surface)
		}
		score = func(value color.NRGBA) float64 {
			return math.Abs(perceptualContrast(compositeText(text, value), value))
		}
	}
	bounded := rule.adjust == adjustBackground && rule.minLightnessGap > 0
	if !bounded && score(visible) >= rule.targetLc {
		return original
	}
	base := oklchFromRGB(visible)
	if bounded {
		surfaceL := oklchFromRGB(surface).l
		gap := rule.minLightnessGap / 100
		limit := selectionLightnessLimit(base, surfaceL, gap, score)
		// Only move toward the surrounding surface, stopping at the visibility
		// limit. There is no black/white fallback for text-selection backgrounds.
		endpoints = []float64{limit}
		if math.Abs(base.l-surfaceL) < gap {
			base.l = limit
			visible = base.rgb()
			original = visible
		}
	}
	if score(visible) >= rule.targetLc {
		return original
	}
	best, bestScore := original, score(visible)
	nearest, distance := original, math.Inf(1)
	for _, endpoint := range endpoints {
		candidate := base
		candidate.l = endpoint
		extreme := candidate.rgb()
		extremeScore := score(extreme)
		if extremeScore > bestScore {
			best, bestScore = extreme, extremeScore
		}
		// The endpoint bounds the permitted correction in this direction.
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

// Preserve the requested side of the surface. If the colors coincide, choose
// the more readable side; at the gamut ends use the side with room for the gap.
func selectionLightnessLimit(base oklchColor, surfaceL, gap float64, score func(color.NRGBA) float64) float64 {
	direction := 1.0
	if base.l < surfaceL {
		direction = -1
	} else if base.l == surfaceL {
		below, above := base, base
		below.l, above.l = surfaceL-gap, surfaceL+gap
		if score(below.rgb()) > score(above.rgb()) {
			direction = -1
		}
	}
	limit := surfaceL + direction*gap
	if limit <= 0 || limit >= 1 {
		direction = -direction
		limit = surfaceL + direction*gap
	}
	candidate := base
	candidate.l = limit
	valid := func(value color.NRGBA) bool {
		return direction*(oklchFromRGB(value).l-surfaceL) >= gap
	}
	if valid(candidate.rgb()) {
		return limit
	}
	// RGB rounding can put the boundary just inside the forbidden gap. Find
	// the nearest representable color outside it, using the same hue/chroma.
	end := 0.0
	if direction > 0 {
		end = 1
	}
	low, high := limit, end
	for range 16 {
		candidate.l = (low + high) / 2
		if valid(candidate.rgb()) {
			high = candidate.l
		} else {
			low = candidate.l
		}
	}
	return high
}
