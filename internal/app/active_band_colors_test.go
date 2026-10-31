package app

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
)

func TestActiveBandOffsetsAreIndependentOfFileColorsAndEachOther(t *testing.T) {
	for _, theme := range builtInThemes {
		for _, enabled := range []bool{false, true} {
			v := newPreviewView()
			v.settingsValues = theme.values.Clone()
			v.previewSettingValue(settingAutoColorEnabled, settings.Value{Bool: enabled})
			v.followMe = true
			file := previewFile("notes.txt")
			fileBefore, _, _ := v.appearance.entryColors(file, true, false)
			header, _, _ := v.metadataHeaderColors(0)
			footer, _ := v.notificationColors(feedback.Info)
			v.previewSettingValue(settingSelectionLightness, settings.Value{Number: 0})
			fileAfter, _, _ := v.appearance.entryColors(file, true, false)
			if got, _, _ := v.metadataHeaderColors(0); got != header {
				t.Fatal("file selection offset changed a Follow me header")
			}
			if got, _ := v.notificationColors(feedback.Info); got != footer {
				t.Fatal("file selection offset changed a notification")
			}
			if enabled && fileBefore == fileAfter {
				t.Fatal("file selection offset stopped affecting file selections")
			}
			v.previewSettingValue(settingHeaderOffset, settings.Value{Number: 0})
			if got, _, _ := v.metadataHeaderColors(0); got == header {
				t.Fatal("header offset did not change the header")
			}
			if got, _ := v.notificationColors(feedback.Info); got != footer {
				t.Fatal("header offset changed a notification")
			}
			header, _, _ = v.metadataHeaderColors(0)
			v.previewSettingValue(settingFooterOffset, settings.Value{Number: 0})
			if got, _ := v.notificationColors(feedback.Info); got == footer {
				t.Fatal("footer offset did not change the notification")
			}
			if got, _, _ := v.metadataHeaderColors(0); got != header {
				t.Fatal("footer offset changed a header")
			}
			if got, _, _ := v.appearance.entryColors(file, true, false); got != fileAfter {
				t.Fatal("header/footer offset changed a file selection")
			}
		}
	}
}

func TestActiveBandWidgetsPreviewCancelSaveAndReset(t *testing.T) {
	for _, tc := range []struct{ group, key, label string }{
		{settingActiveHeader, settingHeaderOffset, "Active header colors"},
		{settingActiveFooter, settingFooterOffset, "Active footer colors"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			v := newPreviewView()
			dir := t.TempDir()
			v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
				return func() (string, error) { return "", settings.Save(dir, values) }
			}
			send, _ := panelInputFrames(v)
			send(key.EditEvent{Text: "@Theme " + tc.label})
			palette := v.paletteInteraction()
			if palette == nil || len(palette.results) == 0 || palette.results[0].command.id != "settings."+tc.group {
				t.Fatal("color setting was not discoverable by its agreed name")
			}
			send(pressed(key.NameReturn))
			s := v.settingsInteraction()
			if s == nil || s.title != "Theme › "+tc.label || len(s.fields) != 1 || s.fields[0].Label != "Background lightness offset" {
				t.Fatal("color dialog did not open with the agreed control")
			}
			preview := v.settingsPanelPreview(0)
			if (tc.group == settingActiveHeader) != preview.activeHeader || (tc.group == settingActiveFooter) != (len(preview.messages) == 4) {
				t.Fatal("color dialog has no matching passive preview")
			}
			if other := v.settingsPanelPreview(1); other.activeHeader || len(other.messages) > 0 {
				t.Fatal("color preview changed another panel")
			}
			p := &v.panels[0]
			selected, position := p.selected, p.list.Position
			p.feedback.ShowWithStyle(feedback.Warning, "Real warning", feedback.Immediate)
			message := p.feedback.Current()
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Constraints: layout.Exact(image.Pt(300, 30))}
			if preview.activeHeader {
				preview.header(v, gtx, 30)
			} else {
				preview.footer(v, gtx, 30)
			}
			if p.selected != selected || p.list.Position != position || p.feedback.Current() != message || v.settingsInteraction() != s {
				t.Fatal("band preview changed browsing, feedback or the settings session")
			}
			send(pressed(key.NameRightArrow))
			if v.settingText(tc.key) != "11" {
				t.Fatal("keyboard edit did not preview the offset")
			}
			send(pressed(key.NameEscape))
			send(pressed(key.NameReturn))
			if v.settingsInteraction() != nil || v.settingText(tc.key) != "10" {
				t.Fatal("Cancel did not restore the offset")
			}
			send(pressed(key.NameEscape))
			v.startSettingsWidget(tc.group)
			v.adjustSetting(tc.key, -1)
			v.finishSettingsWidget(layout.Context{}, true)
			loaded, err := settings.Load(dir, settings.CatalogueDefault)
			if err != nil || loaded.Get(settings.CatalogueDefault, tc.key).Number != 9 {
				t.Fatal("Save lost the edited offset", err)
			}
			v.restoreSettings(loaded, nil)
			v.startSettingsWidget(tc.group)
			v.activateSettingsAction(layout.Context{}, 2)
			if v.settingText(tc.key) != "10" {
				t.Fatal("Reset did not restore the default offset")
			}
			v.finishSettingsWidget(layout.Context{}, false)
			if v.settingText(tc.key) != "9" {
				t.Fatal("Cancel did not undo Reset")
			}
		})
	}
}

func TestLegacyThemesAndSettingsInheritMissingBandOffsetsOnce(t *testing.T) {
	for _, file := range []string{"light.toml", "dark.toml"} {
		data, err := themeFiles.ReadFile("themes/" + file)
		if err != nil {
			t.Fatal(err)
		}
		current, err := parseThemeSnapshot(data)
		if err != nil {
			t.Fatal(err)
		}
		legacy := string(data)
		for _, group := range []string{settingActiveHeader, settingActiveFooter} {
			legacy = strings.Replace(legacy, fmt.Sprintf("[%s]\nlightness_points = %g\n\n", group, current.Get(settings.CatalogueDefault, group+".lightness_points").Number), "", 1)
		}
		for _, explicit := range []bool{false, true} {
			source := legacy
			want := current.Get(settings.CatalogueDefault, settingSelectionLightness).Number
			wantHeader := want
			if explicit {
				source += "\n[" + settingActiveHeader + "]\nlightness_points = 7\n"
				wantHeader = 7
			}
			values, err := parseThemeSnapshot([]byte(source))
			if err != nil || values.Get(settings.CatalogueDefault, settingHeaderOffset).Number != wantHeader || values.Get(settings.CatalogueDefault, settingFooterOffset).Number != want {
				t.Fatal("legacy theme lost its appearance or explicit offset", file, err)
			}
			raw, err := settings.Parse([]byte(source), settings.CatalogueDefault)
			if err != nil {
				t.Fatal(err)
			}
			v := newPreviewView()
			v.restoreSettings(raw, nil)
			v.previewSettingValue(settingSelectionLightness, settings.Value{Number: 0})
			if v.settingsValues.Get(settings.CatalogueDefault, settingHeaderOffset).Number != wantHeader || v.settingsValues.Get(settings.CatalogueDefault, settingFooterOffset).Number != want {
				t.Fatal("legacy settings kept a live link to file colors")
			}
			encoded, err := encodeThemeSnapshot(values)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := parseThemeSnapshot(encoded)
			if err != nil || !(themeSnapshot{values: values}).matches(restored) || !strings.Contains(string(encoded), "["+settingActiveHeader+"]") || !strings.Contains(string(encoded), "["+settingActiveFooter+"]") {
				t.Fatal("saving a legacy theme did not publish independent offsets", err)
			}
		}
	}
}

func TestGeneratorCoordinatesActiveBandOffsets(t *testing.T) {
	for mode, recipe := range defaultThemeRecipe.Modes {
		for _, seed := range []float64{1, 600} {
			v := newPreviewView()
			_ = v.settingsValues.Set(settings.CatalogueDefault, settingThemeGenerator+".mode", settings.Value{Text: mode})
			_ = v.settingsValues.Set(settings.CatalogueDefault, settingThemeGenerator+".seed", settings.Value{Number: seed})
			v.generateTheme()
			v.applySettings()
			if v.settingsValues.Get(settings.CatalogueDefault, settingHeaderOffset).Number != recipe.ActiveBandOffset || v.settingsValues.Get(settings.CatalogueDefault, settingFooterOffset).Number != recipe.ActiveBandOffset {
				t.Fatal("generator did not coordinate the header/footer offsets", mode)
			}
			for _, kind := range []feedback.Kind{feedback.Info, feedback.Success, feedback.Warning, feedback.Error} {
				header, _ := v.activeBandColors(v.feedbackColor(kind), settingHeaderOffset)
				footer, _ := v.notificationColors(kind)
				if header != footer {
					t.Fatal("generator introduced different header/footer colors", mode)
				}
			}
		}
	}
	for _, bad := range []string{"101", "nan"} {
		if _, err := parseThemeRecipe(strings.Replace(themeGeneratorTOML, "active_band_offset = -16", "active_band_offset = "+bad, 1)); err == nil {
			t.Fatal("accepted an invalid generated band offset", bad)
		}
	}
}

func TestActiveBandColorPreviews(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_ACTIVE_BANDS")
	if dir == "" {
		t.Skip("optional active header/footer color previews")
	}
	for _, theme := range builtInThemes {
		for _, group := range []string{settingActiveHeader, settingActiveFooter} {
			v := newPreviewView()
			v.settingsValues = theme.values.Clone()
			v.applySettings()
			v.startSettingsWidget(group)
			if err := renderViewPNG(filepath.Join(dir, theme.name+"-"+strings.TrimPrefix(group, "appearance.theme.")+".png"), v); err != nil {
				t.Fatal(err)
			}
		}
	}
}
