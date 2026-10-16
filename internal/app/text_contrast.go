package app

import (
	"image/color"
	"math"

	"gioui.org/layout"
)

type textContrastPair struct {
	foreground, background color.NRGBA
	rule                   contrastRule
	surface                color.NRGBA
}

// Resolved display colors are disposable and never enter the saved palette.
// Theme changes clear this cache; dynamic file-row pairs are resolved on demand.
type textContrastCache map[textContrastPair]color.NRGBA

func (c *textContrastCache) resolve(foreground, background color.NRGBA, role contrastRole) color.NRGBA {
	return c.resolveOn(foreground, background, color.NRGBA{}, role)
}

func (c *textContrastCache) resolveOn(foreground, background, surface color.NRGBA, role contrastRole) color.NRGBA {
	rule := themeContrastRules[role]
	pair := textContrastPair{foreground, background, rule, surface}
	if resolved, ok := (*c)[pair]; ok {
		return resolved
	}
	resolved := contrastColorOn(foreground, background, surface, rule)
	if *c == nil {
		*c = make(textContrastCache)
	}
	(*c)[pair] = resolved
	return resolved
}

// Gio uses one foreground for both selected and unselected editor text.
// Resolve it against the ordinary surface first, then adjust selection alone.
func (v *view) resolvedTextSelection(text, surface color.NRGBA) color.NRGBA {
	return v.textContrast.resolveOn(text, v.appearance.Colors.TextSelection, surface, contrastTextSelection)
}

func linearColorChannel(n uint8) float64 {
	x := float64(n) / 255
	if x <= 0.04045 {
		return x / 12.92
	}
	return math.Pow((x+0.055)/1.055, 2.4)
}

// Gio blends in linear light. Include text opacity when checking dimmed labels.
func compositeText(foreground, background color.NRGBA) color.NRGBA {
	if foreground.A == 255 {
		return foreground
	}
	alpha := float64(foreground.A) / 255
	channel := func(fg, bg uint8) uint8 {
		value := linearColorChannel(fg)*alpha + linearColorChannel(bg)*(1-alpha)
		if value <= 0.0031308 {
			value *= 12.92
		} else {
			value = 1.055*math.Pow(value, 1/2.4) - 0.055
		}
		return uint8(math.Round(value * 255))
	}
	return color.NRGBA{R: channel(foreground.R, background.R), G: channel(foreground.G, background.G), B: channel(foreground.B, background.B), A: 255}
}

func (v *view) resolvedTextOn(foreground, background color.NRGBA) color.NRGBA {
	return v.resolvedTextFor(v.textRole, foreground, background)
}

func (v *view) resolvedTextFor(role contrastRole, foreground, background color.NRGBA) color.NRGBA {
	return v.textContrast.resolve(foreground, background, role)
}

func (v *view) textAs(role contrastRole, content layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		previous := v.textRole
		v.textRole = role
		defer func() { v.textRole = previous }()
		return content(gtx)
	}
}

func (v *view) currentTextBackground() color.NRGBA {
	background := v.textBackground
	if background.A == 0 {
		background = v.appearance.Colors.Surface
	}
	return background
}

func (v *view) resolvedText(foreground color.NRGBA) color.NRGBA {
	return v.resolvedTextOn(foreground, v.currentTextBackground())
}

// Background scopes follow synchronous layout nesting, including selected
// controls inside bands. Siblings always resume their own surface context.
func (v *view) textOn(background color.NRGBA, content layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		previous := v.textBackground
		v.textBackground = background
		defer func() { v.textBackground = previous }()
		return content(gtx)
	}
}
