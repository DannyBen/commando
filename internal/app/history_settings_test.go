package app

import (
	"os"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
)

func TestHistoryWidgetPreviewCancelSaveAndReset(t *testing.T) {
	v := newPreviewView()
	if !v.settingsValues.Get(settings.CatalogueDefault, settingSaveHistory).Bool || v.historySearchDelay() != 400*time.Millisecond {
		t.Fatal("history defaults changed")
	}
	dir := t.TempDir()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@Interface History"})
	palette := v.paletteInteraction()
	if palette == nil || len(palette.results) == 0 || palette.results[0].command.id != "settings."+settingHistory {
		t.Fatal("history is not discoverable under Interface")
	}
	send(pressed(key.NameReturn))
	s := v.settingsInteraction()
	if s == nil || s.title != "Interface › History" || len(s.fields) != 2 || s.fields[0].Label != "Save history" || s.fields[1].Label != "Match delay (ms)" {
		t.Fatal("history widget did not offer the agreed controls")
	}
	send(pressed(key.NameRightArrow), pressed(key.NameDownArrow), pressed(key.NameRightArrow))
	if v.settingsValues.Get(settings.CatalogueDefault, settingSaveHistory).Bool || v.historySearchDelay() != 500*time.Millisecond {
		t.Fatal("keyboard edits were not previewed")
	}
	if output := os.Getenv("COMMANDO_PREVIEW_HISTORY"); output != "" {
		if err := renderViewPNG(output, v); err != nil {
			t.Fatal(err)
		}
	}
	send(pressed(key.NameEscape))
	send(pressed(key.NameReturn))
	if v.settingsInteraction() != nil || !v.settingsValues.Get(settings.CatalogueDefault, settingSaveHistory).Bool || v.historySearchDelay() != 400*time.Millisecond {
		t.Fatal("Cancel did not restore history preferences")
	}
	send(pressed(key.NameEscape)) // Return from settings search to browsing.
	v.startSettingsWidget(settingHistory)
	v.adjustSetting(settingSaveHistory, 0)
	for range 30 {
		v.adjustSetting(settingHistoryMatchDelay, 1)
	}
	if v.historySearchDelay() != 2*time.Second {
		t.Fatal("match delay did not stop at 2000 ms")
	}
	for range 30 {
		v.adjustSetting(settingHistoryMatchDelay, -1)
	}
	if v.historySearchDelay() != 0 {
		t.Fatal("match delay did not stop at zero")
	}
	for _, value := range []float64{-1, 2001, 100.5} {
		if err := v.settingsValues.Set(settings.CatalogueDefault, settingHistoryMatchDelay, settings.Value{Number: value}); err == nil {
			t.Fatalf("accepted invalid match delay %v", value)
		}
	}
	v.finishSettingsWidget(layout.Context{}, true)
	values, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	restored := newPreviewView()
	restored.restoreSettings(values, nil)
	if restored.settingsValues.Get(settings.CatalogueDefault, settingSaveHistory).Bool || restored.historySearchDelay() != 0 {
		t.Fatal("history preferences did not survive save and startup")
	}
	restored.startSettingsWidget(settingHistory)
	restored.activateSettingsAction(layout.Context{}, 2)
	if !restored.settingsValues.Get(settings.CatalogueDefault, settingSaveHistory).Bool || restored.historySearchDelay() != 400*time.Millisecond {
		t.Fatal("Reset did not restore history defaults")
	}
	restored.finishSettingsWidget(layout.Context{}, false)
	if restored.settingsValues.Get(settings.CatalogueDefault, settingSaveHistory).Bool || restored.historySearchDelay() != 0 {
		t.Fatal("Cancel did not undo Reset")
	}
}
