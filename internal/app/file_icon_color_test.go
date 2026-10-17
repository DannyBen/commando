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
	if got := adjustFileIcon(source, target, rgb(0xffffff), 100); !bytes.Equal(got.Pix, source.Pix) {
		t.Fatal("100 changed original pixels")
	}
	for _, surface := range []color.NRGBA{rgb(0xffffff), rgb(0x111111)} {
		half := adjustFileIcon(source, target, surface, 50)
		low := adjustFileIcon(source, target, surface, 0)
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
	first := v.adjustedFileIcon(icon, browser.Entry{Name: "one.txt"})
	second := v.adjustedFileIcon(icon, browser.Entry{Name: "two.txt"})
	if first != second || len(v.fileIconAdjusted) != 1 {
		t.Fatal("shared artwork and file colors repeated conversion")
	}
	v.adjustedFileIcon(icon, browser.Entry{Name: "two.png"})
	if len(v.fileIconAdjusted) != 2 {
		t.Fatal("different file color reused an incorrect image")
	}
	v.previewSettingValue(settingIconOriginalColor, settings.Value{Number: 40})
	v.adjustedFileIcon(icon, browser.Entry{Name: "one.txt"})
	if len(v.fileIconAdjusted) != 3 {
		t.Fatal("slider change retained an old adjustment")
	}
	v.appearance.Colors.Surface = rgb(0x222222)
	v.adjustedFileIcon(icon, browser.Entry{Name: "one.txt"})
	if len(v.fileIconAdjusted) != 4 {
		t.Fatal("surface change retained an old adjustment")
	}
	v.previewSettingValue(settingIconOriginalColor, settings.Value{Number: 100})
	if v.adjustedFileIcon(icon, browser.Entry{Name: "one.txt"}) != icon.image {
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
		adjustFileIcon(source, rgb(0x287acb), rgb(0xffffff), 30)
	}
}
