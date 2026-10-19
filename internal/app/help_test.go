package app

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

func TestHelpSoundsFollowTransitionsThroughKeyboardAndPalette(t *testing.T) {
	v := newPreviewView()
	recorder := &recordedSounds{}
	v.sounds = recorder
	send, _ := panelInputFrames(v)
	if len(recorder.played) != 0 {
		t.Fatal("initial help visibility played a toggle sound", recorder.played)
	}
	send(pressed(key.NameF1))
	send(pressed(key.NameF1), key.Event{Name: key.NameF1, State: key.Release})
	send(pressed(key.NameF1), key.Event{Name: key.NameF1, State: key.Release})
	if !slices.Equal(recorder.played, []sound.Name{sound.HelpClosed, sound.HelpOpened}) {
		t.Fatal("help transitions, repeats or releases played the wrong sounds", recorder.played)
	}
	send(pressed("`"))
	send(replacePalette(v, "Toggle help"))
	recorder.played = nil
	send(pressed(key.NameReturn))
	if !v.helpHidden || !slices.Equal(recorder.played, []sound.Name{sound.HelpClosed}) {
		t.Fatal("palette help toggle did not use the shared transition sound", recorder.played)
	}
}

func TestHelpToggleInputAndPersistence(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	if v.helpHidden || aboveHelpFooter(v, f32.Point{}).Y >= 0 {
		t.Fatal("first launch hid help")
	}
	selected, marks, owner := v.panels[0].selected, v.panels[0].selectedNames(), v.interaction.current
	send(pressed(key.NameF1))
	if !v.helpHidden || aboveHelpFooter(v, f32.Point{}).Y != 0 {
		t.Fatal("F1 did not hide help")
	}
	send(pressed(key.NameF1))
	if !v.helpHidden {
		t.Fatal("held F1 repeated the toggle")
	}
	send(key.Event{Name: key.NameF1, State: key.Release})
	if !v.helpHidden {
		t.Fatal("F1 release toggled help")
	}
	dir := t.TempDir()
	if err := saveState(dir, v.sessionState(windowState{})); err != nil {
		t.Fatal(err)
	}
	saved, err := loadState(dir)
	if err != nil || !saved.HelpHidden {
		t.Fatal("hidden help was not persisted", err)
	}
	send(pressed(key.NameF1), key.Event{Name: key.NameF1, State: key.Release})
	if v.helpHidden || v.sessionState(windowState{}).HelpHidden {
		t.Fatal("next F1 press did not restore help")
	}
	if v.panels[0].selected != selected || !reflect.DeepEqual(marks, v.panels[0].selectedNames()) || v.interaction.current != owner {
		t.Fatal("help toggle changed browsing state")
	}
	send(pressed("`"))
	send(replacePalette(v, "Toggle help"))
	s := v.paletteInteraction()
	if s == nil || len(s.results) == 0 || s.results[0].command.id != helpCommandID {
		t.Fatal("help toggle was not discoverable in the palette")
	}
	send(pressed(key.NameReturn))
	if !v.helpHidden || !v.interaction.idle() {
		t.Fatal("palette did not execute the shared help command")
	}
}

func TestHelpTogglePreservesForegroundSessions(t *testing.T) {
	for _, context := range []string{"palette", "location", "filter", "name", "settings", "settings editor", "inline action", "confirmation"} {
		t.Run(context, func(t *testing.T) {
			v := newPreviewView()
			send, _ := panelInputFrames(v)
			switch context {
			case "palette":
				send(pressed("`"), key.EditEvent{Text: "theme"})
			case "location":
				send(modified("D", key.ModAlt))
			case "filter":
				send(key.EditEvent{Text: "/"})
				send(key.EditEvent{Text: "docs"})
			case "name":
				send(pressed(key.NameF2))
			case "settings", "settings editor":
				v.startSettingsWidget(settingThemeGenerator)
				send()
				if context == "settings editor" {
					send(pressed(key.NameDownArrow), pressed(key.NameReturn))
				}
			case "inline action":
				v.startInlineActions(0, 4, "", "", []string{"Overwrite", "Cancel"}, nil, nil)
				send()
			case "confirmation":
				v.startConfirmation(layout.Context{}, &confirmationSession{message: "Continue?", acceptLabel: "Continue"})
				send()
			}
			owner, editor := v.interaction.current, v.activeEditor()
			if owner == nil {
				t.Fatal("test did not start its foreground session")
			}
			value, start, end := "", 0, 0
			if editor != nil {
				value = editor.Text()
				start, end = editor.Selection()
			}
			send(pressed(key.NameF1), key.Event{Name: key.NameF1, State: key.Release})
			if !v.helpHidden || v.interaction.current != owner || v.activeEditor() != editor {
				t.Fatal("F1 failed or replaced the foreground session")
			}
			if editor != nil {
				gotStart, gotEnd := editor.Selection()
				if editor.Text() != value || gotStart != start || gotEnd != end {
					t.Fatal("F1 changed editor text or selection")
				}
			}
			send(pressed(key.NameF1), key.Event{Name: key.NameF1, State: key.Release})
			if v.helpHidden || v.interaction.current != owner {
				t.Fatal("F1 failed to restore help within the session")
			}
		})
	}
}

func TestHelpRebindingAndShortcutCapture(t *testing.T) {
	v := newPreviewView()
	if err := v.settingsValues.Set(bindingSettingsCatalogue(), bindingSettingKey(helpCommandID, 0), settings.Value{Text: "F9"}); err != nil {
		t.Fatal(err)
	}
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF1), key.Event{Name: key.NameF1, State: key.Release})
	if v.helpHidden {
		t.Fatal("old help binding still executed")
	}
	send(pressed("`"))
	send(pressed(key.NameF9), key.Event{Name: key.NameF9, State: key.Release})
	if !v.helpHidden || v.paletteInteraction() == nil {
		t.Fatal("replacement binding did not work in the palette")
	}
	send(pressed(key.NameEscape))
	_, s := openShortcutWidget(t, v, send, "Toggle help")
	send(pressed(key.NameReturn))
	if !s.capturing {
		t.Fatal("shortcut capture did not start")
	}
	send(pressed(key.NameF9), key.Event{Name: key.NameF9, State: key.Release})
	if !v.helpHidden || s.capturing {
		t.Fatal("help intercepted shortcut capture")
	}
}

func TestHelpBindingPreservesNativeEditorKeys(t *testing.T) {
	v := newPreviewView()
	if err := v.settingsValues.Set(bindingSettingsCatalogue(), bindingSettingKey(helpCommandID, 0), settings.Value{Text: "Ctrl+Backspace"}); err != nil {
		t.Fatal(err)
	}
	send, _ := panelInputFrames(v)
	send(modified("D", key.ModAlt))
	v.location.SetText("alpha beta")
	v.location.SetCaret(10, 10)
	send(modified(key.NameDeleteBackward, key.ModCtrl))
	if v.helpHidden || v.location.Text() != "alpha " || !v.locationEditing() {
		t.Fatal("help binding intercepted native word deletion")
	}
}

func TestHelpCatalogueValidationAndBindings(t *testing.T) {
	for _, source := range []string{
		`["test"]
lines = []`,
		`["test"]
lines = ["one", "two", "three"]`,
		`["test"]
lines = [""]`,
		`["test"]
lines = ["one\ntwo"]`,
		`["test"]
lines = ["[key:missing.command]"]`,
		`["test"]
lines = ["[key:application.toggle_help"]`,
		`["test"]
line = "misspelled"`,
	} {
		if _, err := parseHelpCatalogue(source, defaultCommands()); err == nil {
			t.Errorf("accepted invalid catalogue: %s", source)
		}
	}
	catalogue, err := parseHelpCatalogue(`["test.unicode"]
lines = ["Å text [literal] [key:application.toggle_help] / [key:application.command_palette]."]`, defaultCommands())
	if err != nil {
		t.Fatal(err)
	}
	v := newPreviewView()
	line := catalogue["test.unicode"].lines[0]
	flatten := func() string {
		var text strings.Builder
		for _, span := range v.resolveHelpLine(line) {
			text.WriteString(span.text)
		}
		return text.String()
	}
	if got := flatten(); got != "Å text [literal] [F1] / [`]." {
		t.Fatal("unexpected default help text", got)
	}
	set := func(id string, slot int, value string) {
		t.Helper()
		if err := v.settingsValues.Set(bindingSettingsCatalogue(), bindingSettingKey(id, slot), settings.Value{Text: value}); err != nil {
			t.Fatal(err)
		}
	}
	set(helpCommandID, 0, "F9")
	set("application.command_palette", 0, "none")
	if got := flatten(); got != "Å text [literal] [F9] / [Ctrl+Shift+P]." {
		t.Fatal("help ignored current or alternate binding", got)
	}
	set("application.command_palette", 1, "none")
	if got := flatten(); !strings.HasSuffix(got, "[unassigned].") {
		t.Fatal("help invented an unassigned shortcut", got)
	}
}
