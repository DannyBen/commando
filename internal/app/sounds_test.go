package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

func TestSoundsEnabledStopsAudioAndGatesAllCues(t *testing.T) {
	v := newPreviewView()
	recorder := &recordedSounds{}
	v.sounds = recorder
	cues := []sound.Name{sound.Navigation, sound.Cancel, sound.Palette, sound.Settings, sound.DirectoryNavigation, sound.MessageInfo, sound.MessageSuccess, sound.MessageWarning, sound.MessageError}
	for _, cue := range cues {
		v.playSound(cue)
	}
	if !slices.Equal(recorder.played, cues) {
		t.Fatal("sounds did not default to enabled")
	}
	v.startSettingsWidget("feedback.sounds")
	s := v.settingsInteraction()
	if s.title != "Feedback › Sounds" || len(s.fields) != 1 || s.fields[0].Label != "Enabled" {
		t.Fatal("Sounds did not offer only Enabled")
	}
	stops, played := recorder.stops, len(recorder.played)
	v.adjustSetting(settingSoundsEnabled, 0)
	if !s.dirty || recorder.stops != stops+1 {
		t.Fatal("disabling sounds did not stop audio immediately")
	}
	for _, cue := range cues {
		v.playSound(cue)
	}
	if len(recorder.played) != played {
		t.Fatal("muted cue escaped the shared gate")
	}
	v.adjustSetting(settingSoundsEnabled, 0)
	baseline := len(recorder.played)
	for _, cue := range cues {
		v.playSound(cue)
	}
	if !slices.Equal(recorder.played[baseline:], cues) {
		t.Fatal("re-enabling sounds did not restore all cues")
	}
}

func TestMutedInputAndFeedbackStillWork(t *testing.T) {
	for _, context := range []string{"navigation", "cancel", "palette", "settings", "messages", "directory"} {
		t.Run(context, func(t *testing.T) {
			v := newPreviewView()
			_ = v.settingsValues.Set(settings.CatalogueDefault, settingSoundsEnabled, settings.Value{Bool: false})
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
	v.adjustSetting(settingSoundsEnabled, 0)
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	v.finishSettingsWidget(layout.Context{}, true)
	saved, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil || saved.Get(settings.CatalogueDefault, settingSoundsEnabled).Bool {
		t.Fatal("disabled sounds did not persist", err)
	}
	restored := newPreviewView()
	recorder := &recordedSounds{}
	restored.sounds = recorder
	restored.restoreSettings(saved, nil)
	restored.announceInteraction(eventSelectionChanged)
	if len(recorder.played) != 0 {
		t.Fatal("startup ignored disabled sounds")
	}
	restored.startSettingsWidget("feedback.sounds")
	restored.adjustSetting(settingSoundsEnabled, 0)
	restored.finishSettingsWidget(layout.Context{}, false)
	baseline := len(recorder.played)
	restored.announceInteraction(eventSelectionChanged)
	if len(recorder.played) != baseline {
		t.Fatal("Cancel did not restore disabled sounds")
	}
	restored.startSettingsWidget("feedback.sounds")
	restored.activateSettingsAction(layout.Context{}, 2)
	if restored.settingsValues.Overridden(settingSoundsEnabled) || !restored.settingsValues.Get(settings.CatalogueDefault, settingSoundsEnabled).Bool {
		t.Fatal("Reset did not inherit enabled sounds")
	}
	restored.finishSettingsWidget(layout.Context{}, true)
	restored.announceInteraction(eventSelectionChanged)
	if recorder.played[len(recorder.played)-1] != sound.Navigation {
		t.Fatal("Reset did not restore sound playback")
	}
}

func TestSoundsSettingsPreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional sound settings preview")
	}
	v := newPreviewView()
	v.startSettingsWidget("feedback.sounds")
	if err := renderViewPNG(filepath.Join(dir, "settings-sounds.png"), v); err != nil {
		t.Fatal(err)
	}
}
