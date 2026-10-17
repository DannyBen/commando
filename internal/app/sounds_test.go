package app

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

func TestSoundProfilesGateAllSemanticCues(t *testing.T) {
	all := []sound.Name{sound.MessageError, sound.MessageWarning, sound.MessageInfo, sound.MessageSuccess, sound.Palette, sound.Settings, sound.Cancel, sound.Navigation, sound.DirectoryNavigation}
	for _, tc := range []struct {
		profile float64
		count   int
	}{{1, 1}, {2, 2}, {3, 4}, {4, 7}, {sound.All, 9}} {
		v := newPreviewView()
		recorder := &recordedSounds{}
		v.sounds = recorder
		_ = v.settingsValues.Set(settings.CatalogueDefault, settingSoundProfile, settings.Value{Number: tc.profile})
		for _, cue := range all {
			v.playSound(cue)
		}
		if !slices.Equal(recorder.played, all[:tc.count]) {
			t.Fatalf("profile %g played %v", tc.profile, recorder.played)
		}
		for _, volume := range recorder.volumes {
			if volume != 50 {
				t.Fatal("default master volume changed")
			}
		}
	}
}

func TestSoundSettingsStopsAudioAndPreviewsAtMasterVolume(t *testing.T) {
	v := newPreviewView()
	recorder := &recordedSounds{}
	v.sounds = recorder
	v.applySettings()
	v.startSettingsWidget("feedback.sounds")
	s := v.settingsInteraction()
	if s.title != "Feedback › Sounds" || len(s.fields) != 3 || s.fields[0].Label != "Enabled" || s.fields[1].Label != "Profile" || s.fields[2].Label != "Master volume" {
		t.Fatal("Sounds must offer Enabled, Profile and Master volume")
	}
	stops, played := recorder.stops, len(recorder.played)
	v.previewSettingValue(settingSoundEnabled, settings.Value{Bool: false})
	if recorder.stops != stops+1 || len(recorder.played) != played {
		t.Fatal("disabling sounds did not stop audio silently")
	}
	v.previewSettingValue(settingSoundEnabled, settings.Value{Bool: true})
	v.previewSettingValue(settingSoundProfile, settings.Value{Number: 1})
	if recorder.played[len(recorder.played)-1] != sound.MessageError {
		t.Fatal("profile preview bypassed profile policy")
	}
	v.previewSettingValue(settingSoundVolume, settings.Value{Number: 35})
	if recorder.played[len(recorder.played)-1] != sound.MessageError || recorder.volumes[len(recorder.volumes)-1] != 35 {
		t.Fatal("volume preview ignored current profile or volume")
	}
	v.previewSettingValue(settingSoundVolume, settings.Value{Number: 0})
	played = len(recorder.played)
	v.playSound(sound.MessageError)
	if len(recorder.played) != played {
		t.Fatal("master volume zero was audible")
	}
	v.finishSettingsWidget(layout.Context{}, false)
	if v.currentSoundSettings().profile != sound.All || v.currentSoundSettings().volume != 50 {
		t.Fatal("Cancel did not restore sound settings")
	}
}

func TestMutedInputAndFeedbackStillWork(t *testing.T) {
	for _, context := range []string{"navigation", "cancel", "palette", "settings", "messages", "directory"} {
		t.Run(context, func(t *testing.T) {
			v := newPreviewView()
			_ = v.settingsValues.Set(settings.CatalogueDefault, settingSoundEnabled, settings.Value{Bool: false})
			recorder := &recordedSounds{}
			v.sounds = recorder
			v.applySettings()
			send, _ := panelInputFrames(v)
			switch context {
			case "navigation":
				before := v.panels[0].selected
				send(pressed(key.NameDownArrow))
				if v.panels[0].selected == before {
					t.Fatal("muting blocked navigation")
				}
			case "cancel":
				send(modified("D", key.ModAlt))
				send(pressed(key.NameEscape))
				if !v.interaction.idle() {
					t.Fatal("muting blocked cancellation")
				}
			case "palette":
				send(pressed("`"))
				if v.paletteInteraction() == nil {
					t.Fatal("muting blocked palette")
				}
			case "settings":
				v.startSettingsWidget("feedback.sounds")
				v.escape(layout.Context{})
				if !v.interaction.idle() {
					t.Fatal("muting blocked settings close")
				}
			case "messages":
				for _, kind := range []feedback.Kind{feedback.Info, feedback.Success, feedback.Warning, feedback.Error} {
					v.showFeedback(0, kind, "Operation result")
					send()
					if v.panels[0].feedback.Current() == nil {
						t.Fatal("muting hid feedback")
					}
				}
			case "directory":
				v.loader = &directoryLoader{requests: make(chan loadRequest, 1)}
				v.queueLoad(loadRequest{panel: 0, path: "/folder"})
				if !v.panels[0].loading {
					t.Fatal("muting blocked directory navigation")
				}
			}
			if len(recorder.played) != 0 {
				t.Fatalf("muted %s played %v", context, recorder.played)
			}
		})
	}
}

func TestSoundsSaveStartupCancelAndReset(t *testing.T) {
	dir := t.TempDir()
	v := newPreviewView()
	v.startSettingsWidget("feedback.sounds")
	v.previewSettingValue(settingSoundEnabled, settings.Value{Bool: false})
	v.previewSettingValue(settingSoundProfile, settings.Value{Number: 2})
	v.previewSettingValue(settingSoundVolume, settings.Value{Number: 35})
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	v.finishSettingsWidget(layout.Context{}, true)
	saved, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	restored := newPreviewView()
	recorder := &recordedSounds{}
	restored.sounds = recorder
	restored.restoreSettings(saved, nil)
	restored.announceInteraction(eventSelectionChanged)
	restored.announceInteraction(eventMessageInfo)
	restored.announceInteraction(eventMessageWarning)
	if len(recorder.played) != 0 || restored.currentSoundSettings() != (soundSettings{enabled: false, profile: 2, volume: 35}) {
		t.Fatal("startup ignored enabled, profile or volume")
	}
	restored.startSettingsWidget("feedback.sounds")
	restored.activateSettingsAction(layout.Context{}, 2)
	if restored.currentSoundSettings() != (soundSettings{enabled: true, profile: sound.All, volume: 50}) {
		t.Fatal("Reset did not restore All sounds and 50%")
	}
	restored.finishSettingsWidget(layout.Context{}, false)
	if restored.currentSoundSettings() != (soundSettings{enabled: false, profile: 2, volume: 35}) {
		t.Fatal("Cancel did not undo Reset")
	}
	restored.startSettingsWidget("feedback.sounds")
	restored.activateSettingsAction(layout.Context{}, 2)
	restored.finishSettingsWidget(layout.Context{}, true)
	restored.announceInteraction(eventSelectionChanged)
	if recorder.played[len(recorder.played)-1] != sound.Navigation {
		t.Fatal("Reset did not restore playback")
	}
}

func TestSoundProfileCyclingAndAutocomplete(t *testing.T) {
	v := newPreviewView()
	v.startSettingsWidget("feedback.sounds")
	send, _ := panelInputFrames(v)
	send(pressed(key.NameDownArrow)) // Select Profile after Enabled.
	for _, label := range []string{"Errors only", "Warnings and errors", "Notifications only", "All except navigation", "All sounds"} {
		send(pressed(key.NameRightArrow))
		if v.settingText(settingSoundProfile) != label {
			t.Fatal("profile did not cycle", label, v.settingText(settingSoundProfile))
		}
	}
	send(pressed(key.NameReturn))
	s := v.settingsInteraction()
	send(key.EditEvent{Range: key.Range{End: len(s.editor.Text())}, Text: "Warnings"}, key.SelectionEvent{Start: 8, End: 8})
	if s.completion.suffix() != " and errors" {
		t.Fatal("profile label did not autocomplete", s.editor.Text(), s.completion.suffix())
	}
	send(pressed(key.NameTab), pressed(key.NameReturn))
	if s.editing || v.currentSoundSettings().profile != 2 {
		t.Fatal("completed profile did not apply")
	}
	send(pressed(key.NameReturn))
	send(key.EditEvent{Range: key.Range{End: len(s.editor.Text())}, Text: "invalid"}, key.SelectionEvent{Start: 7, End: 7})
	send(pressed(key.NameReturn))
	if !s.editing || s.errorText != "Choose an available option" || v.currentSoundSettings().profile != 2 {
		t.Fatal("invalid profile changed sound policy")
	}
}

func TestSoundsSettingsPreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional sound settings preview")
	}
	v := newPreviewView()
	_ = v.settingsValues.Set(settings.CatalogueDefault, settingSoundEnabled, settings.Value{Bool: false})
	v.applySettings()
	v.startSettingsWidget("feedback.sounds")
	if err := renderViewPNG(filepath.Join(dir, "settings-sounds-disabled.png"), v); err != nil {
		t.Fatal(err)
	}
	for _, profile := range []float64{1, 2, 3, 4, sound.All} {
		v := newPreviewView()
		_ = v.settingsValues.Set(settings.CatalogueDefault, settingSoundProfile, settings.Value{Number: profile})
		v.applySettings()
		v.startSettingsWidget("feedback.sounds")
		name := "settings-sounds-" + strconv.Itoa(int(profile)) + ".png"
		if err := renderViewPNG(filepath.Join(dir, name), v); err != nil {
			t.Fatal(err)
		}
	}
}

func TestToggleSoundPaletteAndCustomShortcut(t *testing.T) {
	v := newPreviewView()
	_ = v.settingsValues.Set(settings.CatalogueDefault, settingSoundProfile, settings.Value{Number: 2})
	_ = v.settingsValues.Set(settings.CatalogueDefault, settingSoundVolume, settings.Value{Number: 35})
	recorder := &recordedSounds{}
	v.sounds = recorder
	v.applySettings()
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	send(key.EditEvent{Text: "Feedback Toggle sound"})
	p := v.paletteInteraction()
	if len(p.results) == 0 || p.results[0].command.id != "feedback.toggle_sound" || p.results[0].command.shortcut() != "" {
		t.Fatal("Toggle sound must be discoverable and initially unbound")
	}
	send(pressed(key.NameReturn))
	if !v.interaction.idle() || v.currentSoundSettings() != (soundSettings{enabled: false, profile: 2, volume: 35}) || len(recorder.played) != 0 {
		t.Fatal("palette toggle did not mute while preserving preferences")
	}
	setTestShortcut(t, v, "feedback.toggle_sound", 0, "Ctrl+Y")
	v.applySettings()
	send(modified("Y", key.ModCtrl))
	if v.currentSoundSettings() != (soundSettings{enabled: true, profile: 2, volume: 35}) || !slices.Equal(recorder.played, []sound.Name{sound.MessageWarning}) || recorder.volumes[0] != 35 {
		t.Fatal("bound toggle did not restore sound with one profile-gated preview")
	}
	send(key.Event{Name: "Y", Modifiers: key.ModCtrl, State: key.Release})
	if !v.currentSoundSettings().enabled || len(recorder.played) != 1 {
		t.Fatal("key release toggled or played sound")
	}
}

func TestToggleSoundWorkerSaveAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(strconv.FormatBool(fail), func(t *testing.T) {
			dir := t.TempDir()
			v := newPreviewView()
			_ = v.settingsValues.Set(settings.CatalogueDefault, settingSoundProfile, settings.Value{Number: 2})
			_ = v.settingsValues.Set(settings.CatalogueDefault, settingSoundVolume, settings.Value{Number: 35})
			v.applySettings()
			catalogue := bindingSettingsCatalogue()
			if err := catalogue.Save(dir, v.settingsValues); err != nil {
				t.Fatal(err)
			}
			v.loader = newDirectoryLoader(browser.ReadLocation, nil, len(v.panels))
			v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
				return func() (string, error) {
					if fail {
						return "", os.ErrPermission
					}
					return "", catalogue.Save(dir, values)
				}
			}
			v.toggleBooleanSetting(settingSoundEnabled)
			if v.currentSoundSettings().enabled || !v.mutationsBusy() {
				t.Fatal("toggle did not apply live and track its pending save")
			}
			v.finishOperations()
			loaded, err := settings.LoadAndRepair(dir, catalogue)
			if err != nil {
				t.Fatal(err)
			}
			restored := newPreviewView()
			restored.restoreSettings(loaded, nil)
			want := soundSettings{enabled: fail, profile: 2, volume: 35}
			if v.currentSoundSettings() != want || restored.currentSoundSettings() != want {
				t.Fatal("save/revert changed profile or volume or lost Enabled")
			}
			if fail {
				message := v.panels[0].feedback.Current()
				if message == nil || message.Text != "Settings save: access denied" || message.Kind != feedback.Error {
					t.Fatal("failed toggle did not report its save error")
				}
			}
		})
	}
}
