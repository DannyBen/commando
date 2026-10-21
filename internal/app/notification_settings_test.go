package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/layout"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

func TestNotificationAnimationOffKeepsMessagesAndSounds(t *testing.T) {
	for _, tc := range []struct {
		kind feedback.Kind
		cue  sound.Name
	}{
		{feedback.Info, sound.MessageInfo}, {feedback.Success, sound.MessageSuccess},
		{feedback.Warning, sound.MessageWarning}, {feedback.Error, sound.MessageError},
	} {
		v := newPreviewView()
		recorded := &recordedSounds{}
		v.sounds = recorded
		v.startSettingsWidget("feedback.notifications")
		s := v.settingsInteraction()
		if s.title != "Interface › Animations › Notifications" || len(s.fields) != 1 || s.fields[0].Label != "Enabled" {
			t.Fatal("Notifications did not offer a single Enabled field")
		}
		v.adjustSetting(settingNotificationAnimation, 0)
		if v.appearance.Feedback.Enabled || !s.dirty {
			t.Fatal("Animation did not apply live")
		}
		baseline := len(recorded.played)
		now := time.Unix(1000, 0)
		v.showFeedback(1, tc.kind, "Operation result")
		for i, elapsed := range []time.Duration{0, time.Second, 24 * time.Hour} {
			frame, owns := v.panels[1].feedback.Frame(now.Add(elapsed), v.appearance.Feedback)
			if !owns || frame.Text != "Operation result" || !frame.Next.IsZero() || frame.Started != (i == 0) {
				t.Fatalf("Animation Off did not keep complete steady feedback: %+v", frame)
			}
			v.playFeedbackSound(frame)
		}
		if len(recorded.played) != baseline+1 || recorded.played[baseline] != tc.cue {
			t.Fatal("Animation Off changed semantic message sound")
		}
		v.panels[1].feedback.Dismiss()
		if _, owns := v.panels[1].feedback.Frame(now, v.appearance.Feedback); owns {
			t.Fatal("Animation Off prevented dismissal")
		}
	}
}

func TestAnimationToggleUpdatesExistingMessageAndCancelRestores(t *testing.T) {
	v := newPreviewView()
	now := time.Unix(1000, 0)
	v.showFeedback(1, feedback.Info, "Reading folder")
	first, _ := v.panels[1].feedback.Frame(now, v.appearance.Feedback)
	if first.Next.IsZero() {
		t.Fatal("notifications did not default to animated")
	}
	v.startSettingsWidget("feedback.notifications")
	v.adjustSetting(settingNotificationAnimation, 0)
	frame, _ := v.panels[1].feedback.Frame(now.Add(time.Millisecond), v.appearance.Feedback)
	if frame.Text != "Reading folder" || !frame.Next.IsZero() || frame.Started {
		t.Fatal("live toggle did not complete existing message without re-announcing")
	}
	v.finishSettingsWidget(layout.Context{}, false)
	restored, _ := v.panels[1].feedback.Frame(now.Add(time.Millisecond), v.appearance.Feedback)
	if !v.appearance.Feedback.Enabled || restored.Next.IsZero() || restored.Started {
		t.Fatal("Cancel did not restore animation without re-announcing")
	}
}

func TestNotificationAnimationSaveStartupAndReset(t *testing.T) {
	v := newPreviewView()
	dir := t.TempDir()
	v.startSettingsWidget("feedback.notifications")
	v.adjustSetting(settingNotificationAnimation, 0)
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	v.finishSettingsWidget(layout.Context{}, true)
	values, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	restored := newPreviewView()
	restored.restoreSettings(values, nil)
	if restored.appearance.Feedback.Enabled || values.Get(settings.CatalogueDefault, settingSoundProfile).Number != sound.All {
		t.Fatal("startup did not retain Animation Off independently of sounds")
	}
	// Progress retains its delay, then becomes complete steady text.
	now := time.Unix(1000, 0)
	deadline := now.Add(restored.appearance.FeedbackProgressDelay)
	restored.panels[1].feedback.ProgressAt("Reading folder", deadline)
	before, owns := restored.panels[1].feedback.Frame(now, restored.appearance.Feedback)
	if owns || before.Next != deadline {
		t.Fatal("Animation Off bypassed progress delay")
	}
	visible, owns := restored.panels[1].feedback.Frame(deadline, restored.appearance.Feedback)
	if !owns || visible.Text != "Reading folder" || !visible.Next.IsZero() {
		t.Fatal("delayed progress animated while disabled")
	}
	restored.panels[1].feedback.Complete()
	if _, owns := restored.panels[1].feedback.Frame(deadline, restored.appearance.Feedback); owns {
		t.Fatal("progress did not clear on completion")
	}
	restored.startSettingsWidget("feedback.notifications")
	restored.activateSettingsAction(layout.Context{}, 2)
	if !restored.appearance.Feedback.Enabled || restored.settingsValues.Overridden(settingNotificationAnimation) {
		t.Fatal("Reset did not restore default animation")
	}
	restored.finishSettingsWidget(layout.Context{}, false)
	if restored.appearance.Feedback.Enabled {
		t.Fatal("Cancel did not undo Reset")
	}
}

func TestNotificationsSettingsPreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional notification settings preview")
	}
	v := newPreviewView()
	_ = v.settingsValues.Set(settings.CatalogueDefault, settingNotificationAnimation, settings.Value{Bool: false})
	v.applySettings()
	v.showFeedback(1, feedback.Success, "Files copied")
	v.startSettingsWidget("feedback.notifications")
	if err := renderViewPNG(filepath.Join(dir, "settings-notifications.png"), v); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationsReturningToOnRemovesSavedOverride(t *testing.T) {
	v := newPreviewView()
	dir := t.TempDir()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	v.startSettingsWidget("feedback.notifications")
	v.adjustSetting(settingNotificationAnimation, 0)
	v.finishSettingsWidget(layout.Context{}, true)
	disabled, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil || !disabled.Overridden(settingNotificationAnimation) || disabled.Get(settings.CatalogueDefault, settingNotificationAnimation).Bool {
		t.Fatal("Animation Off did not persist", err)
	}
	v.startSettingsWidget("feedback.notifications")
	v.adjustSetting(settingNotificationAnimation, 0)
	v.finishSettingsWidget(layout.Context{}, true)
	data, err := os.ReadFile(filepath.Join(dir, "settings.toml"))
	if err != nil || len(data) != 0 {
		t.Fatalf("returning Animation to On retained a default override or empty tables: %q, %v", data, err)
	}
	restored, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil || restored.Overridden(settingNotificationAnimation) || !restored.Get(settings.CatalogueDefault, settingNotificationAnimation).Bool {
		t.Fatal("Animation On did not inherit its default", err)
	}
}
