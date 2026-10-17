package app

import (
	"image"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"github.com/dannyben/commando/internal/settings"
	"golang.org/x/image/math/fixed"
)

func TestBundledVictorMonoWeightsWithoutSystemFonts(t *testing.T) {
	faces := defaultTheme.Font
	if len(faces) != 7 || faces[0].Font.Typeface != "Victor Mono" || faces[0].Font.Weight != font.Normal {
		t.Fatal("bundle must contain seven Victor Mono weights, with Regular first")
	}
	shaper := text.NewShaper(text.NoSystemFonts(), text.WithCollection(faces))
	seen := make(map[text.GlyphID]bool)
	for weight := 100; weight <= 700; weight += 100 {
		shaper.LayoutString(text.Parameters{Font: font.Font{Typeface: "Victor Mono", Weight: font.Weight(weight - 400)},
			PxPerEm: fixed.I(15), MaxWidth: 1000}, "A")
		glyph, ok := shaper.NextGlyph()
		if !ok || seen[glyph.ID] {
			t.Fatalf("bundled weight %d did not render its own face", weight)
		}
		seen[glyph.ID] = true
	}
}

func TestFontWidgetInputPreviewCancelResetAndPersistence(t *testing.T) {
	v := newPreviewView()
	before := v.appearance
	v.startSettingsWidget("appearance.font")
	s := v.settingsInteraction()
	if s == nil || s.title != "Appearance › Font" || len(s.fields) != 5 {
		t.Fatal("font settings did not open as one five-field widget")
	}
	send, _ := panelInputFrames(v)
	send(pressed(key.NameReturn))
	send(key.EditEvent{Range: key.Range{Start: 0, End: len(before.FontFamily)}, Text: "Arial"})
	send(pressed(key.NameReturn))
	if s.editing || v.appearance.FontFamily != "Arial" || v.theme.Face == font.Typeface(before.FontFamily) {
		t.Fatal("typed family was not applied live")
	}
	v.adjustSetting(settingFontTextSize, 1)
	v.adjustSetting(settingFontSmallTextSize, 1)
	v.adjustSetting(settingFontWeight, 1)
	v.adjustSetting(settingFontBoldWeight, -1)
	if v.appearance.TextSize != 16 || v.appearance.SmallTextSize != 13 || v.appearance.FontWeight != font.Bold || v.appearance.BoldWeight != font.Medium ||
		v.textEditor(&widget.Editor{}, "").Font.Weight != font.Bold || v.appearance.RowPaddingVertical != before.RowPaddingVertical {
		t.Fatal("font settings did not preview together or changed geometry")
	}
	v.finishSettingsWidget(layout.Context{}, false)
	if v.appearance.FontFamily != before.FontFamily || v.appearance.TextSize != before.TextSize ||
		v.appearance.SmallTextSize != before.SmallTextSize || v.appearance.FontWeight != before.FontWeight || v.appearance.BoldWeight != before.BoldWeight {
		t.Fatal("Cancel did not restore the font")
	}

	v.startSettingsWidget("appearance.font")
	_ = v.setWidgetValue(settingFontFamily, settings.Value{Text: "Arial"})
	v.adjustSetting(settingFontTextSize, 2)
	v.adjustSetting(settingFontWeight, 1)
	v.adjustSetting(settingFontBoldWeight, -1)
	dir := t.TempDir()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	v.finishSettingsWidget(layout.Context{}, true)
	values, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil || !v.interaction.idle() {
		t.Fatal("font settings did not save", err)
	}
	restored := newPreviewView()
	restored.restoreSettings(values, nil)
	if restored.appearance.FontFamily != "Arial" || restored.appearance.TextSize != 17 || restored.appearance.FontWeight != font.Bold || restored.appearance.BoldWeight != font.Medium {
		t.Fatal("font settings did not apply on startup")
	}
	restored.startSettingsWidget("appearance.font")
	restored.activateSettingsAction(layout.Context{}, 2)
	if restored.appearance.FontFamily != before.FontFamily || restored.appearance.TextSize != before.TextSize || restored.appearance.FontWeight != before.FontWeight || restored.appearance.BoldWeight != before.BoldWeight {
		t.Fatal("Reset did not restore bundled defaults")
	}
	restored.finishSettingsWidget(layout.Context{}, false)
	if restored.appearance.FontFamily != "Arial" || restored.appearance.TextSize != 17 || restored.appearance.FontWeight != font.Bold || restored.appearance.BoldWeight != font.Medium {
		t.Fatal("Cancel did not undo Reset")
	}
}

func TestUnavailableFontUsesBundledMetricsAndSizeRemainsLive(t *testing.T) {
	v := newPreviewView()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(1000, 1000)}}
	width := v.textWidth(gtx, "Commando file 123.txt", v.appearance.TextSize)
	height := v.textHeight(gtx, v.appearance.TextSize)
	v.startSettingsWidget("appearance.font")
	for _, family := range []string{"Commando missing family 918273", "", "  Victor Mono  "} {
		if err := v.setWidgetValue(settingFontFamily, settings.Value{Text: family}); err != nil {
			t.Fatal(err)
		}
		v.applySettings()
		if got := v.textWidth(gtx, "Commando file 123.txt", v.appearance.TextSize); got != width {
			t.Fatalf("unavailable/bundled family %q changed metrics: %d, want %d", family, got, width)
		}
	}
	shaper := v.theme.Shaper
	for range 10 {
		v.adjustSetting(settingFontTextSize, 1)
	}
	if v.textHeight(gtx, v.appearance.TextSize) <= height || v.theme.Shaper != shaper {
		t.Fatal("size did not change measured geometry or unnecessarily rebuilt the shaper")
	}
	for _, tc := range []struct {
		key string
		n   float64
	}{{settingFontTextSize, 0}, {settingFontSmallTextSize, 100}, {settingFontWeight, 1000}, {settingFontBoldWeight, 1000}} {
		if err := v.setWidgetValue(tc.key, settings.Value{Number: tc.n}); err == nil {
			t.Fatalf("invalid %s accepted", tc.key)
		}
	}
}

func TestFontWidgetPreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional font widget preview")
	}
	v := newPreviewView()
	v.startSettingsWidget("appearance.font")
	v.adjustSetting(settingFontBoldWeight, 1)
	if err := renderViewPNG(filepath.Join(dir, "settings-font.png"), v); err != nil {
		t.Fatal(err)
	}
	s := v.settingsInteraction()
	s.completion.candidates = []string{"Victor Mono", "Victor Sans"}
	s.editing = true
	s.editor.SetText("Vic")
	s.editor.SetCaret(3, 3)
	if err := renderViewPNG(filepath.Join(dir, "settings-font-completion.png"), v); err != nil {
		t.Fatal(err)
	}
}
