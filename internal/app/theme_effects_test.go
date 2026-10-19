package app

import (
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
)

func TestEffectsLivePreviewCancelSaveAndReset(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	before := v.appearance
	v.startSettingsWidget(settingThemeEffects)
	if s := v.settingsInteraction(); s == nil || len(s.fields) != 3 || s.title != "Theme › Effects" {
		t.Fatal("effects did not open as a three-field widget")
	}
	change := func() {
		v.previewSettingValue(settingDimmedText, settings.Value{Number: 40})
		v.previewSettingValue(settingHover, settings.Value{Number: 25})
		v.previewSettingValue(settingBackdrop, settings.Value{Number: 60})
	}
	change()
	if v.appearance.DimmedTextOpacity != .4 || v.appearance.HoverStrength != .25 || math.Abs(float64(v.appearance.OverlayBackdropOpacity)-.6) > 1e-6 {
		t.Fatal("effects did not apply live")
	}
	samples := v.settingsPreviewItems("effects")
	if len(samples) != 2 || samples[0].foreground.A != 102 || samples[1].background == v.appearance.Colors.Surface {
		t.Fatal("live samples do not reflect dimming and hover", samples)
	}
	v.finishSettingsWidget(layout.Context{}, false)
	if v.appearance.DimmedTextOpacity != before.DimmedTextOpacity || v.appearance.HoverStrength != before.HoverStrength || v.appearance.OverlayBackdropOpacity != before.OverlayBackdropOpacity {
		t.Fatal("Cancel did not restore the effects")
	}
	dir := t.TempDir()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	v.startSettingsWidget(settingThemeEffects)
	change()
	v.finishSettingsWidget(layout.Context{}, true)
	loaded, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil || loaded.Get(settings.CatalogueDefault, settingDimmedText).Number != 40 || loaded.Get(settings.CatalogueDefault, settingHover).Number != 25 || loaded.Get(settings.CatalogueDefault, settingBackdrop).Number != 60 {
		t.Fatal("Save did not persist the effects", err)
	}
	v.startSettingsWidget(settingThemeEffects)
	v.activateSettingsAction(layout.Context{}, 2)
	if v.appearance.DimmedTextOpacity != .7 || v.appearance.HoverStrength != .08 || v.appearance.OverlayBackdropOpacity != .15 {
		t.Fatal("Reset did not preview the default effects")
	}
	v.finishSettingsWidget(layout.Context{}, false)
	if v.appearance.DimmedTextOpacity != .4 || v.appearance.HoverStrength != .25 || math.Abs(float64(v.appearance.OverlayBackdropOpacity)-.6) > 1e-6 {
		t.Fatal("Cancel after Reset did not restore the saved effects")
	}
}

func TestDimmedTextHasSeparateContrastAndOpacity(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	for _, background := range []color.NRGBA{rgb(0xffffff), rgb(0), rgb(0x808080)} {
		foreground := rgb(0)
		if background == rgb(0) {
			foreground = rgb(0xffffff)
		}
		v.appearance.Colors.EditorText = foreground
		v.textBackground = background
		hint := v.hintColor()
		normal := v.resolvedTextFor(contrastPrimary, foreground, background)
		score := math.Abs(perceptualContrast(compositeText(hint, background), background))
		if score < themeContrastRules[contrastDimmedText].targetLc || hint == normal {
			t.Fatal("dimmed text lost readability or became primary text", background, hint, score)
		}
	}
	_ = v.settingsValues.Set(settings.CatalogueDefault, settingContrastGuard, settings.Value{Bool: false})
	v.appearance.DimmedTextOpacity = .4
	if v.hintColor().A != 102 || v.textEditor(&v.nameEditor, "File name").HintColor != v.hintColor() {
		t.Fatal("placeholder did not share the configured opacity")
	}
	secondary := v.resolvedTextFor(contrastSecondary, v.appearance.Colors.Muted, v.currentTextBackground())
	v.appearance.DimmedTextOpacity = 0
	if v.hintColor().A != 0 || v.resolvedTextFor(contrastSecondary, v.appearance.Colors.Muted, v.currentTextBackground()) != secondary {
		t.Fatal("dimming affected ordinary Secondary text")
	}
}

func TestUnavailablePaletteCommandsDimSelectedAndUnselectedText(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	v.panels[0].drives = true
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	send(key.EditEvent{Text: "New"})
	if s := v.paletteInteraction(); s == nil || len(s.results) < 2 || s.results[0].command.allowed(v) || s.results[1].command.allowed(v) {
		t.Fatal("fixture did not show two unavailable creation commands")
	}
	c := v.appearance.Colors
	for _, pair := range []struct{ fg, bg color.NRGBA }{{c.SelectedName, c.Selected}, {c.Foreground, c.Surface}} {
		key := textContrastPair{v.dimmedColor(pair.fg), pair.bg, themeContrastRules[contrastDimmedText], color.NRGBA{}}
		if _, ok := v.textContrast[key]; !ok {
			t.Fatal("palette did not apply de-emphasis on this surface", pair.bg)
		}
	}
	send(pressed(key.NameReturn))
	if v.paletteInteraction() == nil {
		t.Fatal("visual dimming allowed an unavailable action to execute")
	}
}

func TestHoverStrengthRetainsPressAndDisabledStyling(t *testing.T) {
	for _, base := range []color.NRGBA{rgb(0), rgb(0xffffff), rgb(0x6688aa)} {
		hover := targetState{Enabled: true, Hovered: true}
		if hover.background(base, 0) != base || hover.background(base, 1) == base {
			t.Fatal("hover strength endpoints did not affect the background")
		}
		for _, state := range []targetState{{Enabled: false, Hovered: true}, {Enabled: true}, {Enabled: true, Hovered: true, Pressed: true}} {
			if state.background(base, 1) != base {
				t.Fatal("hover strength changed disabled, idle or pressed styling")
			}
		}
	}
}

func TestUnfocusedSettingsChoicesShareDimmedTextOpacity(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	v.startSettingsWidget("guards.recycle_confirmation")
	panelInputFrames(v)
	c := v.appearance.Colors
	pair := textContrastPair{v.dimmedColor(c.Muted), c.Surface, themeContrastRules[contrastDimmedText], color.NRGBA{}}
	if _, ok := v.textContrast[pair]; !ok {
		t.Fatal("unchosen options in unfocused settings rows did not use shared de-emphasis")
	}
}

func TestEffectsIncludedInThemeSnapshots(t *testing.T) {
	values := settings.Values{}
	for key, number := range map[string]float64{settingDimmedText: 45, settingHover: 30, settingBackdrop: 55} {
		if err := values.Set(settings.CatalogueDefault, key, settings.Value{Number: number}); err != nil {
			t.Fatal(err)
		}
	}
	data, err := encodeThemeSnapshot(values)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseThemeSnapshot(data)
	if err != nil || !(themeSnapshot{values: parsed}).matches(values) {
		t.Fatal("theme snapshot lost its effects", err)
	}
	for _, key := range []string{settingDimmedText, settingHover, settingBackdrop} {
		if !parsed.Overridden(key) {
			t.Fatal("theme snapshot omitted an effect", key)
		}
	}
	if parsed.Overridden(settingContrastGuard) {
		t.Fatal("theme snapshot included the user's contrast preference")
	}
}

func TestEffectsPreviews(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_EFFECTS")
	if dir == "" {
		t.Skip("optional effects previews")
	}
	for _, theme := range builtInThemes {
		v := newPreviewView()
		v.settingsValues = theme.values.Clone()
		v.applySettings()
		v.startSettingsWidget(settingThemeEffects)
		if err := renderViewPNG(filepath.Join(dir, theme.name+"-effects.png"), v); err != nil {
			t.Fatal(err)
		}
		v.finishSettingsWidget(layout.Context{}, false)
		v.panels[0].archiveFolder, v.panels[0].path = ".", `C:\Downloads\archive.zip`
		send, _ := panelInputFrames(v)
		send(pressed("`"))
		send(key.EditEvent{Text: "New"})
		if err := renderViewPNG(filepath.Join(dir, theme.name+"-unavailable.png"), v); err != nil {
			t.Fatal(err)
		}
		send(pressed(key.NameEscape))
		v.startSettingsWidget("guards.recycle_confirmation")
		if err := renderViewPNG(filepath.Join(dir, theme.name+"-choices.png"), v); err != nil {
			t.Fatal(err)
		}
	}
}
