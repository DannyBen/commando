package app

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"gioui.org/op/paint"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
)

type fileIconAdjustment struct {
	source        image.Image // Windows supplies immutable images shared by icon identity.
	file, surface color.NRGBA
	original      float64
}

func (v *view) adjustedFileIcon(icon fileIconEntry, file browser.Entry) paint.ImageOp {
	original := v.settingsValues.Get(settings.CatalogueDefault, settingIconOriginalColor).Number
	if original >= 100 {
		return icon.image
	}
	_, target, _ := v.appearance.entryColors(file, false, false)
	key := fileIconAdjustment{icon.source, target, v.appearance.Colors.Surface, original}
	if v.fileIconAdjusted == nil {
		v.fileIconAdjusted = make(map[fileIconAdjustment]fileIconEntry)
	}
	v.fileIconClock++
	if cached, found := v.fileIconAdjusted[key]; found {
		cached.used = v.fileIconClock
		v.fileIconAdjusted[key] = cached
		return cached.image
	}
	img := adjustFileIcon(icon.source, target, key.surface, original)
	op := paint.NewImageOp(img)
	if len(v.fileIconAdjusted) >= 256 {
		var oldest fileIconAdjustment
		var oldestUse uint64 = ^uint64(0)
		for key, cached := range v.fileIconAdjusted {
			if cached.used < oldestUse {
				oldest, oldestUse = key, cached.used
			}
		}
		delete(v.fileIconAdjusted, oldest)
	}
	v.fileIconAdjusted[key] = fileIconEntry{image: op, used: v.fileIconClock}
	return op
}

// Blend chromatic components in Oklab so neutral pixels and opposing hues
// transition smoothly. Keep the lightness detail, gently reducing contrast
// against the list surface only in the lower half of the control.
func adjustFileIcon(source image.Image, file, surface color.NRGBA, original float64) *image.RGBA {
	amount := max(0, min(1, original/100))
	tint := 0.9 * (1 - amount)
	dim := 0.22 * math.Pow(max(0, 1-2*amount), 2)
	target := oklchFromRGB(file)
	background := oklchFromRGB(surface).l
	hue := target.h * math.Pi / 180
	ta, tb := math.Cos(hue), math.Sin(hue)
	output := image.NewRGBA(source.Bounds())
	if amount == 1 {
		draw.Draw(output, output.Bounds(), source, source.Bounds().Min, draw.Src)
		return output
	}
	rgba, _ := source.(*image.RGBA)
	for y := output.Rect.Min.Y; y < output.Rect.Max.Y; y++ {
		for x := output.Rect.Min.X; x < output.Rect.Max.X; x++ {
			var pixel color.NRGBA
			if rgba != nil {
				p := rgba.RGBAAt(x, y)
				if p.A == 0 {
					continue
				}
				pixel = color.NRGBA{iconUnpremultiply(p.R, p.A), iconUnpremultiply(p.G, p.A), iconUnpremultiply(p.B, p.A), p.A}
			} else {
				pixel = color.NRGBAModel.Convert(source.At(x, y)).(color.NRGBA)
			}
			if pixel.A == 0 {
				continue
			}
			value := oklchFromRGB(pixel)
			angle := value.h * math.Pi / 180
			// Protect white highlights and dark details from strong color casts.
			chroma := target.c * 0.55 * math.Sqrt(max(0, 4*value.l*(1-value.l)))
			a := value.c*math.Cos(angle)*(1-tint) + chroma*ta*tint
			b := value.c*math.Sin(angle)*(1-tint) + chroma*tb*tint
			value.l += (background - value.l) * dim
			value.c, value.h = math.Hypot(a, b), wrapHue(math.Atan2(b, a)*180/math.Pi)
			adjusted := value.rgb()
			a8 := uint32(pixel.A)
			output.SetRGBA(x, y, color.RGBA{uint8((uint32(adjusted.R)*a8 + 127) / 255), uint8((uint32(adjusted.G)*a8 + 127) / 255), uint8((uint32(adjusted.B)*a8 + 127) / 255), pixel.A})
		}
	}
	return output
}

func iconUnpremultiply(channel, alpha uint8) uint8 {
	return uint8(min(255, (uint32(channel)*255+uint32(alpha)/2)/uint32(alpha)))
}
