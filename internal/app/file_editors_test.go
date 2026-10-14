package app

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
)

func setEditorRule(t *testing.T, v *view, extensions, program string) {
	t.Helper()
	var item settings.Values
	for key, text := range map[string]string{"ext": extensions, "path": program, "args": settings.SelectedFilesToken} {
		if err := item.Set(settings.CatalogueDefault, settings.EditorCollection+"."+key, settings.Value{Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	row := item.Item(settings.EditorCollection)
	if err := settings.NormalizeEditor(row); err != nil {
		t.Fatal(err)
	}
	v.settingsValues.SetCollection(settings.EditorCollection, append(v.settingsValues.Collection(settings.EditorCollection), row))
}

func TestEditorWidgetCreatePickSaveAndDiscover(t *testing.T) {
	v := newPreviewView()
	program := filepath.Join("/programs", "editor.exe")
	v.panels[1] = panel{path: "/programs", selected: 1, items: []entry{
		{name: "folder", file: browser.Entry{Name: "folder", Directory: true}},
		{name: "editor.exe", file: browser.Entry{Name: "editor.exe"}},
	}}
	v.panels[1].rows = make(map[int]*fileRow)
	v.panels[1].list.Axis = layout.Vertical
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@editors new"})
	send(pressed(key.NameReturn))
	s := v.settingsInteraction()
	if s == nil || s.title != "Programs › Editors › New" || len(s.actionLabels()) != 2 {
		t.Fatal("New did not open editor fields with Save/Cancel")
	}
	send(pressed(key.NameReturn))
	send(key.EditEvent{Text: ".TXT md"})
	send(pressed(key.NameTab))
	if s.selected != 1 || s.editing {
		t.Fatal("Tab did not retain extensions and advance")
	}
	send(pressed(key.NameRightArrow))
	if !s.selecting || v.active != 1 || v.settingText(settings.EditorCollection+".path") != "" {
		t.Fatal("file picker changed the unconfirmed program")
	}
	v.panels[1].selected = 0
	v.updateSettingsSelection()
	if v.settingText(settings.EditorCollection+".path") != "" {
		t.Fatal("folder was accepted as executable")
	}
	v.panels[1].selected = 1
	send(pressed(key.NameReturn))
	if s.selecting || v.settingsInteraction() != s || v.settingText(settings.EditorCollection+".path") != program {
		t.Fatal("Enter did not choose the program and return to its widget")
	}
	send(modified(key.NameReturn, key.ModCtrl))
	rules := v.settingsValues.EditorRules()
	if !v.interaction.idle() || len(rules) != 1 || rules[0].Program != program || !reflect.DeepEqual(rules[0].Extensions, []string{".TXT", "md"}) {
		t.Fatalf("saved rules = %#v", rules)
	}
	send(key.EditEvent{Text: "@editors .TXT md"})
	send(pressed(key.NameReturn))
	if v.settingsInteraction() == nil || v.settingsInteraction().title != "Programs › Editors › .TXT md" || v.settingText(settings.EditorCollection+".ext") != ".TXT md" {
		t.Fatal("saved rule was not discoverable/editable")
	}
	send(pressed(key.NameReturn))
	send(key.EditEvent{Text: "go"})
	send(pressed(key.NameEscape))
	if v.settingsValues.EditorRules()[0].Extensions[0] != ".TXT" {
		t.Fatal("cancelled draft changed rule")
	}
}

func TestEditorWidgetDeleteCancelFailureAndRetry(t *testing.T) {
	v := newPreviewView()
	setEditorRule(t, v, "txt", "text.exe")
	setEditorRule(t, v, "*", "fallback.exe")
	dir := t.TempDir()
	if err := settings.Save(dir, v.settingsValues); err != nil {
		t.Fatal(err)
	}
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", os.ErrPermission }
	}
	v.startCollectionWidget(settings.EditorCollection, 0, "text.exe")
	s := v.settingsInteraction()
	v.activateSettingsAction(layout.Context{}, 2)
	if !s.deleted || len(v.settingsValues.EditorRules()) != 2 {
		t.Fatal("Delete did not stage removal")
	}
	v.finishSettingsWidget(layout.Context{}, true)
	if v.settingsInteraction() != s || s.errorText == "" {
		t.Fatal("failed deletion save closed widget")
	}
	loaded, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil || len(loaded.EditorRules()) != 2 {
		t.Fatal("failed deletion changed disk", err)
	}
	v.finishSettingsWidget(layout.Context{}, false)
	if len(v.settingsValues.EditorRules()) != 2 {
		t.Fatal("Cancel lost removed rule")
	}
	v.startCollectionWidget(settings.EditorCollection, 0, "text.exe")
	v.activateSettingsAction(layout.Context{}, 2)
	v.finishSettingsWidget(layout.Context{}, true)
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	v.finishSettingsWidget(layout.Context{}, true)
	loaded, err = settings.Load(dir, settings.CatalogueDefault)
	if err != nil || !v.interaction.idle() || len(loaded.EditorRules()) != 1 || loaded.EditorRules()[0].Program != "fallback.exe" {
		t.Fatalf("retry = %#v, %v", loaded.EditorRules(), err)
	}
}

func TestEditorWidgetInvalidDraftAndSaveRetryDoesNotDuplicate(t *testing.T) {
	v := newPreviewView()
	v.startCollectionWidget(settings.EditorCollection, -1, "New")
	s := v.settingsInteraction()
	v.finishSettingsWidget(layout.Context{}, true)
	if v.settingsInteraction() != s || s.errorText == "" || len(v.settingsValues.EditorRules()) != 0 {
		t.Fatal("invalid New rule committed")
	}
	_ = s.draft.Set(settings.CatalogueDefault, settings.EditorCollection+".ext", settings.Value{Text: "txt"})
	_ = s.draft.Set(settings.CatalogueDefault, settings.EditorCollection+".path", settings.Value{Text: "editor.exe"})
	attempts := 0
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) {
			attempts++
			if attempts == 1 {
				return "", errors.New("failure")
			}
			return "", nil
		}
	}
	v.finishSettingsWidget(layout.Context{}, true)
	v.finishSettingsWidget(layout.Context{}, true)
	if !v.interaction.idle() || len(v.settingsValues.EditorRules()) != 1 || attempts != 2 {
		t.Fatal("save retry duplicated new rule")
	}
}

func TestF4CapturesMarkedFilesAndPreservesBrowsing(t *testing.T) {
	v := newPreviewView()
	v.panels[0] = panel{path: "/files", selected: 0, marks: map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true}, items: []entry{
		{name: "current.txt", file: browser.Entry{Name: "current.txt"}},
		{name: "one.txt", file: browser.Entry{Name: "one.txt"}},
		{name: "two.txt", file: browser.Entry{Name: "two.txt"}},
		{name: "three.txt", file: browser.Entry{Name: "three.txt"}},
		{name: "four.txt", file: browser.Entry{Name: "four.txt"}},
		{name: "folder", file: browser.Entry{Name: "folder", Directory: true}},
	}}
	v.panels[0].rows = make(map[int]*fileRow)
	v.panels[0].list.Axis = layout.Vertical
	setEditorRule(t, v, "txt", "text.exe")
	v.loader = &directoryLoader{requests: make(chan loadRequest, 1)}
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF4))
	request := <-v.loader.requests
	want := []string{filepath.Join("/files", "one.txt"), filepath.Join("/files", "two.txt"), filepath.Join("/files", "three.txt"), filepath.Join("/files", "four.txt")}
	if len(request.editor.launches) != 1 || !reflect.DeepEqual(request.editor.launches[0].Arguments, want) {
		t.Fatalf("captured = %#v", request.editor)
	}
	send(pressed(key.NameRightArrow))
	v.acceptLoad(loadResult{request: request})
	if v.active != 1 || v.panels[0].selected != 0 || len(v.panels[0].marks) != 5 || v.panels[0].loading {
		t.Fatal("completion lost focus, selection or marks")
	}
	v.active = 0
	v.panels[0].marks = nil
	send(pressed(key.NameF4))
	request = <-v.loader.requests
	if !reflect.DeepEqual(request.editor.launches[0].Arguments, []string{filepath.Join("/files", "current.txt")}) {
		t.Fatal("unmarked invocation did not use current file")
	}
	v.acceptLoad(loadResult{request: request, err: os.ErrNotExist})
	if v.panels[0].feedback.Current().Text != "Cannot start editor" {
		t.Fatal("launch failure was not reported")
	}
	v.startCollectionWidget(settings.EditorCollection, -1, "New")
	v.editSelectedFiles()
	select {
	case <-v.loader.requests:
		t.Fatal("F4 escaped foreground settings")
	default:
	}
}

func TestEditorLaunchRunsThroughWorker(t *testing.T) {
	loader := newDirectoryLoader(browser.ReadLocation, nil)
	defer func() { loader.close(); <-loader.stopped }()
	var calls []settings.EditorLaunch
	loader.launchEditor = func(program string, args []string, dir string) error {
		if dir != "/files" {
			t.Errorf("wrong working folder %q", dir)
		}
		calls = append(calls, settings.EditorLaunch{Program: program, Arguments: args})
		return nil
	}
	launches := []settings.EditorLaunch{{Program: "editor", Arguments: []string{"one", "two", "three", "four"}}}
	if !loader.submit(loadRequest{path: "/files", editor: &editorRequest{launches: launches}}) {
		t.Fatal("submit failed")
	}
	result := <-loader.results
	loader.complete(result.request)
	if result.err != nil || !reflect.DeepEqual(calls, launches) || loader.busy.Load() != 0 {
		t.Fatalf("worker calls = %#v, %v", calls, result.err)
	}
}

func TestEditorWidgetPreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional headless editor preview")
	}
	v := newPreviewView()
	v.startCollectionWidget(settings.EditorCollection, -1, "New")
	if err := renderViewPNG(filepath.Join(dir, "settings-editor-new.png"), v); err != nil {
		t.Fatal(err)
	}
	v.finishSettingsWidget(layout.Context{}, false)
	setEditorRule(t, v, "txt md go", `C:\Program Files\Sublime Text\sublime_text.exe`)
	v.startCollectionWidget(settings.EditorCollection, 0, "txt md go")
	if err := renderViewPNG(filepath.Join(dir, "settings-editor-saved.png"), v); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsFilePickerFilterConfirmAndCancel(t *testing.T) {
	v := newView([]panel{{path: "/source"}, {path: "/programs"}})
	v.acceptLoad(loadResult{request: loadRequest{panel: 1}, directory: browser.Directory{Path: "/programs", Entries: []browser.Entry{
		{Name: "paintdotnet.exe"}, {Name: "paintdotnet.ico"},
	}}})
	send, _ := panelInputFrames(v)
	v.startCollectionWidget(settings.EditorCollection, -1, "New")
	s := v.settingsInteraction()
	s.selected = 1
	original := "/old/editor.exe"
	_ = s.draft.Set(settings.CatalogueDefault, settings.EditorCollection+".path", settings.Value{Text: original})
	send(pressed(key.NameRightArrow))
	send(key.EditEvent{Text: "/"})
	send(key.EditEvent{Text: "exe"})
	send(pressed(key.NameReturn))
	if !s.selecting || v.filterInteraction() != nil || v.panels[1].filter != "exe" || v.settingText(settings.EditorCollection+".path") != original {
		t.Fatalf("filter editing changed program or ended picking: selecting=%v filterSession=%v filter=%q program=%q", s.selecting, v.filterInteraction(), v.panels[1].filter, v.settingText(settings.EditorCollection+".path"))
	}
	send(pressed(key.NameReturn))
	chosen := filepath.Join("/programs", "paintdotnet.exe")
	if s.selecting || v.settingsInteraction() != s || v.settingText(settings.EditorCollection+".path") != chosen || v.panels[1].filter != "exe" {
		t.Fatal("Enter did not confirm filtered executable and return")
	}
	// Re-enter the picker, clear its filter, and highlight the icon instead.
	s.selecting = true
	v.panels[1].setFilter("")
	v.panels[1].selected = 1
	send(pressed(key.NameEscape))
	if s.selecting || v.settingsInteraction() != s || v.settingText(settings.EditorCollection+".path") != chosen {
		t.Fatal("Escape accepted an unconfirmed icon")
	}
}

func TestSettingsFilePickerEnterOpensFolderThenChoosesFile(t *testing.T) {
	v := newView([]panel{{path: "/source"}, {path: "/programs"}})
	v.acceptLoad(loadResult{request: loadRequest{panel: 1}, directory: browser.Directory{Path: "/programs", Entries: []browser.Entry{{Name: "Editor", Directory: true}}}})
	v.loader = &directoryLoader{requests: make(chan loadRequest, 1)}
	send, _ := panelInputFrames(v)
	v.startCollectionWidget(settings.EditorCollection, -1, "New")
	s := v.settingsInteraction()
	s.selected = 1
	send(pressed(key.NameRightArrow))
	send(pressed(key.NameReturn))
	var request loadRequest
	select {
	case request = <-v.loader.requests:
	default:
		t.Fatal("Enter did not navigate folder")
	}
	if request.path != filepath.Join("/programs", "Editor") || !s.selecting || v.settingText(settings.EditorCollection+".path") != "" {
		t.Fatal("folder navigation changed selected program")
	}
	v.acceptLoad(loadResult{request: request, directory: browser.Directory{Path: request.path, Entries: []browser.Entry{{Name: "editor.exe"}}}})
	send(pressed(key.NameReturn))
	if s.selecting || v.settingsInteraction() != s || v.settingText(settings.EditorCollection+".path") != filepath.Join(request.path, "editor.exe") {
		t.Fatal("Enter did not choose file after folder navigation")
	}
}
