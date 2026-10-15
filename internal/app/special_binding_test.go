package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
)

func setTestShortcut(t *testing.T, v *view, id string, slot int, text string) {
	t.Helper()
	if err := v.settingsValues.Set(bindingSettingsCatalogue(), bindingSettingKey(id, slot), settings.Value{Text: text}); err != nil {
		t.Fatal(err)
	}
}

func TestFilterBindingChangesInvocationAndRetainsQuery(t *testing.T) {
	v := newPreviewView()
	v.panels[0].setFilter("go")
	setTestShortcut(t, v, "view.filter_files", 0, "Ctrl+F")
	send, _ := panelInputFrames(v)
	send(pressed("/"), key.EditEvent{Text: "/"})
	if v.filterInteraction() != nil || v.panels[0].prefix != "/" {
		t.Fatal("old slash shortcut did not return to filename typing")
	}
	send(modified("F", key.ModCtrl))
	if v.filterInteraction() == nil || v.filterEditor.Text() != "go" || v.filterEditor.SelectedText() != "go" {
		t.Fatal("replacement filter shortcut lost the existing query or selection")
	}
	send(pressed(key.NameReturn))
	v.settingsValues.Reset(bindingSettingKey("view.filter_files", 0))
	send(modified("F", key.ModCtrl))
	if v.filterInteraction() != nil {
		t.Fatal("reset retained the replacement shortcut")
	}
	send(pressed("/"), key.EditEvent{Text: "/"})
	if v.filterInteraction() == nil || v.filterEditor.Text() != "go" || v.filterEditor.SelectedText() != "go" {
		t.Fatal("reset slash inserted its invocation text or lost the query")
	}
}

func TestSettingsBindingKeepsPalettePrefixSyntaxIndependent(t *testing.T) {
	v := newPreviewView()
	setTestShortcut(t, v, "application.settings", 0, "Ctrl+,")
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@font"})
	if v.paletteInteraction() != nil || v.panels[0].prefix != "@font" {
		t.Fatal("old at-sign shortcut still opened settings")
	}
	send(modified(",", key.ModCtrl))
	p := v.paletteInteraction()
	if p == nil || !p.settingsMode || v.commandSearch.Text() != "@" {
		t.Fatal("replacement settings shortcut did not seed the fixed @ prefix")
	}
	send(replacePalette(v, ""))
	send(key.EditEvent{Text: "@font"})
	if v.paletteInteraction() != p || !p.settingsMode || p.query != "@font" {
		t.Fatal("rebinding settings changed the palette's @ syntax")
	}
	send(pressed(key.NameEscape))
	setTestShortcut(t, v, "application.settings", 0, "none")
	send(modified(",", key.ModCtrl))
	if v.paletteInteraction() != nil {
		t.Fatal("cleared settings shortcut still opened the palette")
	}
	send(pressed("`"))
	send(key.EditEvent{Text: "@"})
	if !v.paletteInteraction().settingsMode {
		t.Fatal("clearing a browsing shortcut disabled palette syntax")
	}
	send(pressed(key.NameEscape))
	v.settingsValues.Reset(bindingSettingKey("application.settings", 0))
	send(key.EditEvent{Text: "@font"})
	if p = v.paletteInteraction(); p == nil || !p.settingsMode || p.query != "@font" {
		t.Fatal("reset did not restore @ invocation and its initial search")
	}
}

func TestPaletteBindingsReplaceBothDefaultsAndToggle(t *testing.T) {
	v := newPreviewView()
	setTestShortcut(t, v, "application.command_palette", 0, "none")
	setTestShortcut(t, v, "application.command_palette", 1, "Ctrl+K")
	send, _ := panelInputFrames(v)
	send(pressed("`"), key.EditEvent{Text: "`"})
	if v.paletteInteraction() != nil || v.panels[0].prefix != "`" {
		t.Fatal("cleared backtick binding still invoked the palette")
	}
	send(modified("P", key.ModCtrl|key.ModShift))
	if v.paletteInteraction() != nil {
		t.Fatal("replaced Ctrl+Shift+P still invoked the palette")
	}
	send(modified("K", key.ModCtrl))
	p := v.paletteInteraction()
	if p == nil || v.commandSearch.Text() != "" {
		t.Fatal("replacement palette shortcut did not open a blank search")
	}
	send(modified("P", key.ModCtrl|key.ModShift))
	if v.paletteInteraction() != p {
		t.Fatal("old palette shortcut still closed the palette")
	}
	send(key.EditEvent{Text: "`"})
	if p.query != "`" {
		t.Fatal("modified invocation swallowed subsequent search text")
	}
	send(modified("K", key.ModCtrl))
	if v.paletteInteraction() != nil {
		t.Fatal("replacement palette shortcut did not close the palette")
	}
	setTestShortcut(t, v, "application.command_palette", 1, "Ctrl+`")
	send(modified("`", key.ModCtrl))
	send(key.EditEvent{Text: "`"})
	if p = v.paletteInteraction(); p == nil || p.query != "`" {
		t.Fatal("modified backtick invocation swallowed later search text")
	}
	send(pressed(key.NameEscape))
	v.settingsValues.Reset(bindingSettingKey("application.command_palette", 0))
	v.settingsValues.Reset(bindingSettingKey("application.command_palette", 1))
	send(pressed("`"), key.EditEvent{Text: "`"})
	if v.paletteInteraction() == nil || v.commandSearch.Text() != "" {
		t.Fatal("reset did not restore clean backtick invocation")
	}
}

func TestSpecialBindingOverridesSurviveRestart(t *testing.T) {
	v := newPreviewView()
	want := map[string]string{
		bindingSettingKey("application.command_palette", 0): "Ctrl+K",
		bindingSettingKey("application.command_palette", 1): "Ctrl+Shift+K",
		bindingSettingKey("application.settings", 0):        "Ctrl+,",
		bindingSettingKey("view.filter_files", 0):           "Ctrl+F",
		bindingSettingKey("entry.paste", 1):                 "Ctrl+Shift+V",
		bindingSettingKey("entry.context_menu", 0):          "Ctrl+Y",
		bindingSettingKey("entry.context_menu", 1):          "none",
	}
	catalogue := bindingSettingsCatalogue()
	for field, text := range want {
		if err := v.settingsValues.Set(catalogue, field, settings.Value{Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	if err := catalogue.Save(dir, v.settingsValues); err != nil {
		t.Fatal(err)
	}
	loaded, err := settings.LoadAndRepair(dir, catalogue)
	if err != nil {
		t.Fatal("special binding overrides were repaired instead of restored", err)
	}
	for field, text := range want {
		if !loaded.Overridden(field) || loaded.Get(catalogue, field).Text != text {
			t.Fatal("binding override did not survive restart", field)
		}
	}
}

func TestPaletteInvocationBindingsRespectTextEditingAndCtrlE(t *testing.T) {
	v := newPreviewView()
	setTestShortcut(t, v, "entry.paste", 0, "none")
	setTestShortcut(t, v, "application.command_palette", 0, "Ctrl+V")
	setTestShortcut(t, v, "application.command_palette", 1, "Ctrl+E")
	router, send := clipboardFrames(v)
	send(modified("V", key.ModCtrl))
	p := v.paletteInteraction()
	if p == nil {
		t.Fatal("browsing binding did not open the palette")
	}
	send(modified("V", key.ModCtrl))
	if !router.ClipboardRequested() || v.paletteInteraction() != p {
		t.Fatal("palette binding stole text-field paste")
	}
	send(clipboardData("metadata"))
	if p.query != "metadata" {
		t.Fatal("text-field paste was not preserved")
	}
	send(modified("E", key.ModCtrl))
	if v.settingsInteraction() == nil || v.settingsInteraction().bindingCommand == nil {
		t.Fatal("palette binding stole fixed Ctrl+E editing")
	}
}

func TestNativePasteAliasCanBeReplacedWithoutChangingTextPaste(t *testing.T) {
	v, source, target := clipboardView(t)
	c := attachTestFileClipboard(v)
	c.data = browser.FileClipboard{Paths: []string{filepath.Join(source, "a.txt")}, Sequence: 1}
	v.active = 1
	v.nativeKeys = &nativeKeyInput{invalidate: func() {}}
	setTestShortcut(t, v, "entry.paste", 1, "Ctrl+Shift+V")
	send, _ := panelInputFrames(v)
	v.nativeKeys.push(modified(insertKey, key.ModShift))
	send()
	if v.panels[1].loading || v.transferPending {
		t.Fatal("old native paste alias still dispatched a transfer")
	}
	if _, err := os.Stat(filepath.Join(target, "a.txt")); !os.IsNotExist(err) {
		t.Fatal("old native paste alias copied a file", err)
	}
	send(modified("V", key.ModCtrl|key.ModShift))
	waitLoaded(t, v, 1)
	data, err := os.ReadFile(filepath.Join(target, "a.txt"))
	if err != nil || string(data) != "alpha" {
		t.Fatal("replacement paste alias did not use the shared transfer flow", err)
	}
	send(modified("D", key.ModAlt))
	v.location.SetText("")
	send()
	v.nativeKeys.push(modified(insertKey, key.ModShift))
	send()
	send(clipboardData("text paste"))
	if v.location.Text() != "text paste" {
		t.Fatal("file-paste rebind disabled native text-field paste")
	}
}

func TestNativeMenuAliasCanBeReplaced(t *testing.T) {
	v := menuTestView()
	v.nativeKeys = &nativeKeyInput{invalidate: func() {}}
	setTestShortcut(t, v, "entry.context_menu", 0, "Ctrl+Y")
	setTestShortcut(t, v, "entry.context_menu", 1, "Ctrl+Shift+Y")
	send, _ := panelInputFrames(v)
	v.nativeKeys.push(pressed(contextMenuKey))
	send()
	send(modified("/", key.ModAlt))
	if !v.interaction.idle() {
		t.Fatal("old context-menu shortcuts still invoked a native session")
	}
	want := v.panels[0].selectedPaths()
	send(modified("Y", key.ModCtrl|key.ModShift))
	s, ok := v.interaction.current.(*nativeSession)
	if !ok || !reflect.DeepEqual(s.request.paths, want) || s.request.options.ClientPoint == nil {
		t.Fatal("replacement shortcut lost marked items or the keyboard menu anchor")
	}
}

func TestShortcutCaptureUsesTypedSymbolsForSpecialBindings(t *testing.T) {
	for _, tc := range []struct {
		query, text string
		press       key.Event
	}{
		{"Application Settings", "@", modified("2", key.ModShift)},
		{"Filter files", "/", pressed("/")},
		{"Command palette", "`", pressed("`")},
	} {
		t.Run(tc.query, func(t *testing.T) {
			v := newPreviewView()
			send, _ := panelInputFrames(v)
			_, s := openShortcutWidget(t, v, send, tc.query)
			send(pressed(key.NameReturn))
			send(tc.press, key.EditEvent{Text: tc.text})
			if s.capturing || v.settingText(s.fields[0].Key) != tc.text || v.commandSearch.Text() != tc.query {
				t.Fatal("capture lost the typed symbol or changed the palette query", s.errorText)
			}
		})
	}
}

func TestShortcutCaptureWhitespaceDoesNotRestoreDefault(t *testing.T) {
	v := newPreviewView()
	setTestShortcut(t, v, "view.next_metadata_column", 0, "Ctrl+Y")
	send, _ := panelInputFrames(v)
	_, s := openShortcutWidget(t, v, send, "Next metadata column")
	send(pressed(key.NameReturn))
	send(pressed(key.NameSpace), key.EditEvent{Text: " "})
	if !s.capturing || s.dirty || v.settingText(s.fields[0].Key) != "Ctrl+Y" {
		t.Fatal("whitespace text was accepted as an inherited binding")
	}
}
