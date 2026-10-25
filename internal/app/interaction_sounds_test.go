package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"unicode/utf8"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

func TestLocationEditorUsesClickAndSingleNavigationCue(t *testing.T) {
	for _, enter := range []key.Name{key.NameReturn, key.NameEnter} {
		t.Run(string(enter), func(t *testing.T) {
			root := t.TempDir()
			child := filepath.Join(root, "child")
			if err := os.Mkdir(child, 0700); err != nil {
				t.Fatal(err)
			}
			v := newBrowserView([]string{root}, nil)
			defer v.loader.close()
			waitLoaded(t, v, 0)
			recorder := &recordedSounds{}
			v.sounds = recorder
			send, _ := panelInputFrames(v)
			send(modified("D", key.ModAlt))
			if !v.locationEditing() || !slices.Equal(recorder.played, []sound.Name{sound.Click}) {
				t.Fatal("location opening did not use one quiet click", recorder.played)
			}
			send(key.EditEvent{Range: key.Range{End: utf8.RuneCountInString(v.location.Text())}, Text: child})
			recorder.played = nil
			send(pressed(enter))
			waitLoaded(t, v, 0)
			send()
			if v.locationEditing() || v.panels[0].path != child || !slices.Equal(recorder.played, []sound.Name{sound.DirectoryNavigation}) {
				t.Fatal("location submission did not navigate with one cue", recorder.played)
			}
			send(modified("D", key.ModAlt))
			recorder.played = nil
			send(pressed(key.NameEscape))
			if v.locationEditing() || !slices.Equal(recorder.played, []sound.Name{sound.Click}) {
				t.Fatal("location cancellation lost its single shared cue", recorder.played)
			}
		})
	}
}

func TestInteractionSoundMapUsesConfiguredCues(t *testing.T) {
	for event, cue := range interactionSounds {
		if cue != "" && !sound.Allows(cue, sound.All) {
			t.Fatalf("event %s maps to an unconfigured cue %s", event, cue)
		}
	}
}

func TestSoundMapControlsSharedTextAndShortcutEditing(t *testing.T) {
	beforeStart, beforeConfirm := interactionSounds[eventEditStarted], interactionSounds[eventEditConfirmed]
	interactionSounds[eventEditStarted], interactionSounds[eventEditConfirmed] = sound.Enabled, sound.Disabled
	t.Cleanup(func() {
		interactionSounds[eventEditStarted], interactionSounds[eventEditConfirmed] = beforeStart, beforeConfirm
	})
	for _, capture := range []bool{false, true} {
		name := "text"
		if capture {
			name = "shortcut"
		}
		t.Run(name, func(t *testing.T) {
			v := newPreviewView()
			recorder := &recordedSounds{}
			v.sounds = recorder
			send, _ := panelInputFrames(v)
			if capture {
				openShortcutWidget(t, v, send, "Next metadata column")
			} else {
				send(key.EditEvent{Text: "@font"})
				send(pressed(key.NameReturn))
			}
			recorder.played = nil
			send(pressed(key.NameReturn))
			if !slices.Equal(recorder.played, []sound.Name{sound.Enabled}) {
				t.Fatal("shared edit start ignored central mapping", recorder.played)
			}
			recorder.played = nil
			if capture {
				send(modified("Y", key.ModCtrl))
			} else {
				send(key.EditEvent{Text: "Example Font"})
				if len(recorder.played) != 0 {
					t.Fatal("native typing played a cue")
				}
				send(pressed(key.NameReturn))
			}
			if !slices.Equal(recorder.played, []sound.Name{sound.Disabled}) {
				t.Fatal("shared edit confirmation ignored central mapping", recorder.played)
			}
		})
	}
}

func TestSilentOrMutedInteractionEventsDoNotPlay(t *testing.T) {
	v := newPreviewView()
	recorder := &recordedSounds{}
	v.sounds = recorder
	before := interactionSounds[eventWidgetReset]
	interactionSounds[eventWidgetReset] = ""
	t.Cleanup(func() { interactionSounds[eventWidgetReset] = before })
	v.announceInteraction(eventWidgetReset)
	v.announceInteraction(interactionEvent("unknown"))
	if len(recorder.played) != 0 {
		t.Fatal("silent events reached audio player", recorder.played)
	}
	if err := v.settingsValues.Set(settings.CatalogueDefault, settingSoundEnabled, settings.Value{Bool: false}); err != nil {
		t.Fatal(err)
	}
	v.announceInteraction(eventEditStarted)
	if len(recorder.played) != 0 {
		t.Fatal("interaction event bypassed shared mute gate")
	}
}

func TestImplicitWidgetCommitsPlayOnlyTheirEnclosingAction(t *testing.T) {
	v := newPreviewView()
	recorder := &recordedSounds{}
	v.sounds = recorder
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@font"})
	send(pressed(key.NameReturn))
	send(pressed(key.NameReturn))
	recorder.played = nil
	send(pressed(key.NameTab))
	if !slices.Equal(recorder.played, []sound.Name{sound.Navigation}) {
		t.Fatal("Tab doubled field commit and navigation cues", recorder.played)
	}
	send(pressed(key.NameReturn))
	recorder.played = nil
	send(modified(key.NameReturn, key.ModCtrl))
	if !slices.Equal(recorder.played, []sound.Name{sound.Closed}) {
		t.Fatal("Save doubled field commit and save cues", recorder.played)
	}
}

func TestWidgetResetAndStagedDeleteShareActionFeedback(t *testing.T) {
	for _, deletion := range []bool{false, true} {
		name := "reset"
		if deletion {
			name = "delete"
		}
		t.Run(name, func(t *testing.T) {
			v := editorBindingView()
			recorder := &recordedSounds{}
			v.sounds = recorder
			send, _ := panelInputFrames(v)
			if deletion {
				v.startCollectionWidget(settings.EditorCollection, 0, "txt md")
			} else {
				v.startSettingsWidget("appearance.font")
			}
			send()
			s := v.settingsInteraction()
			s.actionsFocused, s.action = true, 2
			recorder.played = nil
			send(pressed(key.NameReturn))
			if !slices.Equal(recorder.played, []sound.Name{sound.Click}) || deletion && !s.deleted {
				t.Fatal("accepted shared action did not produce one cue", recorder.played)
			}
			recorder.played = nil
			send(key.Event{Name: key.NameReturn, State: key.Release})
			if len(recorder.played) != 0 {
				t.Fatal("action release played a cue")
			}
		})
	}
}

func TestFailedSavesAndRestoredSessionsDoNotAnnounceSuccess(t *testing.T) {
	v := newPreviewView()
	recorder := &recordedSounds{}
	v.sounds = recorder
	send, _ := panelInputFrames(v)
	p, s := openShortcutWidget(t, v, send, "Next metadata column")
	recorder.played = nil
	s.pending = true
	v.acceptSettingsSave(s, "", os.ErrPermission)
	if len(recorder.played) != 0 || v.settingsInteraction() != s {
		t.Fatal("failed save sounded successful or closed its session")
	}
	if v.startInteraction(&settingsSession{}) || len(recorder.played) != 0 {
		t.Fatal("rejected session opening played a cue")
	}
	s.pending = true
	v.acceptSettingsSave(s, "", nil)
	if v.paletteInteraction() != p || !slices.Equal(recorder.played, []sound.Name{sound.Closed}) {
		t.Fatal("restoring palette added an opening cue", recorder.played)
	}
	// A save carrying a footer message leaves its cue to the existing message owner.
	send(pressed(key.NameEscape))
	v.startSettingsWidget("appearance.font")
	s = v.settingsInteraction()
	recorder.played = nil
	s.pending = true
	v.acceptSettingsSave(s, "Restart Commando", nil)
	if !slices.Equal(recorder.played, []sound.Name{sound.MessageInfo}) {
		t.Fatal("save result did not arbitrate its message against the close cue", recorder.played)
	}
	send()
	if !slices.Equal(recorder.played, []sound.Name{sound.MessageInfo}) {
		t.Fatal("save message did not own its single cue", recorder.played)
	}
}

func TestShortcutNoOpAndRejectedEditsStaySilent(t *testing.T) {
	v := newPreviewView()
	recorder := &recordedSounds{}
	v.sounds = recorder
	send, _ := panelInputFrames(v)
	openShortcutWidget(t, v, send, "Next metadata column")
	send(pressed(key.NameDeleteForward))
	recorder.played = nil
	send(pressed(key.NameDeleteForward))
	if len(recorder.played) != 0 {
		t.Fatal("clearing an unassigned shortcut played a change cue")
	}
	send(pressed(key.NameReturn))
	recorder.played = nil
	send(modified("C", key.ModCtrl))
	if !slices.Equal(recorder.played, []sound.Name{sound.Opened}) || v.confirmationInteraction() == nil {
		t.Fatal("conflicting capture did not announce its question once", recorder.played)
	}
}

func TestInvalidWidgetTextDraftStaysSilent(t *testing.T) {
	v := newPreviewView()
	recorder := &recordedSounds{}
	v.sounds = recorder
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@font"})
	send(pressed(key.NameReturn))
	s := v.settingsInteraction()
	s.selected = 1
	send(pressed(key.NameReturn))
	send(key.EditEvent{Text: "not a number"})
	recorder.played = nil
	send(pressed(key.NameReturn))
	if len(recorder.played) != 0 || !s.editing || s.errorText == "" {
		t.Fatal("invalid text draft played a confirmation cue", recorder.played)
	}
}

func TestInlineEditingUsesClickForStartAndCancel(t *testing.T) {
	for _, editor := range []string{"filter", "rename", "create", "location", "inline"} {
		t.Run(editor, func(t *testing.T) {
			v := newPreviewView()
			v.panels[0].clearMarks() // F2 must edit one name rather than open multi-rename.
			recorder := &recordedSounds{}
			v.sounds = recorder
			send, _ := panelInputFrames(v)
			switch editor {
			case "filter":
				send(pressed("/"), key.EditEvent{Text: "/"})
			case "rename":
				send(pressed(key.NameF2))
			case "create":
				send(newFolderKey())
			case "location":
				send(modified("D", key.ModAlt))
			case "inline":
				v.startInlineActions(0, 4, "", "", []string{"Choose"}, nil, nil)
				send()
			}
			if !slices.Equal(recorder.played, []sound.Name{sound.Click}) {
				t.Fatal("inline editing did not start with one click", recorder.played)
			}
			send(pressed(key.NameEscape))
			if !v.interaction.idle() || !slices.Equal(recorder.played, []sound.Name{sound.Click, sound.Click}) {
				t.Fatal("inline editing did not cancel with one click", recorder.played)
			}
		})
	}
}

func TestWidgetEditingClicksWithoutClosingWidget(t *testing.T) {
	v := newPreviewView()
	recorder := &recordedSounds{}
	v.sounds = recorder
	send, _ := panelInputFrames(v)
	v.startSettingsWidget("appearance.font")
	send()
	widget := v.settingsInteraction()
	if widget == nil || !slices.Equal(recorder.played, []sound.Name{sound.Opened}) {
		t.Fatal("settings widget did not open with Open", recorder.played)
	}
	recorder.played = nil
	send(pressed(key.NameReturn))
	send(pressed(key.NameReturn))
	if v.settingsInteraction() != widget || widget.editing || !slices.Equal(recorder.played, []sound.Name{sound.Click, sound.Click}) {
		t.Fatal("field editing used lifecycle cues or closed the widget", recorder.played)
	}
	recorder.played = nil
	widget.actionsFocused, widget.action = true, 1 // Explicit Cancel closes a dirty widget.
	send(pressed(key.NameReturn))
	if v.settingsInteraction() != nil || !slices.Equal(recorder.played, []sound.Name{sound.Closed}) {
		t.Fatal("closing the actual widget did not retain Close", recorder.played)
	}
}
