package app

import (
	"image"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

func openShortcutWidget(t *testing.T, v *view, send func(...event.Event), query string) (*paletteSession, *settingsSession) {
	t.Helper()
	send(pressed("`"))
	send(key.EditEvent{Text: query})
	p := v.paletteInteraction()
	if p == nil || len(p.results) == 0 {
		t.Fatal("command was not found")
	}
	send(modified("E", key.ModCtrl))
	s := v.settingsInteraction()
	if s == nil || s.bindingCommand == nil || s.palette != p || len(s.fields) != 2 {
		t.Fatal("Ctrl+E did not open the two-slot widget")
	}
	return p, s
}

func TestShortcutWidgetSavesThroughWorkerAndRestoresPalette(t *testing.T) {
	v := newPreviewView()
	v.active = 1
	dir := t.TempDir()
	v.loader = newDirectoryLoader(browser.ReadLocation, nil, len(v.panels))
	defer v.loader.close()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", bindingSettingsCatalogue().Save(dir, values) }
	}
	send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	p, s := openShortcutWidget(t, v, send, "metadata")
	// Use a non-first result so restoring selection is independently observable.
	send(pressed(key.NameEscape))
	send(pressed(key.NameDownArrow))
	selected := p.selected
	send(modified("E", key.ModCtrl))
	s = v.settingsInteraction()
	id := s.bindingCommand.id
	query, start, end := v.commandSearch.Text(), 0, 0
	start, end = v.commandSearch.Selection()
	marks := maps.Clone(v.panels[1].marks)
	row := v.panels[1].selected
	previous := s.bindingCommand.shortcut()
	send(pressed(key.NameReturn))
	if !s.capturing || v.activeEditor() != nil {
		t.Fatal("shortcut capture used a native text editor")
	}
	send(pressed(key.NameCtrl), key.Event{Name: "Y", Modifiers: key.ModCtrl, State: key.Release})
	if !s.capturing || s.dirty {
		t.Fatal("modifier-only input or a release assigned a shortcut")
	}
	send(modified("Y", key.ModCtrl))
	if s.capturing || v.settingText(s.fields[0].Key) != "Ctrl+Y" {
		t.Fatalf("first shortcut was not captured: capturing=%v value=%q error=%q focus=%v", s.capturing, v.settingText(s.fields[0].Key), s.errorText, router.Source().Focused(&v.focus))
	}
	if v.resolveCommandBindings(*s.bindingCommand).shortcut() != previous {
		t.Fatal("unsaved shortcut changed active bindings")
	}
	send(pressed(key.NameTab), pressed(key.NameReturn))
	send(modified("Y", key.ModCtrl|key.ModShift))
	send(modified(key.NameReturn, key.ModCtrl))
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for s.pending {
		select {
		case result := <-v.loader.results:
			v.acceptLoad(result)
		case <-deadline.C:
			t.Fatal("shortcut save did not finish")
		}
	}
	send()
	caretStart, caretEnd := v.commandSearch.Selection()
	if v.paletteInteraction() != p || p.selected != selected || v.commandSearch.Text() != query || caretStart != start || caretEnd != end || !router.Source().Focused(&v.commandSearch) || v.active != 1 {
		t.Fatal("Save lost palette search, selection, caret, focus or panel context")
	}
	if p.results[selected].command.shortcut() != "Ctrl+Y / Ctrl+Shift+Y" || v.panels[1].selected != row || !maps.Equal(v.panels[1].marks, marks) {
		t.Fatal("Save left stale shortcuts or changed file selection and marks")
	}
	loaded, err := settings.Load(dir, bindingSettingsCatalogue())
	if err != nil || loaded.Get(bindingSettingsCatalogue(), bindingSettingKey(id, 0)).Text != "Ctrl+Y" || loaded.Get(bindingSettingsCatalogue(), bindingSettingKey(id, 1)).Text != "Ctrl+Shift+Y" {
		t.Fatal("shortcuts did not persist through the worker", err)
	}
}

func TestShortcutWidgetCaptureCancelClearResetAndWholeCancel(t *testing.T) {
	v := newPreviewView()
	v.nativeKeys = &nativeKeyInput{invalidate: func() {}}
	send, _ := panelInputFrames(v)
	p, s := openShortcutWidget(t, v, send, "Next metadata column")
	send(pressed(key.NameReturn))
	if !v.nativeKeys.suppressesAltText('y') {
		t.Fatal("capture did not suppress native Alt character handling")
	}
	send(pressed(key.NameEscape))
	if s.capturing || s.dirty || v.nativeKeys.suppressesAltText('y') {
		t.Fatal("Escape did not cancel capture without modifying a value")
	}
	send(pressed(key.NameDeleteForward))
	if v.settingText(s.fields[0].Key) != "Unassigned" || !s.dirty {
		t.Fatal("Delete did not explicitly clear the selected slot")
	}
	// Tab through slot 2 and the shared Save/Cancel/Reset actions.
	send(pressed(key.NameTab), pressed(key.NameTab), pressed(key.NameTab), pressed(key.NameTab), pressed(key.NameReturn))
	if s.draft.Overridden(s.fields[0].Key) || v.settingText(s.fields[0].Key) != "Ctrl+M" {
		t.Fatal("Reset did not restore the inherited default")
	}
	// Dirty Escape selects Cancel, and Enter explicitly discards the draft.
	send(pressed(key.NameEscape))
	send(pressed(key.NameReturn))
	if v.paletteInteraction() != p || v.settingsValues.Overridden(s.fields[0].Key) {
		t.Fatal("Cancel lost the palette or retained shortcut changes")
	}
}

func TestShortcutWidgetConflictsAndProtectedSlots(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	_, s := openShortcutWidget(t, v, send, "Next metadata column")
	send(pressed(key.NameReturn))
	send(modified("C", key.ModCtrl))
	if v.confirmationInteraction() == nil || s.capturing || s.errorText != "" || s.dirty || s.draft.Overridden(s.fields[0].Key) {
		t.Fatal("conflict changed a binding or was not identified", s.errorText)
	}
	send(pressed(key.NameEscape))
	send(pressed(key.NameReturn))
	send(modified("Y", key.ModCtrl))
	send(pressed(key.NameTab), pressed(key.NameReturn))
	send(modified("Y", key.ModCtrl))
	if v.confirmationInteraction() == nil || s.capturing || s.draft.Overridden(s.fields[1].Key) {
		t.Fatal("duplicate slots were accepted or lost their conflict", s.errorText)
	}
	send(pressed(key.NameEscape))
	send(pressed(key.NameEscape))
	send(pressed(key.NameReturn))
	send(pressed(key.NameEscape))
	_, s = openShortcutWidget(t, v, send, "Clear filter or marks")
	if s.selected != 1 {
		t.Fatal("protected first slot was selected instead of its editable alternative")
	}
	send(pressed(key.NameUpArrow), pressed(key.NameReturn))
	if s.capturing || s.errorText != "Shortcut is protected" {
		t.Fatal("protected shortcut could be captured")
	}
	send(pressed(key.NameDeleteForward))
	if s.dirty || s.draft.Overridden(s.fields[0].Key) {
		t.Fatal("protected shortcut could be cleared")
	}
	send(pressed(key.NameEscape))
	send(pressed(key.NameEscape))
	_, s = openShortcutWidget(t, v, send, "Entry Paste")
	send(pressed(key.NameDownArrow), pressed(key.NameReturn))
	if !s.capturing || v.settingText(s.fields[1].Key) != "Shift+Insert" {
		t.Fatal("native alias was not editable")
	}
}

func TestShortcutWidgetCapturesNativeInsertWithoutPasting(t *testing.T) {
	v := newPreviewView()
	v.nativeKeys = &nativeKeyInput{invalidate: func() {}}
	send, _ := panelInputFrames(v)
	_, s := openShortcutWidget(t, v, send, "Next metadata column")
	send(pressed(key.NameReturn))
	v.nativeKeys.push(modified(insertKey, key.ModShift))
	send()
	if v.confirmationInteraction() == nil || s.capturing || v.transferPending {
		t.Fatal("native Shift+Insert pasted or bypassed conflict checking")
	}
	send(pressed(key.NameEscape))
	send(pressed(key.NameReturn))
	v.nativeKeys.push(modified(insertKey, key.ModCtrl))
	send()
	if s.capturing || v.settingText(s.fields[0].Key) != "Ctrl+Insert" || v.transferPending || v.settingsValues.Overridden(s.fields[0].Key) {
		t.Fatal("native Insert was not captured as a staged shortcut")
	}
}

func TestShortcutWidgetFailedSaveRetainsDraftAndOldBindings(t *testing.T) {
	v := newPreviewView()
	v.prepareSettingsSave = func(settings.Values) func() (string, error) {
		return func() (string, error) { return "", os.ErrPermission }
	}
	send, _ := panelInputFrames(v)
	_, s := openShortcutWidget(t, v, send, "Next metadata column")
	send(pressed(key.NameReturn))
	send(modified("Y", key.ModCtrl))
	send(modified(key.NameReturn, key.ModCtrl))
	if v.settingsInteraction() != s || s.pending || s.errorText != "Settings save: access denied" || v.settingText(s.fields[0].Key) != "Ctrl+Y" || v.settingsValues.Overridden(s.fields[0].Key) {
		t.Fatal("failed Save lost the draft, error or original binding")
	}
}

func TestShortcutWidgetPreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_ACTIONS")
	if dir == "" {
		t.Skip("optional headless shortcut widget preview")
	}
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	_, _ = openShortcutWidget(t, v, send, "Next metadata column")
	for _, name := range []string{"shortcuts", "shortcuts-capture", "shortcuts-conflict"} {
		if name == "shortcuts-capture" {
			send(pressed(key.NameReturn))
		} else if name == "shortcuts-conflict" {
			send(modified("C", key.ModCtrl))
		}
		if err := renderViewPNG(filepath.Join(dir, name+".png"), v); err != nil {
			t.Fatal(err)
		}
	}
}

func TestShortcutWidgetSoundsForClearAndCapture(t *testing.T) {
	v := newPreviewView()
	recorder := &recordedSounds{}
	v.sounds = recorder
	send, _ := panelInputFrames(v)
	openShortcutWidget(t, v, send, "Next metadata column")
	recorder.played = nil
	send(pressed(key.NameDeleteForward))
	send(pressed(key.NameReturn))
	send(modified("Y", key.ModCtrl))
	if !slices.Equal(recorder.played, []sound.Name{sound.Navigation, sound.Settings, sound.Settings}) {
		t.Fatal("clear and capture did not use the expected cues", recorder.played)
	}
	recorder.played = nil
	send(key.Event{Name: "Y", Modifiers: key.ModCtrl, State: key.Release})
	if len(recorder.played) != 0 {
		t.Fatal("key release played a sound")
	}
	// Move from Shortcut 1 through Shortcut 2, Save and Cancel to Reset.
	send(pressed(key.NameTab), pressed(key.NameTab), pressed(key.NameTab), pressed(key.NameTab))
	recorder.played = nil
	send(pressed(key.NameReturn))
	if !slices.Equal(recorder.played, []sound.Name{sound.Settings}) {
		t.Fatal("Reset did not play one settings cue", recorder.played)
	}
}
