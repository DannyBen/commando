package app

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/settings"
)

func TestBindingOverridesPersistDispatchAndReset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.toml")
	document := "[keybindings.view.next_metadata_column]\nshortcut_1 = 'Ctrl+Y'\nshortcut_2 = 'Ctrl+Shift+Y'\n"
	if err := os.WriteFile(path, []byte(document), 0600); err != nil {
		t.Fatal(err)
	}
	catalogue := bindingSettingsCatalogue()
	values, err := settings.LoadAndRepair(dir, catalogue)
	if err != nil {
		t.Fatal(err)
	}
	// Saving another setting must retain both shortcut overrides.
	if err := values.Set(catalogue, settingLightness, settings.Value{Number: 0.42}); err != nil {
		t.Fatal(err)
	}
	store := dataFolderStore{base: dir, active: dir, chosen: dir}
	if _, err := store.save(values, sessionState{}); err != nil {
		t.Fatal(err)
	}
	values, err = settings.Load(dir, catalogue)
	if err != nil {
		t.Fatal(err)
	}
	v := newPreviewView()
	v.restoreSettings(values, nil)
	send, _ := panelInputFrames(v)
	before := v.panels[0].metadata
	send(modified("M", key.ModCtrl))
	if v.panels[0].metadata != before {
		t.Fatal("old shortcut still executed after replacement")
	}
	send(modified("Y", key.ModCtrl))
	if v.panels[0].metadata != before.cycle(1) {
		t.Fatal("replacement shortcut was not delivered or executed")
	}
	send(modified("Y", key.ModCtrl|key.ModShift))
	if v.panels[0].metadata != metadataNone { // Created is disabled by default.
		t.Fatal("second shortcut was not delivered or executed")
	}
	send(pressed("`"))
	send(key.EditEvent{Text: "Next metadata column"})
	if s := v.paletteInteraction(); len(s.results) == 0 || s.results[0].command.shortcut() != "Ctrl+Y / Ctrl+Shift+Y" {
		t.Fatal("palette did not show effective shortcuts")
	}
	send(pressed(key.NameEscape))
	values.Reset(bindingSettingKey("view.next_metadata_column", 0))
	values.Reset(bindingSettingKey("view.next_metadata_column", 1))
	if err := catalogue.Save(dir, values); err != nil {
		t.Fatal(err)
	}
	reset, err := settings.Load(dir, catalogue)
	if err != nil || reset.Overridden(bindingSettingKey("view.next_metadata_column", 0)) {
		t.Fatal("reset did not remove the override", err)
	}
	v.restoreSettings(reset, nil)
	before = v.panels[0].metadata
	send(modified("M", key.ModCtrl))
	if v.panels[0].metadata != before.cycle(1) {
		t.Fatal("reset did not restore the default shortcut")
	}
}

func TestExplicitlyUnassignedBindingSurvivesRestart(t *testing.T) {
	catalogue := bindingSettingsCatalogue()
	var values settings.Values
	setting := bindingSettingKey("view.next_metadata_column", 0)
	if err := values.Set(catalogue, setting, settings.Value{Text: "none"}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := catalogue.Save(dir, values); err != nil {
		t.Fatal(err)
	}
	loaded, err := settings.Load(dir, catalogue)
	if err != nil || !loaded.Overridden(setting) || loaded.Get(catalogue, setting).Text != "none" {
		t.Fatal("explicit unassignment was lost", err)
	}
	v := newPreviewView()
	v.restoreSettings(loaded, nil)
	send, _ := panelInputFrames(v)
	before := v.panels[0].metadata
	send(modified("M", key.ModCtrl))
	if v.panels[0].metadata != before {
		t.Fatal("unassigned default still executed")
	}
	send(pressed("`"))
	send(key.EditEvent{Text: "Next metadata column"})
	s := v.paletteInteraction()
	if s.results[0].command.shortcut() != "" {
		t.Fatal("unassigned shortcut remained visible")
	}
	send(pressed(key.NameReturn))
	if v.panels[0].metadata != before.cycle(1) {
		t.Fatal("unassigning a shortcut disabled palette execution")
	}
}

func TestReassigningTerminalDefaultOmitsBothRedundantOverrides(t *testing.T) {
	catalogue := bindingSettingsCatalogue()
	terminal := bindingSettingKey("programs.terminal_here", 0)
	generator := bindingSettingKey("settings.appearance.theme.generator", 0)
	var values settings.Values
	if err := values.Set(catalogue, generator, settings.Value{Text: "Ctrl+T"}); err != nil {
		t.Fatal(err)
	}
	// Simulate the upgrade: Terminal's newly introduced default yields to the
	// user's existing generator binding until the user reassigns it.
	if err := validateBindingOverrides(&values); err != nil {
		t.Fatal(err)
	}
	if values.Get(catalogue, terminal).Text != "none" {
		t.Fatal("upgrade did not preserve the generator's shortcut")
	}
	if err := values.Set(catalogue, terminal, settings.Value{Text: "Ctrl+T"}); err != nil {
		t.Fatal(err)
	}
	resolved, _, ok := reassignBindings(values, "programs.terminal_here", 0)
	if !ok || resolved.Get(catalogue, generator).Text != "none" {
		t.Fatal("reassignment did not clear the generator")
	}
	dir := t.TempDir()
	if err := catalogue.Save(dir, resolved); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "settings.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "" {
		t.Fatalf("effective defaults remained in the user file:\n%s", data)
	}
	// Filtering a saved copy must preserve the live settings transaction.
	if resolved.Get(catalogue, terminal).Text != "Ctrl+T" || resolved.Get(catalogue, generator).Text != "none" {
		t.Fatal("saving changed the supplied values")
	}
	loaded, err := settings.Load(dir, catalogue)
	if err != nil || loaded.Overridden(terminal) || loaded.Overridden(generator) {
		t.Fatal("default bindings did not reload as inherited", err)
	}
}

func TestBindingPersistenceComparesEffectiveDefaultsPerSlot(t *testing.T) {
	for _, tc := range []struct {
		name, command, text string
		slot                int
		stored              bool
	}{
		{"default key", "programs.terminal_here", "Ctrl+T", 0, false},
		{"equivalent key spelling", "programs.terminal_here", "ctrl+t", 0, false},
		{"empty secondary", "programs.terminal_here", "none", 1, false},
		{"unbound settings command", "settings.appearance.theme.generator", "none", 0, false},
		{"default generated key", "panel.focus_3", "Alt+3", 0, false},
		{"unbound dynamic command", "panel.focus_12", "none", 0, false},
		{"explicit unassignment", "programs.terminal_here", "none", 0, true},
		{"custom key", "programs.terminal_here", "Ctrl+Y", 0, true},
		{"custom settings key", "settings.appearance.theme.generator", "Ctrl+Y", 0, true},
		{"custom secondary", "programs.terminal_here", "Ctrl+Y", 1, true},
		{"default key in another slot", "programs.terminal_here", "Ctrl+T", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			catalogue := bindingSettingsCatalogue()
			field := bindingSettingKey(tc.command, tc.slot)
			var values settings.Values
			if err := values.Set(catalogue, field, settings.Value{Text: tc.text}); err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			if err := catalogue.Save(dir, values); err != nil {
				t.Fatal(err)
			}
			loaded, err := settings.Load(dir, catalogue)
			if err != nil || loaded.Overridden(field) != tc.stored {
				t.Fatalf("stored=%v want=%v err=%v", loaded.Overridden(field), tc.stored, err)
			}
			if tc.stored && loaded.Get(catalogue, field).Text != tc.text {
				t.Fatal("non-default shortcut was changed")
			}
		})
	}
}

func TestCustomBrowsingPasteBindingLeavesTextFieldPasteFixed(t *testing.T) {
	v := newPreviewView()
	catalogue := bindingSettingsCatalogue()
	for field, text := range map[string]string{
		bindingSettingKey("entry.paste", 0):               "none",
		bindingSettingKey("view.next_metadata_column", 0): "Ctrl+V",
	} {
		if err := v.settingsValues.Set(catalogue, field, settings.Value{Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	if err := catalogue.ValidateOverrides(&v.settingsValues); err != nil {
		t.Fatal(err)
	}
	router, send := clipboardFrames(v)
	before := v.panels[0].metadata
	send(modified("V", key.ModCtrl))
	if v.panels[0].metadata != before.cycle(1) {
		t.Fatal("Ctrl+V did not execute its new browsing action")
	}
	send(modified("D", key.ModAlt))
	v.location.SetText("")
	send()
	send(modified("V", key.ModCtrl))
	send(clipboardData("pasted text"))
	if v.location.Text() != "pasted text" || !router.Source().Focused(&v.location) || v.panels[0].metadata != before.cycle(1) {
		t.Fatal("browsing binding intercepted text-field paste or changed focus")
	}
}

func TestInvalidBindingsUseSettingsRecovery(t *testing.T) {
	for _, invalid := range []string{
		"[keybindings.entry.rename]\nshortcut_1 = 'Ctrl+'",
		"[keybindings.entry.rename]\nshortcut_1 = 42",
		"[keybindings.selection.clear_filter_or_marks]\nshortcut_1 = 'none'",
		"[keybindings.entry.rename]\nshortcut_1 = 'Ctrl+Y'\n[keybindings.entry.copy]\nshortcut_1 = 'Ctrl+Y'",
		"[keybindings.entry.rename]\nshortcut_1 = 'Ctrl+Y'\nshortcut_2 = 'Ctrl+Y'",
		"[keybindings.entry.rename]\nshortcut_1 = 'Up'",
		"[keybindings.entry.rename]\nshortcut_1 = 'Esc'",
		"[keybindings.entry.rename]\nshortcut_1 = 'x'",
	} {
		t.Run(invalid, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "settings.toml")
			original := "appearance.autocolor.lsh.lightness = 0.42\n" + invalid + "\n"
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			catalogue := bindingSettingsCatalogue()
			values, err := settings.LoadAndRepair(dir, catalogue)
			if !errors.Is(err, settings.ErrRepaired) || values.Get(catalogue, settingLightness).Number != 0.42 {
				t.Fatal("invalid binding was not repaired or lost another setting", err)
			}
			backup, err := os.ReadFile(path + ".bak")
			if err != nil || string(backup) != original {
				t.Fatal("recovery lost the original settings document", err)
			}
			if _, err := settings.Load(dir, catalogue); err != nil {
				t.Fatal("repaired settings did not load", err)
			}
		})
	}
}

func TestBindingSwapLoadsAsOneSet(t *testing.T) {
	dir := t.TempDir()
	document := "[keybindings.entry.copy]\nshortcut_1 = 'Ctrl+X'\n[keybindings.entry.cut]\nshortcut_1 = 'Ctrl+C'\n"
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(document), 0600); err != nil {
		t.Fatal(err)
	}
	values, err := settings.LoadAndRepair(dir, bindingSettingsCatalogue())
	if err != nil {
		t.Fatal("valid swap was rejected while only one side was loaded", err)
	}
	v := newPreviewView()
	v.restoreSettings(values, nil)
	for _, c := range v.commandCatalogue() {
		if c.id == "entry.copy" && c.shortcut() != "Ctrl+X" || c.id == "entry.cut" && c.shortcut() != "Ctrl+C" {
			t.Fatal("swap lost a binding", c.id, c.shortcut())
		}
	}
}

func TestConflictingSaveDoesNotChangeFileOrCaller(t *testing.T) {
	catalogue := bindingSettingsCatalogue()
	var values settings.Values
	field := bindingSettingKey("entry.rename", 0)
	if err := values.Set(catalogue, field, settings.Value{Text: "Ctrl+C"}); err != nil {
		t.Fatal(err)
	}
	if err := values.Set(catalogue, bindingSettingKey("entry.copy", 0), settings.Value{Text: "Ctrl+C"}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.toml")
	if err := os.WriteFile(path, []byte("# existing\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := catalogue.Save(dir, values); !errors.Is(err, settings.ErrValue) {
		t.Fatal("conflicting bindings were saved", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "# existing") || !values.Overridden(field) {
		t.Fatal("failed save changed the file or caller", err)
	}
}

func TestAltBindingUpdatesWindowsInputAndResets(t *testing.T) {
	v := newPreviewView()
	v.nativeKeys = &nativeKeyInput{invalidate: func() {}}
	catalogue := bindingSettingsCatalogue()
	field := bindingSettingKey("view.next_metadata_column", 0)
	if err := v.settingsValues.Set(catalogue, field, settings.Value{Text: "Alt+Y"}); err != nil {
		t.Fatal(err)
	}
	v.applySettings()
	send, _ := panelInputFrames(v)
	before := v.panels[0].metadata
	send(modified("Y", key.ModAlt))
	if v.panels[0].metadata != before.cycle(1) || !v.nativeKeys.suppressesAltText('y') || !v.nativeKeys.suppressesAltText('Y') || v.nativeKeys.suppressesAltText(' ') {
		t.Fatal("Alt binding lost its action, case handling or native character suppression")
	}
	v.settingsValues.Reset(field)
	v.applySettings()
	if v.nativeKeys.suppressesAltText('y') {
		t.Fatal("reset retained old Alt character suppression")
	}
}

func TestNewDefaultBindingPreservesSavedAssignments(t *testing.T) {
	original := commands
	base := slices.DeleteFunc(slices.Clone(commands), func(c command) bool { return c.id == "programs.terminal_here" })
	t.Cleanup(func() { commands = original })
	launches := 0
	terminal := cmd("programs.terminal_here", "Programs", "Terminal here", "T", key.ModCtrl, nil)
	terminal.bindings[1].Default = shortcut{Name: "T", Modifiers: key.ModCtrl | key.ModShift}
	terminal.execute = func(*view) { launches++ }

	for _, defaultFirst := range []bool{false, true} {
		commands = append([]command{terminal}, base...)
		if !defaultFirst {
			commands = append(append([]command{}, base...), terminal)
		}
		for _, tc := range []struct {
			name, document string
			terminalOwnsT  bool
		}{
			{"fresh", "", true},
			{"saved primary", "[keybindings.view.next_metadata_column]\nshortcut_1 = 'Ctrl+T'\n", false},
			{"saved secondary", "[keybindings.view.next_metadata_column]\nshortcut_2 = 'Ctrl+T'\n", false},
		} {
			t.Run(tc.name+"/first_command="+commands[0].id, func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, "settings.toml")
				document := tc.document + "[keybindings.entry.rename]\nshortcut_1 = 'Ctrl+Y'\n"
				if err := os.WriteFile(path, []byte(document), 0600); err != nil {
					t.Fatal(err)
				}
				catalogue := bindingSettingsCatalogue()
				loaded, err := settings.LoadAndRepair(dir, catalogue)
				if err != nil {
					t.Fatal("upgrade treated saved bindings as invalid", err)
				}
				data, err := os.ReadFile(path)
				if err != nil || string(data) != document {
					t.Fatal("upgrade rewrote the user's settings", err)
				}
				if _, err := os.Stat(path + ".bak"); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("upgrade triggered settings recovery", err)
				}
				for restart := range 2 {
					v := newPreviewView()
					v.restoreSettings(loaded, nil)
					resolved := v.resolveCommandBindings(terminal)
					if (resolved.effectiveShortcut(0).Name != "") != tc.terminalOwnsT || resolved.effectiveShortcut(1) != terminal.bindings[1].Default {
						t.Fatal("wrong terminal slots were unassigned", resolved.shortcut(), restart)
					}
					if loaded.Get(catalogue, bindingSettingKey("entry.rename", 0)).Text != "Ctrl+Y" {
						t.Fatal("unrelated custom binding was lost")
					}
					launches = 0
					send, _ := panelInputFrames(v)
					before := v.panels[0].metadata
					send(modified("T", key.ModCtrl))
					if tc.terminalOwnsT {
						if launches != 1 || v.panels[0].metadata != before {
							t.Fatal("fresh install did not execute the default")
						}
					} else if launches != 0 || v.panels[0].metadata != before.cycle(1) {
						t.Fatal("upgrade did not preserve custom Ctrl+T dispatch")
					}
					if err := catalogue.Save(dir, loaded); err != nil {
						t.Fatal(err)
					}
					loaded, err = settings.LoadAndRepair(dir, catalogue)
					if err != nil {
						t.Fatal("saved upgrade did not reload", err)
					}
				}
			})
		}
	}
}

func TestSavedAssignmentCanReplaceGeneratedDefault(t *testing.T) {
	catalogue := bindingSettingsCatalogue()
	values, err := settings.Parse([]byte("[keybindings.entry.rename]\nshortcut_1 = 'Alt+1'\n"), catalogue)
	if err != nil {
		t.Fatal(err)
	}
	v := view{settingsValues: values}
	if got := v.resolveCommandBindings(panelFocusCommand(1)); got.shortcut() != "" {
		t.Fatal("generated default kept the saved assignment's shortcut", got.shortcut())
	}
	if got := v.resolveCommandBindings(panelFocusCommand(2)); got.shortcut() != "Alt+2" {
		t.Fatal("unrelated generated default changed", got.shortcut())
	}
}
