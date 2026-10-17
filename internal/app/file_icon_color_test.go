package app

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"testing"

	"gioui.org/op/paint"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
)

func TestFileIconColorPreservesOriginalAlphaAndShading(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 4, 1))
	for i, pixel := range []color.NRGBA{{128, 128, 128, 255}, {180, 80, 40, 96}, {240, 240, 240, 255}, {0, 0, 0, 0}} {
		source.Set(i, 0, pixel)
	}
	before := append([]byte(nil), source.Pix...)
	target := rgb(0x287acb)
	if got := adjustFileIcon(source, target, rgb(0xffffff), 100, fileIconState{saturation: 1}); !bytes.Equal(got.Pix, source.Pix) {
		t.Fatal("100 changed original pixels")
	}
	for _, surface := range []color.NRGBA{rgb(0xffffff), rgb(0x111111)} {
		half := adjustFileIcon(source, target, surface, 50, fileIconState{saturation: 1})
		low := adjustFileIcon(source, target, surface, 0, fileIconState{saturation: 1})
		for x := range 4 {
			if half.RGBAAt(x, 0).A != source.RGBAAt(x, 0).A || low.RGBAAt(x, 0).A != source.RGBAAt(x, 0).A {
				t.Fatal("adjustment changed transparency")
			}
		}
		originalL := oklchFromRGB(color.NRGBAModel.Convert(source.At(0, 0)).(color.NRGBA)).l
		halfL := oklchFromRGB(color.NRGBAModel.Convert(half.At(0, 0)).(color.NRGBA)).l
		lowColor := oklchFromRGB(color.NRGBAModel.Convert(low.At(0, 0)).(color.NRGBA))
		backgroundL := oklchFromRGB(surface).l
		if math.Abs(halfL-originalL) > 0.01 || math.Abs(lowColor.l-backgroundL) >= math.Abs(originalL-backgroundL) {
			t.Fatal("lightness did not stay original at 50 and become gentler at 0")
		}
		if distance := math.Abs(math.Remainder(lowColor.h-oklchFromRGB(target).h, 360)); distance > 3 {
			t.Fatal("neutral pixel did not take the file hue")
		}
		if low.RGBAAt(2, 0).R <= low.RGBAAt(0, 0).R {
			t.Fatal("adjustment flattened highlight detail")
		}
	}
	if !bytes.Equal(source.Pix, before) {
		t.Fatal("adjustment mutated the source")
	}
}

func TestFileIconAdjustmentCacheSharesArtworkAndTracksSettings(t *testing.T) {
	v := newPreviewView()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, rgb(0x906040))
	icon := fileIconEntry{source: img, image: paint.NewImageOp(img)}
	v.previewSettingValue(settingIconOriginalColor, settings.Value{Number: 30})
	first := v.adjustedFileIcon(icon, browser.Entry{Name: "one.txt"}, false)
	second := v.adjustedFileIcon(icon, browser.Entry{Name: "two.txt"}, false)
	if first != second || len(v.fileIconAdjusted) != 1 {
		t.Fatal("shared artwork and file colors repeated conversion")
	}
	v.adjustedFileIcon(icon, browser.Entry{Name: "two.png"}, false)
	if len(v.fileIconAdjusted) != 2 {
		t.Fatal("different file color reused an incorrect image")
	}
	v.previewSettingValue(settingIconOriginalColor, settings.Value{Number: 40})
	v.adjustedFileIcon(icon, browser.Entry{Name: "one.txt"}, false)
	if len(v.fileIconAdjusted) != 3 {
		t.Fatal("slider change retained an old adjustment")
	}
	v.appearance.Colors.Surface = rgb(0x222222)
	v.adjustedFileIcon(icon, browser.Entry{Name: "one.txt"}, false)
	if len(v.fileIconAdjusted) != 4 {
		t.Fatal("surface change retained an old adjustment")
	}
	v.previewSettingValue(settingIconOriginalColor, settings.Value{Number: 100})
	if v.adjustedFileIcon(icon, browser.Entry{Name: "one.txt"}, false) != icon.image {
		t.Fatal("100 did not reuse the untouched original")
	}
}

func BenchmarkFileIconAdjustment(b *testing.B) {
	source := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			source.Set(x, y, color.NRGBA{uint8(x * 8), uint8(y * 8), 120, 200})
		}
	}
	for b.Loop() {
		adjustFileIcon(source, rgb(0x287acb), rgb(0xffffff), 30, fileIconState{saturation: 1})
	}
}

func TestFileIconStateDimmingPreservesAlphaAndFollowsLightnessDirection(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 2, 1))
	source.Set(0, 0, color.NRGBA{128, 96, 80, 255})
	source.Set(1, 0, color.NRGBA{128, 96, 80, 96})
	initial := oklchFromRGB(color.NRGBAModel.Convert(source.At(0, 0)).(color.NRGBA))
	for _, direction := range []float64{-1, 1} {
		for _, amount := range []float64{80, 100} {
			state := fileIconState{lightness: direction * 0.09, saturation: 0.52}
			ordinary := adjustFileIcon(source, rgb(0x287acb), rgb(0xffffff), amount, fileIconState{saturation: 1})
			special := adjustFileIcon(source, rgb(0x287acb), rgb(0xffffff), amount, state)
			cut := state
			cut.cutLightness = direction * 0.20
			cutImage := adjustFileIcon(source, rgb(0x287acb), rgb(0xffffff), amount, cut)
			baseColor := oklchFromRGB(color.NRGBAModel.Convert(ordinary.At(0, 0)).(color.NRGBA))
			specialColor := oklchFromRGB(color.NRGBAModel.Convert(special.At(0, 0)).(color.NRGBA))
			cutColor := oklchFromRGB(color.NRGBAModel.Convert(cutImage.At(0, 0)).(color.NRGBA))
			if direction*(specialColor.l-initial.l) < 0.02 || direction*(cutColor.l-specialColor.l) < 0.04 {
				t.Fatal("hidden/dotfile and cut offsets did not follow their configured direction")
			}
			if lshFromOKLCH(specialColor).s >= lshFromOKLCH(baseColor).s {
				t.Fatal("hidden/dotfile saturation was not reduced")
			}
			for _, img := range []*image.RGBA{ordinary, special, cutImage} {
				if img.RGBAAt(1, 0).A != 96 {
					t.Fatal("file state changed transparency")
				}
			}
		}
	}
}

func TestFileIconStrongSpecialOffsetsPreserveShading(t *testing.T) {
	// Bright folder highlights and gray document shading must survive the
	// user's +25 dotfile / +30 hidden offsets, even with original color at 100.
	source := image.NewRGBA(image.Rect(0, 0, 4, 1))
	for x, pixel := range []color.NRGBA{rgb(0xffd65a), rgb(0xe5a820), rgb(0xdddddd), rgb(0x999999)} {
		source.Set(x, 0, pixel)
	}
	for _, direction := range []float64{-1, 1} {
		for _, amount := range []float64{80, 100} {
			for _, state := range []fileIconState{
				{lightness: direction * 0.25, saturation: 0.2},
				{lightness: direction * 0.55, saturation: 0.13, cutLightness: direction * 0.2},
			} {
				img := adjustFileIcon(source, rgb(0x287acb), rgb(0xffffff), amount, state)
				for x := range 4 {
					pixel := img.RGBAAt(x, 0)
					if pixel.R == pixel.G && pixel.G == pixel.B && (pixel.R == 255 || pixel.R == 0) {
						t.Fatalf("special-file adjustment clipped pixel %d: %+v", x, state)
					}
				}
				for _, pair := range [][2]int{{0, 1}, {2, 3}} {
					high := oklchFromRGB(color.NRGBAModel.Convert(img.At(pair[0], 0)).(color.NRGBA)).l
					low := oklchFromRGB(color.NRGBAModel.Convert(img.At(pair[1], 0)).(color.NRGBA)).l
					if high-low < 0.025 {
						t.Fatalf("special-file adjustment erased shading: %f, %f", high, low)
					}
				}
			}
		}
	}
}

func TestFileIconStateVariantsUsePreferencesAndDoNotLeakBetweenRows(t *testing.T) {
	for _, theme := range builtInThemes {
		v := newPreviewView()
		v.settingsValues = theme.values.Clone()
		v.applySettings()
		img := image.NewRGBA(image.Rect(0, 0, 1, 1))
		img.Set(0, 0, rgb(0x806050))
		icon := fileIconEntry{image: paint.NewImageOp(img), source: img}
		normal := browser.Entry{Name: "normal.txt"}
		special := browser.Entry{Name: ".normal.txt", Hidden: true}
		for _, amount := range []float64{80, 100} {
			v.previewSettingValue(settingIconOriginalColor, settings.Value{Number: amount})
			plain := v.adjustedFileIcon(icon, normal, false)
			dimmed := v.adjustedFileIcon(icon, special, false)
			cut := v.adjustedFileIcon(icon, special, true)
			if plain == dimmed || dimmed == cut || v.adjustedFileIcon(icon, normal, false) != plain || v.adjustedFileIcon(icon, special, false) != dimmed {
				t.Fatal("file state did not create independent, reusable icon variants")
			}
		}
		v.previewSettingValue(settingAutoColorEnabled, settings.Value{Bool: false})
		if v.adjustedFileIcon(icon, normal, false) != v.adjustedFileIcon(icon, special, false) {
			t.Fatal("special-file settings applied while filename Auto color was off")
		}
		if v.adjustedFileIcon(icon, normal, true) == v.adjustedFileIcon(icon, normal, false) {
			t.Fatal("cut dimming stopped working with Auto color off")
		}
	}
}
