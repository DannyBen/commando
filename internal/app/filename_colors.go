package app

import (
	"hash/fnv"
	"image/color"
	"math"
	"strings"

	"github.com/dannyben/commando/internal/browser"
)

// Category keys have a separate namespace from actual extensions.
func filenameColorKey(file browser.Entry) string {
	if file.Directory || file.Parent {
		return "category:._dir"
	}
	if dot := strings.LastIndexByte(file.Name, '.'); dot > 0 && dot < len(file.Name)-1 {
		return "extension:" + strings.ToLower(file.Name[dot:])
	}
	if strings.HasPrefix(file.Name, ".") {
		return "category:._extless_dotfile"
	}
	return "category:._extless"
}

type oklchColor struct{ l, c, h float64 }

func (s filenameColorSettings) filename(file browser.Entry) oklchColor {
	// FNV-1a 32-bit over UTF-8, mapped uniformly to [0,360). This mapping
	// never depends on the listing, platform, or encounter order.
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(filenameColorKey(file)))
	value := oklchColor{s.filenameLightness, s.filenameChroma,
		wrapHue(float64(hash.Sum32())/4294967296*360 + s.hueSeed)}
	if file.Hidden && !file.Parent {
		value.l += s.hiddenLightnessOffset
		value.c *= s.hiddenChromaMultiplier
	}
	if strings.HasPrefix(file.Name, ".") && !file.Parent {
		value.l += s.dotfileLightnessOffset
		value.c *= s.dotfileChromaMultiplier
	}
	value.l, value.c = clampLightness(value.l), max(0, value.c)
	return value
}

func (s filenameColorSettings) activeBackground(value oklchColor) oklchColor {
	value.l = clampLightness(value.l + s.activeBackgroundLightnessOffset)
	return value
}

func clampLightness(l float64) float64 { return max(0, min(1, l)) }

func wrapHue(h float64) float64 { return math.Mod(math.Mod(h, 360)+360, 360) }

// Convert OKLCH to linear sRGB using the combined Oklab/LMS/sRGB matrices,
// then apply the sRGB transfer function. Reference: https://www.w3.org/TR/css-color/
func (value oklchColor) linearRGB() [3]float64 {
	a, b := value.c*math.Cos(value.h*math.Pi/180), value.c*math.Sin(value.h*math.Pi/180)
	l := value.l + 0.3963377773761749*a + 0.2158037573099136*b
	m := value.l - 0.1055613458156586*a - 0.0638541728258133*b
	s := value.l - 0.0894841775298119*a - 1.2914855480194092*b
	l, m, s = l*l*l, m*m*m, s*s*s
	return [3]float64{
		4.0767416621*l - 3.3077115913*m + 0.2309699292*s,
		-1.2684380046*l + 2.6097574011*m - 0.3413193965*s,
		-0.0041960863*l - 0.7034186147*m + 1.7076147010*s,
	}
}

func inSRGB(rgb [3]float64) bool {
	for _, channel := range rgb {
		if channel < -1e-7 || channel > 1+1e-7 {
			return false
		}
	}
	return true
}

// Binary search reduces only chroma to fit sRGB; channel clamping below
// handles floating-point rounding at the boundary, rather than gamut mapping.
func (value oklchColor) gamutMapped() oklchColor {
	value.l, value.c = clampLightness(value.l), max(0, value.c)
	if inSRGB(value.linearRGB()) {
		return value
	}
	low, high := 0.0, value.c
	for range 28 {
		value.c = (low + high) / 2
		if inSRGB(value.linearRGB()) {
			low = value.c
		} else {
			high = value.c
		}
	}
	value.c = low
	return value
}

func (value oklchColor) rgb() color.NRGBA {
	rgb := value.gamutMapped().linearRGB()
	encode := func(channel float64) uint8 {
		channel = max(0, min(1, channel))
		if channel <= 0.0031308 {
			channel *= 12.92
		} else {
			channel = 1.055*math.Pow(channel, 1/2.4) - 0.055
		}
		return uint8(math.Round(channel * 255))
	}
	return color.NRGBA{R: encode(rgb[0]), G: encode(rgb[1]), B: encode(rgb[2]), A: 255}
}

// Shared row styling keeps directory/drive and cut precedence in one place.
func (t Theme) entryColors(file browser.Entry, active, cut bool) (background, filename, metadata color.NRGBA) {
	background, filename = t.Colors.Surface, t.Colors.Filename
	colors := defaultFilenameColors
	if t.FilenameColors != nil {
		colors = *t.FilenameColors
	}
	generated := colors.enabled && file.Path == ""
	if generated {
		filename = colors.filename(file).rgb()
	}
	if active {
		background = t.Colors.Selected
		if generated {
			background = colors.activeBackground(colors.filename(file)).rgb()
			filename = t.Colors.Surface
		} else {
			filename = t.Colors.SelectedName
		}
	}
	if cut {
		filename = t.Colors.CutFilename
		if active {
			filename = t.Colors.SelectedCutFilename
		}
	}
	metadata = t.Colors.Metadata
	if active {
		metadata = t.Colors.SelectedMetadata
	}
	if generated || cut {
		metadata = filename
	}
	return
}
