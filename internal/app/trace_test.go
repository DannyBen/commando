package app

import (
	"os"
	"path/filepath"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/diagnostics"
	"github.com/dannyben/commando/internal/settings"
)

func TestTraceWidgetStagesChangesUntilSave(t *testing.T) {
	v := newPreviewView()
	v.activeDataFolder = t.TempDir()
	t.Cleanup(func() { diagnostics.Close() })
	v.startSettingsWidget("debug.trace")
	send, _ := panelInputFrames(v)
	send(pressed(key.NameRightArrow))
	send(pressed(key.NameEnd))
	if v.settingText(settingTraceMask) != "127" || diagnostics.Enabled(diagnostics.FileOperations) || v.settingsValues.Get(settings.CatalogueDefault, settingTraceMask).Number != 0 {
		t.Fatal("draft mask leaked into running settings")
	}
	// Reset, cycling back to OFF and saving must never create a log.
	v.settingsInteraction().actionsFocused, v.settingsInteraction().action = true, 2
	send(pressed(key.NameReturn))
	send(modified(key.NameReturn, key.ModCtrl))
	if _, err := os.Stat(filepath.Join(v.activeDataFolder, "trace-log.jsonl")); !os.IsNotExist(err) {
		t.Fatal("editing an OFF mask created a log")
	}
	v.startSettingsWidget("debug.trace")
	send(pressed(key.NameRightArrow))
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(v.activeDataFolder, values) }
	}
	send(modified(key.NameReturn, key.ModCtrl))
	if !diagnostics.Enabled(diagnostics.FileOperations) {
		t.Fatal("saved mask was not applied")
	}
	restored, err := settings.Load(v.activeDataFolder, settings.CatalogueDefault)
	if err != nil || restored.Get(settings.CatalogueDefault, settingTraceMask).Number != 1 {
		t.Fatal("draft mask was not persisted", err)
	}
	v.startSettingsWidget("debug.trace")
	send(pressed(key.NameHome))
	if !diagnostics.Enabled(diagnostics.FileOperations) {
		t.Fatal("editing OFF prematurely stopped tracing")
	}
	v.settingsInteraction().actionsFocused, v.settingsInteraction().action = true, 1
	send(pressed(key.NameReturn))
	if !diagnostics.Enabled(diagnostics.FileOperations) || v.settingText(settingTraceMask) != "1" {
		t.Fatal("Cancel changed committed mask")
	}
}

func TestTraceCLIOverrideDoesNotChangeSavedValue(t *testing.T) {
	t.Cleanup(func() { diagnostics.Close() })
	for _, mask := range []diagnostics.Mask{0, diagnostics.Shell} {
		v := newPreviewView()
		v.activeDataFolder = t.TempDir()
		v.traceOverride = &mask
		if err := v.settingsValues.Set(settings.CatalogueDefault, settingTraceMask, settings.Value{Number: 1}); err != nil {
			t.Fatal(err)
		}
		v.applySettings()
		if diagnostics.Enabled(diagnostics.FileOperations) || diagnostics.Enabled(diagnostics.Shell) != (mask == diagnostics.Shell) {
			t.Fatal("CLI override was not honored")
		}
		if v.settingText(settingTraceMask) != "1" {
			t.Fatal("CLI changed saved mask")
		}
		diagnostics.Close()
		if mask == 0 {
			if _, err := os.Stat(filepath.Join(v.activeDataFolder, "trace-log.jsonl")); !os.IsNotExist(err) {
				t.Fatal("explicit OFF wrote a log")
			}
		}
	}
}

func TestTraceMaskRangeMatchesSettings(t *testing.T) {
	d, _ := settings.CatalogueDefault.Definition(settingTraceMask)
	if d.Default.Number != 0 || d.Maximum == nil || *d.Maximum != float64(diagnostics.All) {
		t.Fatal("settings mask range drifted")
	}
	for _, n := range []float64{-1, 1.5, 128} {
		var v settings.Values
		if err := v.Set(settings.CatalogueDefault, settingTraceMask, settings.Value{Number: n}); err == nil {
			t.Fatalf("accepted %g", n)
		}
	}
}

func TestTraceSettingsPreview(t *testing.T) {
	directory := os.Getenv("COMMANDO_PREVIEW_TRACE")
	if directory == "" {
		t.Skip("optional trace settings preview")
	}
	v := newPreviewView()
	v.startSettingsWidget("debug.trace")
	if err := v.widgetValues().Set(settings.CatalogueDefault, settingTraceMask, settings.Value{Number: 4}); err != nil {
		t.Fatal(err)
	}
	if err := renderViewPNG(filepath.Join(directory, "trace-settings.png"), v); err != nil {
		t.Fatal(err)
	}
}
