package app

import (
	"os"
	"slices"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

func TestUntaggedCommandDefaultsToClickAndRejectedCommandsStaySilent(t *testing.T) {
	v := newPreviewView()
	recorder := &recordedSounds{}
	v.sounds = recorder
	calls := 0
	c := command{id: "test.new_action", execute: func(*view) { calls++ }}
	v.executeCommand(layout.Context{}, c)
	if calls != 1 || !slices.Equal(recorder.played, []sound.Name{sound.Click}) {
		t.Fatal("new command required a handwritten sound", recorder.played)
	}
	recorder.played = nil
	c.available = func(*view) bool { return false }
	v.executeCommand(layout.Context{}, c)
	if calls != 1 || len(recorder.played) != 0 {
		t.Fatal("rejected command announced execution")
	}
	c.available = nil
	v.executeCommand(layout.Context{}, c.withSound(eventWidgetOpened))
	if !slices.Equal(recorder.played, []sound.Name{sound.Opened}) {
		t.Fatal("command sound tag did not replace its default click")
	}
	recorder.played = nil
	v.executeCommand(layout.Context{}, c.withSoundState(func(v *view) bool { return v.followMe }, eventEnabled))
	if len(recorder.played) != 0 {
		t.Fatal("unchanged boolean command announced a change")
	}
}

func TestSoundActionChoosesOneOutcomeAndDoesNotReplayItsNotification(t *testing.T) {
	v := newPreviewView()
	recorder := &recordedSounds{}
	v.sounds = recorder
	send, _ := panelInputFrames(v)
	c := command{id: "test.combined", execute: func(v *view) {
		defer v.beginSoundAction()() // Nested ownership cannot flush early.
		v.announceInteraction(eventSelectionChanged)
		v.announceBoolean(true)
		v.showFeedback(0, feedback.Error, "Access denied")
		v.announceInteraction(eventWidgetClosed)
	}}
	v.executeCommand(layout.Context{}, c)
	if !slices.Equal(recorder.played, []sound.Name{sound.MessageError}) {
		t.Fatal("action played competing cues", recorder.played)
	}
	send()
	if len(recorder.played) != 1 {
		t.Fatal("footer replayed an acknowledged sound")
	}
	// A later action is independent, even without advancing a clock.
	v.executeCommand(layout.Context{}, command{execute: func(*view) {}})
	if !slices.Equal(recorder.played, []sound.Name{sound.MessageError, sound.Click}) {
		t.Fatal("unrelated command was coalesced with the previous action")
	}
}

func TestSoundArbitrationPrecedesProfileGating(t *testing.T) {
	v := newPreviewView()
	recorder := &recordedSounds{}
	v.sounds = recorder
	_ = v.settingsValues.Set(settings.CatalogueDefault, settingSoundProfile, settings.Value{Number: 3})
	v.executeCommand(layout.Context{}, command{execute: func(v *view) {
		v.showFeedback(0, feedback.Info, "Enabled")
		v.announceBoolean(true)
	}})
	if len(recorder.played) != 0 {
		t.Fatal("a gated toggle fell back to a lower-priority Info cue", recorder.played)
	}
}

func TestSimultaneousCompletionsAnnounceErrorOnceWithoutLosingMessages(t *testing.T) {
	v := newPreviewView()
	recorder := &recordedSounds{}
	v.sounds = recorder
	v.loader = &directoryLoader{results: make(chan loadResult, 2)}
	v.loader.results <- loadResult{request: loadRequest{panel: 0}, err: os.ErrPermission}
	v.loader.results <- loadResult{request: loadRequest{panel: 1, successMessage: "Ready"}, directory: browser.Directory{Path: v.panels[1].path}}
	v.pollLoads()
	if !slices.Equal(recorder.played, []sound.Name{sound.MessageError}) {
		t.Fatal("completion sounds competed or Info replaced Error", recorder.played)
	}
	if v.panels[0].feedback.Current() == nil || v.panels[1].feedback.Current() == nil {
		t.Fatal("sound arbitration lost a panel's visible feedback")
	}
}

func TestEveryBooleanWidgetUsesSharedOnOffAndNoOpPolicy(t *testing.T) {
	for _, field := range settings.CatalogueDefault.UI.Fields {
		definition, ok := settings.CatalogueDefault.Definition(field.Key)
		if !ok || definition.Kind != settings.Boolean || field.Key == settingSoundEnabled {
			continue
		}
		t.Run(field.Key, func(t *testing.T) {
			v := newPreviewView()
			recorder := &recordedSounds{}
			v.sounds = recorder
			before := v.settingsValues.Get(settings.CatalogueDefault, field.Key).Bool
			value := settings.Value{Bool: !before}
			v.previewSettingValue(field.Key, value)
			v.previewSettingValue(field.Key, value) // Repeated endpoints are quiet.
			v.previewSettingValue(field.Key, settings.Value{Bool: before})
			want := []sound.Name{sound.Enabled, sound.Disabled}
			if before {
				slices.Reverse(want)
			}
			if !slices.Equal(recorder.played, want) {
				t.Fatal("boolean field bypassed the shared policy", recorder.played)
			}
		})
	}
}

func TestEveryDirectBooleanSettingUsesSharedOnOff(t *testing.T) {
	for _, c := range newPreviewView().commandCatalogue() {
		if c.toggleSetting == "" || c.toggleSetting == settingSoundEnabled {
			continue
		}
		t.Run(c.id, func(t *testing.T) {
			v := newPreviewView()
			recorder := &recordedSounds{}
			v.sounds = recorder
			before := v.settingsValues.Get(settings.CatalogueDefault, c.toggleSetting).Bool
			v.executeCommand(layout.Context{}, c)
			v.executeCommand(layout.Context{}, c)
			want := []sound.Name{sound.Enabled, sound.Disabled}
			if before {
				slices.Reverse(want)
			}
			if !slices.Equal(recorder.played, want) {
				t.Fatal("direct toggle bypassed shared policy", recorder.played)
			}
		})
	}
}

func TestMetadataCycleClicksThroughKeyboardAndPaletteAtInteractionLevel(t *testing.T) {
	v := newPreviewView()
	recorder := &recordedSounds{}
	v.sounds = recorder
	_ = v.settingsValues.Set(settings.CatalogueDefault, settingSoundProfile, settings.Value{Number: 4})
	send, _ := panelInputFrames(v)
	before := v.panels[0].metadata
	send(modified("M", key.ModCtrl))
	send(key.Event{Name: "M", Modifiers: key.ModCtrl, State: key.Release})
	if v.panels[0].metadata == before || !slices.Equal(recorder.played, []sound.Name{sound.Click}) {
		t.Fatal("Ctrl+M did not produce one interaction click", recorder.played)
	}
	send(pressed("`"))
	send(key.EditEvent{Text: "Next metadata column"})
	recorder.played = nil
	send(pressed(key.NameReturn))
	if !slices.Equal(recorder.played, []sound.Name{sound.Click}) {
		t.Fatal("palette handoff doubled or changed the command cue", recorder.played)
	}
	for _, column := range []metadataColumn{metadataExtension, metadataSize, metadataModified, metadataCreated} {
		_ = v.settingsValues.Set(settings.CatalogueDefault, settingMetadataColumns+"."+column.stateName(), settings.Value{Bool: false})
	}
	v.panels[0].metadata = metadataNone
	recorder.played = nil
	send(modified("M", key.ModCtrl))
	if len(recorder.played) != 0 {
		t.Fatal("cycling without an available next column sounded changed")
	}
}

func TestShortcutEditorHandoffAnnouncesOnlyItsOpening(t *testing.T) {
	v := newPreviewView()
	recorder := &recordedSounds{}
	v.sounds = recorder
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	send(key.EditEvent{Text: "Next metadata column"})
	recorder.played = nil
	send(modified("E", key.ModCtrl))
	if v.settingsInteraction() == nil || !slices.Equal(recorder.played, []sound.Name{sound.Opened}) {
		t.Fatal("shortcut editor handoff emitted competing lifecycle sounds", recorder.played)
	}
}

func TestMouseCancellationAndItsResultShareOneCue(t *testing.T) {
	v := newPreviewView()
	var ops op.Ops
	v.beginName(layout.Context{Ops: &ops}, &nameSession{kind: nameDuplicate, panel: 0, row: 4,
		path: v.panels[0].path, previousPosition: v.panels[0].list.Position}, "copy.txt", false)
	v.transferPending = true
	recorder := &recordedSounds{}
	v.sounds = recorder
	if !v.cancelInteraction(layout.Context{Ops: &ops}) || !v.interaction.idle() {
		t.Fatal("mouse cancellation was rejected")
	}
	if !slices.Equal(recorder.played, []sound.Name{sound.MessageInfo}) {
		t.Fatal("mouse cancellation played both close and result cues", recorder.played)
	}
	send, _ := panelInputFrames(v)
	send()
	if len(recorder.played) != 1 {
		t.Fatal("cancellation result replayed in the footer")
	}
}

func TestSortToggleOwnsInfoSoundAndNoOpsStaySilent(t *testing.T) {
	v := newPreviewView()
	recorder := &recordedSounds{}
	v.sounds = recorder
	send, _ := panelInputFrames(v)
	send(modified(key.NameF3, key.ModCtrl))
	send(modified(key.NameF3, key.ModCtrl))
	if !slices.Equal(recorder.played, []sound.Name{sound.Enabled, sound.Disabled}) {
		t.Fatal("sort notification doubled its toggle sounds", recorder.played)
	}
	v.panels[0].loading = true
	recorder.played = nil
	send(modified(key.NameF3, key.ModCtrl))
	if len(recorder.played) != 0 {
		t.Fatal("busy sort command announced a change")
	}
}

type soundTestSession struct{}

func (*soundTestSession) interactionPanel() int { return 0 }

func TestNewSessionsAutomaticallyUseSharedOpenClose(t *testing.T) {
	v := newPreviewView()
	recorder := &recordedSounds{}
	v.sounds = recorder
	s := &soundTestSession{}
	if !v.startInteraction(s) || v.startInteraction(&soundTestSession{}) {
		t.Fatal("invalid session acceptance")
	}
	if !v.finishInteraction(s) || v.finishInteraction(s) {
		t.Fatal("invalid session completion")
	}
	if !slices.Equal(recorder.played, []sound.Name{sound.Opened, sound.Closed}) {
		t.Fatal("new session required feature-specific audio code", recorder.played)
	}
}
