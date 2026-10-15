package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
)

func editorBindingView() *view {
	v := newPreviewView()
	v.settingsValues.SetCollection(settings.EditorCollection, []map[string]any{
		{"id": "123", "ext": []string{"txt", "md"}, "path": "text.exe", "args": settings.SelectedFilesToken},
		{"id": "new", "ext": []string{"go"}, "path": "go.exe", "args": settings.SelectedFilesToken},
	})
	return v
}

func TestEditorRuleShortcutFollowsIdentityAcrossEditsReorderingAndRestart(t *testing.T) {
	v := editorBindingView()
	setTestShortcut(t, v, editorRuleCommandPrefix+"123", 0, "Ctrl+Y")
	setTestShortcut(t, v, editorRuleCommandPrefix+"123", 1, "Ctrl+Shift+Y")
	setTestShortcut(t, v, editorRuleCommandPrefix+"new", 0, "Ctrl+Alt+Y")
	rows := v.settingsValues.Collection(settings.EditorCollection)
	rows[0]["ext"], rows[0]["path"] = []string{"rb"}, "changed.exe"
	v.settingsValues.SetCollection(settings.EditorCollection, []map[string]any{rows[1], rows[0]})
	dir := t.TempDir()
	catalogue := bindingSettingsCatalogue()
	if err := catalogue.Save(dir, v.settingsValues); err != nil {
		t.Fatal(err)
	}
	loaded, err := settings.LoadAndRepair(dir, catalogue)
	if err != nil {
		t.Fatal(err)
	}
	v.restoreSettings(loaded, nil)
	send, _ := panelInputFrames(v)
	for _, modifiers := range []key.Modifiers{key.ModCtrl, key.ModCtrl | key.ModShift} {
		send(modified("Y", modifiers))
		s := v.settingsInteraction()
		if s == nil || s.item != 1 || s.draft.Item(settings.EditorCollection)["id"] != "123" || s.draft.Item(settings.EditorCollection)["path"] != "changed.exe" {
			t.Fatal("shortcut followed old position or label")
		}
		for _, f := range s.fields {
			if f.Key == settings.EditorCollection+".id" {
				t.Fatal("identity leaked into widget fields")
			}
		}
		send(pressed(key.NameEscape))
	}
	send(modified("Y", key.ModCtrl|key.ModAlt))
	if s := v.settingsInteraction(); s == nil || s.item != 0 || s.draft.Item(settings.EditorCollection)["id"] != "new" {
		t.Fatal("user ID new collided with Editors New command")
	}
}

func TestSavedEditorRuleShortcutWidgetUsesSharedSaveAndConflicts(t *testing.T) {
	v := editorBindingView()
	v.loader = newDirectoryLoader(browser.ReadLocation, nil, len(v.panels))
	defer v.loader.close()
	dir := t.TempDir()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", bindingSettingsCatalogue().Save(dir, values) }
	}
	send, _ := panelInputFrames(v)
	p, s := openShortcutWidget(t, v, send, "@txt md")
	if s.bindingCommand.id != editorRuleCommandPrefix+"123" {
		t.Fatal("saved rule not found", s.bindingCommand.id)
	}
	send(pressed(key.NameReturn))
	send(modified("C", key.ModCtrl))
	if s.errorText != "Conflict: Copy" || !s.capturing {
		t.Fatal("saved rule bypassed shared conflicts", s.errorText)
	}
	send(modified("Y", key.ModCtrl))
	send(modified(key.NameReturn, key.ModCtrl))
	waitEditorRuleSave(t, v, s)
	send()
	if v.paletteInteraction() != p || p.results[p.selected].command.shortcut() != "Ctrl+Y" {
		t.Fatal("shortcut save did not restore palette")
	}
	send(pressed(key.NameEscape))
	send(modified("Y", key.ModCtrl))
	if s := v.settingsInteraction(); s == nil || s.item != 0 || s.bindingCommand != nil {
		t.Fatal("saved shortcut did not open rule widget")
	}
}

func waitEditorRuleSave(t *testing.T, v *view, s *settingsSession) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for s.pending {
		select {
		case result := <-v.loader.results:
			v.acceptLoad(result)
		case <-deadline.C:
			t.Fatal("save timed out")
		}
	}
}

func TestDeletingEditorRuleRemovesOnlyItsBindingsAndCancelRestores(t *testing.T) {
	v := editorBindingView()
	setTestShortcut(t, v, editorRuleCommandPrefix+"123", 0, "Ctrl+Y")
	setTestShortcut(t, v, editorRuleCommandPrefix+"new", 0, "Ctrl+Alt+Y")
	send, _ := panelInputFrames(v)
	stageDelete := func() *settingsSession {
		send(modified("Y", key.ModCtrl))
		s := v.settingsInteraction()
		// Select the existing shared Delete action.
		send(pressed(key.NameTab), pressed(key.NameTab), pressed(key.NameTab), pressed(key.NameTab), pressed(key.NameTab), pressed(key.NameReturn))
		if !s.deleted {
			t.Fatal("Delete did not stage rule removal")
		}
		return s
	}
	stageDelete()
	send(pressed(key.NameEscape), pressed(key.NameReturn))
	if len(v.settingsValues.Collection(settings.EditorCollection)) != 2 || !v.settingsValues.Overridden(bindingSettingKey(editorRuleCommandPrefix+"123", 0)) {
		t.Fatal("Cancel lost rule or bindings")
	}
	stageDelete()
	send(modified(key.NameReturn, key.ModCtrl))
	if len(v.settingsValues.Collection(settings.EditorCollection)) != 1 || v.settingsValues.Overridden(bindingSettingKey(editorRuleCommandPrefix+"123", 0)) || !v.settingsValues.Overridden(bindingSettingKey(editorRuleCommandPrefix+"new", 0)) {
		t.Fatal("Save retained deleted binding or removed another rule's binding")
	}
	send(modified("Y", key.ModCtrl))
	if !v.interaction.idle() {
		t.Fatal("deleted shortcut remained executable")
	}
}

func TestOrphanRuleBindingsAreRemovedOnNextSave(t *testing.T) {
	v := editorBindingView()
	setTestShortcut(t, v, editorRuleCommandPrefix+"old", 0, "Ctrl+Y")
	setTestShortcut(t, v, editorRuleCommandPrefix+"123", 0, "Ctrl+Alt+Y")
	dir := t.TempDir()
	catalogue := bindingSettingsCatalogue()
	if err := catalogue.Save(dir, v.settingsValues); err != nil {
		t.Fatal(err)
	}
	loaded, err := settings.LoadAndRepair(dir, catalogue)
	if err != nil || loaded.Overridden(bindingSettingKey(editorRuleCommandPrefix+"old", 0)) || !loaded.Overridden(bindingSettingKey(editorRuleCommandPrefix+"123", 0)) {
		t.Fatal("orphan bindings were not pruned", err)
	}
	if !v.settingsValues.Overridden(bindingSettingKey(editorRuleCommandPrefix+"old", 0)) {
		t.Fatal("saving mutated caller transaction")
	}
}

func TestDuplicateRuleIdentityRecoveryKeepsShortcutWithFirstRule(t *testing.T) {
	dir := t.TempDir()
	source := "[[programs.editors]]\nid='123'\next=['txt']\npath='one.exe'\n[[programs.editors]]\nid='123'\next=['md']\npath='two.exe'\n[keybindings.settings.programs.editors.rules.123]\nshortcut_1='Ctrl+Y'\n"
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	values, err := settings.LoadAndRepair(dir, bindingSettingsCatalogue())
	if err == nil || len(values.Collection(settings.EditorCollection)) != 2 {
		t.Fatal("duplicate identity did not repair", err)
	}
	v := newPreviewView()
	v.restoreSettings(values, nil)
	send, _ := panelInputFrames(v)
	send(modified("Y", key.ModCtrl))
	if s := v.settingsInteraction(); s == nil || s.item != 0 {
		t.Fatal("shortcut moved away from first duplicate")
	}
}

func TestNewEditorRuleGetsSameIdentityInRuntimeAndSavedSettings(t *testing.T) {
	v := editorBindingView()
	dir := t.TempDir()
	v.loader = newDirectoryLoader(browser.ReadLocation, nil, len(v.panels))
	defer v.loader.close()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", bindingSettingsCatalogue().Save(dir, values) }
	}
	send, _ := panelInputFrames(v)
	v.startCollectionWidget(settings.EditorCollection, -1, "New")
	s := v.settingsInteraction()
	s.draft.SetItem(settings.EditorCollection, map[string]any{"ext": []string{"rb"}, "path": "ruby.exe", "args": settings.SelectedFilesToken})
	send()
	send(modified(key.NameReturn, key.ModCtrl))
	waitEditorRuleSave(t, v, s)
	rows := v.settingsValues.Collection(settings.EditorCollection)
	if len(rows) != 3 {
		t.Fatal("new rule was not saved")
	}
	id, _ := rows[2]["id"].(string)
	if len(id) != 12 || !settings.ValidIdentity(id) || id == rows[0]["id"] || id == rows[1]["id"] {
		t.Fatal("new rule did not receive its own generated identity", id)
	}
	loaded, err := settings.LoadAndRepair(dir, bindingSettingsCatalogue())
	if err != nil || loaded.Collection(settings.EditorCollection)[2]["id"] != id {
		t.Fatal("save changed runtime identity", err)
	}
}
