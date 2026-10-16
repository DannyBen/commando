package app

import (
	"image/color"
	"math"
	"testing"

	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
)

func TestDotfileSettingsAffectConfigAndCombineWithHidden(t *testing.T) {
	for _, name := range []string{".config", ".dotfile", ".dotfile.txt"} {
		v := newPreviewView()
		v.settingsValues = builtInThemes[0].values.Clone()
		for key, n := range map[string]float64{
			settingLightness: 40, settingSaturation: 80,
			settingHiddenLightness: 5, settingHiddenSaturation: 0.5,
			settingDotfileLightness: 0, settingDotfileSaturation: 1,
		} {
			if err := v.settingsValues.Set(settings.CatalogueDefault, key, settings.Value{Number: n}); err != nil {
				t.Fatal(err)
			}
		}
		v.applySettings()
		file := browser.Entry{Name: name}
		before := v.appearance.FilenameColors.filename(file)
		displayed := func() color.NRGBA {
			bg, fg, _ := v.appearance.entryColors(file, false, false)
			return v.resolvedTextFor(v.appearance.entryTextRoles(file, false, false).filename, fg, bg)
		}
		beforeDisplay := displayed()
		v.startSettingsWidget("appearance.autocolor.special_files")
		if err := v.setWidgetValue(settingDotfileLightness, settings.Value{Number: 10}); err != nil {
			t.Fatal(err)
		}
		v.applySettings()
		after := v.appearance.FilenameColors.filename(file)
		if math.Abs(after.l-before.l-0.1) > 1e-9 || displayed() == beforeDisplay {
			t.Fatal("dotfile lightness does not visibly affect", name)
		}
		beforeDisplay = displayed()
		if err := v.setWidgetValue(settingDotfileSaturation, settings.Value{Number: 0.25}); err != nil {
			t.Fatal(err)
		}
		v.applySettings()
		dot := v.appearance.FilenameColors.filename(file)
		dotLSH := lshFromOKLCH(dot)
		if math.Abs(dotLSH.s-20) > 1e-7 || displayed() == beforeDisplay {
			t.Fatal("dotfile saturation does not visibly affect", name)
		}
		file.Hidden = true
		combined := lshFromOKLCH(v.appearance.FilenameColors.filename(file))
		if math.Abs(combined.l-55) > 1e-7 || math.Abs(combined.s-10) > 1e-7 || combined.h != dotLSH.h {
			t.Fatal("hidden dotfile did not add offsets and multiply saturation", name, combined)
		}
		ordinary := v.appearance.FilenameColors.filename(browser.Entry{Name: "normal-file.txt"})
		ordinaryLSH := lshFromOKLCH(ordinary)
		if math.Abs(ordinaryLSH.l-40) > 1e-7 || math.Abs(ordinaryLSH.s-80) > 1e-7 {
			t.Fatal("dotfile controls changed an ordinary file")
		}
	}
}
