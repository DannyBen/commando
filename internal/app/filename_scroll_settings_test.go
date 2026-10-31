package app

import (
	"reflect"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
)

func TestFilenameScrollSettingsSaveCancelResetAndIndependence(t *testing.T) {
	v := filenameScrollView()
	dir := t.TempDir()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	before := v.settingsValues.Clone()
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@Interface Animations Filenames"})
	palette := v.paletteInteraction()
	if palette == nil || len(palette.results) == 0 || palette.results[0].command.id != "settings."+settingFilenameScroll {
		t.Fatal("filename animations were not discoverable in settings search")
	}
	send(pressed(key.NameReturn))
	s := v.settingsInteraction()
	if s == nil || s.title != "Interface › Animations › Filenames" || len(s.fields) != 3 || s.fields[0].Label != "Enabled" || s.fields[1].Label != "Animation" || s.fields[2].Label != "Speed" {
		t.Fatal("filename animation controls are incorrect")
	}
	send(pressed(key.NameRightArrow), pressed(key.NameDownArrow), pressed(key.NameRightArrow), pressed(key.NameDownArrow), pressed(key.NameRightArrow))
	if v.settingText(settingFilenameScrollEnabled) != "false" || v.settingText(settingFilenameScrollModel) != "Ticker" || v.settingText(settingFilenameScrollSpeed) != "6" || v.panels[0].filenameScroll != (textMarquee{}) {
		t.Fatal("editing the controls failed or restarted animation behind settings")
	}
	if v.settingText(settingLocationScrollModel) != "Ping Pong" || v.settingText(settingLocationScrollSpeed) != "5" {
		t.Fatal("filename controls changed the location animation")
	}
	send(pressed(key.NameEscape))
	send(pressed(key.NameReturn))
	if !reflect.DeepEqual(v.settingsValues, before) {
		t.Fatal("Cancel did not restore filename animation settings")
	}
	send(pressed(key.NameEscape))
	v.startSettingsWidget(settingFilenameScroll)
	v.adjustSetting(settingFilenameScrollModel, 1)
	v.adjustSetting(settingFilenameScrollSpeed, 1)
	v.finishSettingsWidget(layout.Context{}, true)
	loaded, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	restored := filenameScrollView()
	restored.restoreSettings(loaded, nil)
	sendRestored, _ := panelInputFrames(restored)
	if !restored.panels[0].filenameScroll.ticker || restored.panels[0].filenameScroll.speed != 36 {
		t.Fatal("saved filename animation did not apply after startup")
	}
	restored.startSettingsWidget(settingFilenameScroll)
	sendRestored()
	for range 20 {
		restored.adjustSetting(settingFilenameScrollSpeed, 1)
	}
	if restored.settingText(settingFilenameScrollSpeed) != "10" {
		t.Fatal("speed did not stop at 10")
	}
	for range 20 {
		restored.adjustSetting(settingFilenameScrollSpeed, -1)
	}
	if restored.settingText(settingFilenameScrollSpeed) != "1" {
		t.Fatal("speed did not stop at 1")
	}
	for _, n := range []float64{0, 11, 1.5} {
		if err := restored.settingsValues.Set(settings.CatalogueDefault, settingFilenameScrollSpeed, settings.Value{Number: n}); err == nil {
			t.Fatal("accepted invalid filename speed", n)
		}
	}
	restored.activateSettingsAction(layout.Context{}, 2)
	if restored.settingText(settingFilenameScrollEnabled) != "true" || restored.settingText(settingFilenameScrollModel) != "Ping Pong" || restored.settingText(settingFilenameScrollSpeed) != "5" {
		t.Fatal("Reset did not restore filename animation defaults")
	}
	restored.finishSettingsWidget(layout.Context{}, false)
	sendRestored()
	if !restored.panels[0].filenameScroll.ticker || restored.panels[0].filenameScroll.speed != 36 {
		t.Fatal("Cancel did not undo Reset and restart the saved animation")
	}
}
