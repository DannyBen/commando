package app

import (
	"os"
	"reflect"
	"slices"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/diagnostics"
	"github.com/dannyben/commando/internal/settings"
)

func deferWidget(t *testing.T, group string) {
	t.Helper()
	before := settings.CatalogueDefault
	t.Cleanup(func() { settings.CatalogueDefault = before })
	settings.CatalogueDefault.UI.Widgets = slices.Clone(before.UI.Widgets)
	for i := range settings.CatalogueDefault.UI.Widgets {
		w := &settings.CatalogueDefault.UI.Widgets[i]
		if w.Key == group {
			w.Deferred = true
			return
		}
	}
	t.Fatal("widget not found", group)
}

func TestDeferredWidgetsApplyOnlyAfterSave(t *testing.T) {
	for _, tc := range []struct{ group, field string }{
		{"appearance.autocolor", settingLightness},
		{"appearance.font", settingFontTextSize},
		{"feedback.sounds", settingSoundVolume},
		{settingThemeGenerator, settingThemeGenerator + ".color_shift"},
	} {
		t.Run(tc.group, func(t *testing.T) {
			deferWidget(t, tc.group)
			v := newPreviewView()
			before := v.settingsValues.Clone()
			v.startSettingsWidget(tc.group)
			s := v.settingsInteraction()
			for i, f := range s.fields {
				if f.Key == tc.field {
					s.selected = i
				}
			}
			send, _ := panelInputFrames(v)
			send(pressed(key.NameRightArrow))
			if !reflect.DeepEqual(v.settingsValues.Clone(), before) || v.widgetValues().Get(settings.CatalogueDefault, tc.field) == before.Get(settings.CatalogueDefault, tc.field) {
				t.Fatal("draft edit changed committed values or failed to update control")
			}
			want := v.widgetValues().Clone()
			send(modified(key.NameReturn, key.ModCtrl))
			if !reflect.DeepEqual(v.settingsValues, want) {
				t.Fatal("Save did not commit widget draft")
			}
		})
	}
}

func TestDeferredSaveFailureRetainsDraftWithoutApplying(t *testing.T) {
	v := newPreviewView()
	v.activeDataFolder = t.TempDir()
	t.Cleanup(func() { diagnostics.Close() })
	v.startSettingsWidget("debug.trace")
	s := v.settingsInteraction()
	send, _ := panelInputFrames(v)
	send(pressed(key.NameRightArrow))
	var captured settings.Values
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		captured = values
		return func() (string, error) { return "", os.ErrPermission }
	}
	send(modified(key.NameReturn, key.ModCtrl))
	if v.settingsInteraction() != s || s.errorText == "" || v.settingText(settingTraceMask) != "1" {
		t.Fatal("save failure lost editable draft")
	}
	if captured.Get(settings.CatalogueDefault, settingTraceMask).Number != 1 || v.settingsValues.Get(settings.CatalogueDefault, settingTraceMask).Number != 0 || diagnostics.Enabled(diagnostics.FileOperations) {
		t.Fatal("failed save applied settings or saved wrong candidate")
	}
	if _, err := os.Stat(v.activeDataFolder + "/trace-log.jsonl"); !os.IsNotExist(err) {
		t.Fatal("failed save created a trace")
	}
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) { return func() (string, error) { return "", nil } }
	send(modified(key.NameReturn, key.ModCtrl))
	if v.settingsInteraction() != nil || !diagnostics.Enabled(diagnostics.FileOperations) {
		t.Fatal("successful retry did not apply draft")
	}
}

func TestDeferredWidgetWaitsForWorkerSaveResult(t *testing.T) {
	v := newPreviewView()
	v.activeDataFolder = t.TempDir()
	t.Cleanup(func() { diagnostics.Close() })
	v.loader = &directoryLoader{requests: make(chan loadRequest, 1)}
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		if values.Get(settings.CatalogueDefault, settingTraceMask).Number != 1 {
			t.Fatal("worker received committed value instead of draft")
		}
		return func() (string, error) { return "", nil }
	}
	v.startSettingsWidget("debug.trace")
	s := v.settingsInteraction()
	send, _ := panelInputFrames(v)
	send(pressed(key.NameRightArrow))
	send(modified(key.NameReturn, key.ModCtrl))
	request := <-v.loader.requests
	if !s.pending || diagnostics.Enabled(diagnostics.FileOperations) {
		t.Fatal("pending save applied draft")
	}
	message, err := request.settingsSave.run()
	if diagnostics.Enabled(diagnostics.FileOperations) {
		t.Fatal("worker applied UI settings")
	}
	v.acceptSettingsSave(s, message, err)
	if v.settingsInteraction() != nil || !diagnostics.Enabled(diagnostics.FileOperations) {
		t.Fatal("successful completion did not apply draft")
	}
}
