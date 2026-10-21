package app

import (
	"hash/fnv"
	"image/color"
	"math"
	"math/rand/v2"
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

func (s filenameColorSettings) filename(file browser.Entry) oklchColor {
	return s.specialFileColor(file, s.baseFilename(file)).oklch()
}

func (s filenameColorSettings) baseFilename(file browser.Entry) lshColor {
	// Seed a local PCG from the extension hash plus salt, then take one hue.
	// Without shared random state, colors stay independent of listing,
	// platform and encounter order.
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(filenameColorKey(file)))
	seed := math.Float64bits(float64(hash.Sum32()) + s.salt)
	random := rand.New(rand.NewPCG(seed, 0))
	return lshColor{s.filenameLightness * 100, s.filenameSaturation * 100,
		random.Float64() * 360}
}

func (s filenameColorSettings) specialFileColor(file browser.Entry, value lshColor) lshColor {
	if file.Hidden && !file.Parent {
		value.l += s.hiddenLightnessOffset * 100
		value.s *= s.hiddenSaturationMultiplier
	}
	if strings.HasPrefix(file.Name, ".") && !file.Parent {
		value.l += s.dotfileLightnessOffset * 100
		value.s *= s.dotfileSaturationMultiplier
	}
	return value
}

func (s filenameColorSettings) activeBackground(value oklchColor) oklchColor {
	value.l = clampLightness(value.l + s.activeBackgroundLightnessOffset)
	return value
}

func (t Theme) filenameSelectionColors(value oklchColor) (background, text color.NRGBA) {
	colors := defaultFilenameColors
	if t.FilenameColors != nil {
		colors = *t.FilenameColors
	}
	return colors.activeBackground(value).rgb(), t.Colors.Surface
}

func (s filenameColorSettings) cutColor(value oklchColor) oklchColor {
	value.l = clampLightness(value.l + s.cutLightnessOffset)
	return value
}

// Shared row styling keeps directory/drive and cut precedence in one place.
func (t Theme) entryColors(file browser.Entry, active, cut bool) (background, filename, metadata color.NRGBA) {
	background, filename = t.Colors.Surface, t.Colors.Filename
	colors := defaultFilenameColors
	if t.FilenameColors != nil {
		colors = *t.FilenameColors
	}
	generated := colors.enabled && file.Path == ""
	value := oklchColor{}
	if generated {
		value = colors.filename(file)
		filename = value.rgb()
	}
	if cut {
		if !generated {
			value = oklchFromRGB(filename)
		}
		value = colors.cutColor(value)
		filename = value.rgb()
	}
	metadata = t.Colors.Metadata
	if generated || cut {
		metadata = filename
	}
	if active {
		if generated {
			background, filename = t.filenameSelectionColors(value)
			metadata = filename
		} else {
			background, filename, metadata = t.Colors.Selected, t.Colors.SelectedName, t.Colors.SelectedMetadata
		}
	}
	return
}

// File text roles keep filenames and matching metadata under one policy.
// Only actual dimming states get the weaker floor; selected rows remain primary.
type fileTextRoles struct{ filename, metadata contrastRole }

func (t Theme) entryTextRoles(file browser.Entry, active, cut bool) fileTextRoles {
	roles := fileTextRoles{contrastPrimary, contrastSecondary}
	colors := defaultFilenameColors
	if t.FilenameColors != nil {
		colors = *t.FilenameColors
	}
	generated := colors.enabled && file.Path == ""
	if !active && !file.Parent && file.Path == "" && (cut || generated && (file.Hidden || strings.HasPrefix(file.Name, "."))) {
		return fileTextRoles{contrastDimmedFile, contrastDimmedFile}
	}
	if active || cut || generated {
		roles.metadata = contrastPrimary
	}
	return roles
}
