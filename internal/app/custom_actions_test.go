package app

import (
	"image"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
)

func customActionRow(id, name, target, args string) map[string]any {
	return map[string]any{"id": id, "name": name, "target": target, "mode": settings.ActionSelected, "ext": []string{"*"}, "path": "tool.exe", "args": args, "terminal": false}
}

func TestCustomActionPaletteAndShortcutUseOnlyOriginatingPanelMarks(t *testing.T) {
	for _, token := range []string{settings.EntryToken, settings.EntriesToken} {
		t.Run(token, func(t *testing.T) {
			v, root := compareView(t, 3)
			row := customActionRow("send", "Send test", settings.ActionFiles, "--send "+token)
			row["terminal"] = true
			v.settingsValues.SetCollection(settings.ActionCollection, []map[string]any{row})
			p := &v.panels[0]
			for _, name := range []string{"alpha file.txt", "beta file.md"} {
				selectName(t, p, name)
				p.toggleMark(p.selected)
			}
			selectName(t, p, "gamma file.txt") // Marks must override the current row.
			for pi := 1; pi < len(v.panels); pi++ {
				v.panels[pi].toggleMark(v.panels[pi].selected)
			}
			calls := make(chan []string, 3)
			v.loader.launchAction = func(program string, args []string, directory string, terminal bool) error {
				if program != "tool.exe" || directory != root || !terminal {
					t.Error("worker lost launch configuration", program, directory, terminal)
				}
				calls <- args
				return nil
			}
			setTestShortcut(t, v, customActionCommandPrefix+"send", 0, "Ctrl+Alt+J")
			send, _ := panelInputFrames(v)
			marks, selected, position := maps.Clone(p.marks), p.selected, p.list.Position
			for pass := range 2 {
				if pass == 0 {
					send(pressed("`"))
					send(key.EditEvent{Text: "Send test"})
					send(pressed(key.NameReturn))
				} else {
					send(modified("J", key.ModCtrl|key.ModAlt))
				}
				waitLoaded(t, v, 0)
				want := [][]string{{"--send", filepath.Join(root, "alpha file.txt"), filepath.Join(root, "beta file.md")}}
				if token == settings.EntryToken {
					want = [][]string{{"--send", filepath.Join(root, "alpha file.txt")}, {"--send", filepath.Join(root, "beta file.md")}}
				}
				if len(calls) != len(want) {
					t.Fatal("wrong invocation count", len(calls), len(want))
				}
				for _, args := range want {
					if got := <-calls; !slices.Equal(got, args) {
						t.Fatal("wrong target set or order", got, args)
					}
				}
				if !v.interaction.idle() || v.mutationsBusy() || v.active != 0 || !maps.Equal(marks, p.marks) || p.selected != selected || p.list.Position != position || p.feedback.Current() != nil {
					t.Fatal("action changed browsing state or produced success feedback")
				}
			}
		})
	}
}

func TestCustomActionAvailabilityAndFolderWorkingDirectory(t *testing.T) {
	v, root := compareView(t, 3)
	p := &v.panels[0]
	rows := []map[string]any{
		customActionRow("files", "Files", settings.ActionFiles, settings.EntriesToken),
		customActionRow("folders", "Folders", settings.ActionFolders, settings.EntryToken),
		customActionRow("both", "Both", settings.ActionEntries, settings.EntriesToken),
		customActionRow("here", "Here", settings.ActionFolders, "--here"),
	}
	rows[3]["mode"] = settings.ActionHere
	v.settingsValues.SetCollection(settings.ActionCollection, rows)
	allowed := func(id string) bool {
		for _, c := range v.customActionCommands() {
			if c.id == customActionCommandPrefix+id {
				return c.allowed(v)
			}
		}
		return false
	}
	if !allowed("files") || allowed("folders") || !allowed("here") {
		t.Fatal("file availability incorrect")
	}
	selectName(t, p, "directory")
	if allowed("files") || !allowed("folders") {
		t.Fatal("folder availability incorrect")
	}
	p.toggleMark(p.selected)
	selectName(t, p, "alpha file.txt")
	p.toggleMark(p.selected)
	if !allowed("files") || !allowed("folders") || !allowed("both") || !allowed("here") {
		t.Fatal("mixed selection did not supply the configured entry kinds")
	}
	rows[0]["ext"] = []string{"md"}
	v.settingsValues.SetCollection(settings.ActionCollection, rows)
	p.marks = nil
	if allowed("files") {
		t.Fatal("extension restriction ignored")
	}
	selectName(t, p, "beta file.md")
	if !allowed("files") {
		t.Fatal("matching extension rejected")
	}
	calls := make(chan string, 2)
	v.loader.launchAction = func(_ string, _ []string, directory string, _ bool) error {
		calls <- directory
		return nil
	}
	selectName(t, p, "directory")
	v.runCustomAction("folders")
	waitLoaded(t, v, 0)
	if directory := <-calls; directory != filepath.Join(root, "directory") {
		t.Fatal("folder action did not start inside selected folder", directory)
	}
	v.runCustomAction("here")
	v.active = 2
	waitLoaded(t, v, 0)
	if directory := <-calls; directory != root || v.active != 2 {
		t.Fatal("current-folder action followed selected folder or changed panel focus", directory, v.active)
	}
	v.active = 0
	p.drives = true
	if allowed("here") {
		t.Fatal("action available on Home")
	}
	p.drives = false
	v.loader.busy.Store(1)
	if allowed("here") {
		t.Fatal("action ignored worker busy policy")
	}
	v.loader.busy.Store(0)
}

func TestFolderActionModesResolvePanelTargets(t *testing.T) {
	root := t.TempDir()
	p := panel{path: root, items: []entry{
		{file: browser.Entry{Name: "..", Directory: true, Parent: true}},
		{file: browser.Entry{Name: "a.txt"}},
		{file: browser.Entry{Name: "one", Directory: true}},
		{file: browser.Entry{Name: "b.bin"}},
		{file: browser.Entry{Name: "two", Directory: true}},
	}}
	for _, tc := range []struct {
		name     string
		selected int
		marks    []int
		folders  []int
	}{
		{"file", 1, nil, nil},
		{"folder", 2, nil, []int{2}},
		{"multiple folders", 1, []int{2, 4}, []int{2, 4}},
		{"mixed", 0, []int{1, 2, 3, 4}, []int{2, 4}},
		{"multiple files", 2, []int{1, 3}, nil},
		{"parent", 0, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p.selected, p.marks = tc.selected, map[int]bool{}
			for _, row := range tc.marks {
				p.marks[row] = true
			}
			for _, mode := range []string{settings.ActionSelected, settings.ActionHere, settings.ActionAuto} {
				rule := settings.ActionRule{Target: settings.ActionFolders, Mode: mode, Extensions: []string{"zip"}}
				var want []customActionTarget
				if mode == settings.ActionHere || mode == settings.ActionAuto && len(tc.folders) == 0 {
					want = []customActionTarget{{root, true}}
				} else {
					for _, row := range tc.folders {
						want = append(want, customActionTarget{filepath.Join(root, p.items[row].file.Name), true})
					}
				}
				if got := p.customActionTargets(rule); !slices.Equal(got, want) {
					t.Fatal("folder mode resolved the wrong paths", mode, got, want)
				}
			}
		})
	}
	// Both ignores even a saved Here mode and restrictive file extensions.
	p.selected, p.marks = 1, map[int]bool{1: true, 2: true, 3: true, 4: true}
	got := p.customActionTargets(settings.ActionRule{Target: settings.ActionEntries, Mode: settings.ActionHere, Extensions: []string{"zip"}})
	if len(got) != 4 || got[0].path != filepath.Join(root, "a.txt") || got[2].path != filepath.Join(root, "b.bin") {
		t.Fatal("Both used Here or applied file extensions", got)
	}
}

func TestActionSettingsKeepSixRowsAndRetainInactiveValues(t *testing.T) {
	v := newPreviewView()
	row := customActionRow("explorer", "Open in Explorer", settings.ActionFolders, settings.EntryToken)
	row["mode"], row["ext"] = settings.ActionAuto, []string{"zip"}
	v.settingsValues.SetCollection(settings.ActionCollection, []map[string]any{row})
	dir := t.TempDir()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	v.startCollectionWidget(settings.ActionCollection, 0, "Open in Explorer")
	s := v.settingsInteraction()
	send, _ := panelInputFrames(v)
	s.selected = 1
	var size image.Point
	measure := func() {
		t.Helper()
		var ops op.Ops
		dims := v.layoutSettingsWidget(layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(960, 540))})
		if size == (image.Point{}) {
			size = dims.Size
		}
		if len(s.fields) != 6 || dims.Size != size {
			t.Fatal("changing action type changed dialog footprint", len(s.fields), dims.Size, size)
		}
	}
	measure()
	if s.fields[2].Key != settings.ActionCollection+".mode" || v.settingText(s.fields[2].Key) != settings.ActionAuto {
		t.Fatal("folder mode row missing")
	}
	send(pressed(key.NameRightArrow)) // Folders -> Files
	measure()
	if s.fields[2].Key != settings.ActionCollection+".ext" || v.settingText(s.fields[2].Key) != "zip" {
		t.Fatal("extension row was not restored")
	}
	if err := v.setWidgetValue(settings.ActionCollection+".ext", settings.Value{Text: "md"}); err != nil {
		t.Fatal(err)
	}
	send(pressed(key.NameRightArrow)) // Files -> Both
	measure()
	if s.fields[2].Key != "" || v.focusSettingsField(layout.Context{}, s, 2) {
		t.Fatal("Both exposed an interactive third row")
	}
	send(pressed(key.NameDownArrow))
	if s.selected != 3 {
		t.Fatal("Down focused the empty row")
	}
	send(pressed(key.NameUpArrow))
	if s.selected != 1 {
		t.Fatal("Up focused the empty row")
	}
	send(pressed(key.NameTab))
	if s.selected != 3 {
		t.Fatal("Tab focused the empty row")
	}
	send(modified(key.NameTab, key.ModShift))
	if s.selected != 1 {
		t.Fatal("Shift+Tab focused the empty row")
	}
	send(pressed(key.NameLeftArrow)) // Both -> Files
	if v.settingText(s.fields[2].Key) != "md" {
		t.Fatal("switching to Both discarded extensions")
	}
	send(pressed(key.NameLeftArrow)) // Files -> Folders
	if v.settingText(s.fields[2].Key) != settings.ActionAuto {
		t.Fatal("switching applicability discarded folder mode")
	}
	send(pressed(key.NameRightArrow), pressed(key.NameRightArrow)) // Both, with inactive values retained.
	send(modified(key.NameReturn, key.ModCtrl))
	loaded, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil || len(loaded.ActionRules()) != 1 || loaded.ActionRules()[0].Mode != settings.ActionAuto || loaded.ActionRules()[0].Extensions[0] != "md" || loaded.ActionRules()[0].Target != settings.ActionEntries {
		t.Fatal("save/reload lost inactive values", loaded.ActionRules(), err)
	}
}

func TestCustomActionStopsOnFirstLaunchFailure(t *testing.T) {
	v, _ := compareView(t, 1)
	p := &v.panels[0]
	for _, name := range []string{"alpha file.txt", "beta file.md", "gamma file.txt"} {
		selectName(t, p, name)
		p.toggleMark(p.selected)
	}
	v.settingsValues.SetCollection(settings.ActionCollection, []map[string]any{customActionRow("fail", "Fail", settings.ActionFiles, settings.EntryToken)})
	calls := 0
	started, release := make(chan struct{}), make(chan struct{})
	v.loader.launchAction = func(string, []string, string, bool) error {
		calls++
		if calls == 1 {
			close(started)
			<-release
		}
		if calls == 2 {
			return os.ErrPermission
		}
		return nil
	}
	v.runCustomAction("fail")
	<-started
	if v.requestClose() {
		close(release)
		t.Fatal("close accepted during launch")
	}
	close(release)
	waitLoaded(t, v, 0)
	message := p.feedback.Current()
	if calls != 2 || message == nil || message.Kind != feedback.Error || message.Text != "Some actions failed to start" || v.mutationsBusy() {
		t.Fatal("partial failure was not reconciled", calls, message)
	}
}

func TestCustomActionSettingsAndRunBindingsRetainIdentity(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@actions new"})
	send(pressed(key.NameReturn))
	s := v.settingsInteraction()
	if s == nil || s.collection != settings.ActionCollection || len(s.fields) != 6 {
		t.Fatal("custom action settings missing")
	}
	send(pressed(key.NameReturn))
	send(key.EditEvent{Text: "Send test"})
	send(pressed(key.NameTab))
	if err := v.setWidgetValue(settings.ActionCollection+".path", settings.Value{Text: "tool.exe"}); err != nil {
		t.Fatal(err)
	}
	send(modified(key.NameReturn, key.ModCtrl))
	rules := v.settingsValues.ActionRules()
	if len(rules) != 1 || rules[0].Name != "Send test" || !v.interaction.idle() {
		t.Fatal("action was not created", rules)
	}
	id := rules[0].ID
	setTestShortcut(t, v, customActionCommandPrefix+id, 0, "Ctrl+Alt+J")
	setTestShortcut(t, v, "settings."+settings.ActionCollection+".rules."+id, 0, "Ctrl+Alt+K")
	rows := v.settingsValues.Collection(settings.ActionCollection)
	rows[0]["name"] = "Renamed action"
	rows = append([]map[string]any{customActionRow("other", "Other", settings.ActionFiles, settings.EntryToken)}, rows...)
	v.settingsValues.SetCollection(settings.ActionCollection, rows)
	dir := t.TempDir()
	catalogue := bindingSettingsCatalogue()
	if err := catalogue.Save(dir, v.settingsValues); err != nil {
		t.Fatal(err)
	}
	loaded, err := settings.Load(dir, catalogue)
	if err != nil {
		t.Fatal(err)
	}
	v.restoreSettings(loaded, nil)
	send(modified("K", key.ModCtrl|key.ModAlt))
	if s := v.settingsInteraction(); s == nil || s.item != 1 || s.title != "Programs › Actions › Renamed action" {
		t.Fatal("settings binding lost action identity")
	}
	send(pressed(key.NameEscape))
	send(pressed("`"))
	send(key.EditEvent{Text: "Renamed action"})
	p := v.paletteInteraction()
	if p == nil || len(p.results) != 1 || p.results[0].command.shortcut() != "Ctrl+Alt+J" {
		t.Fatal("run command lost its binding after rename/reorder/reload")
	}
	send(modified("E", key.ModCtrl))
	if s := v.settingsInteraction(); s == nil || s.bindingCommand == nil || s.bindingCommand.id != customActionCommandPrefix+id {
		t.Fatal("run command cannot use shared shortcut editor")
	}
	send(pressed(key.NameEscape))
	send(pressed(key.NameEscape))
	if !v.interaction.idle() {
		t.Fatal("shortcut editing did not return through palette to browsing")
	}
	v.startCollectionWidget(settings.ActionCollection, 1, "Renamed action")
	v.activateSettingsAction(layout.Context{}, 2)
	v.finishSettingsWidget(layout.Context{}, true)
	if len(v.settingsValues.ActionRules()) != 1 || v.settingsValues.Overridden(bindingSettingKey(customActionCommandPrefix+id, 0)) || v.settingsValues.Overridden(bindingSettingKey("settings."+settings.ActionCollection+".rules."+id, 0)) {
		t.Fatal("deleting action left stale commands/bindings", v.settingsValues.ActionRules(), v.settingsInteraction())
	}
}

func TestCustomActionPreviews(t *testing.T) {
	if os.Getenv("COMMANDO_ACTION_PREVIEWS") != "1" {
		t.Skip("set COMMANDO_ACTION_PREVIEWS=1 to render custom action settings and palette")
	}
	dir := filepath.Join("..", "..", "tmp", "agent", "screenshots")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		file, target, mode, name, program string
		size                              image.Point
	}{
		{"action-files", settings.ActionFiles, settings.ActionSelected, "Open with 7-Zip", "7zFM.exe", image.Pt(960, 540)},
		{"action-folders", settings.ActionFolders, settings.ActionAuto, "Open in Explorer", "explorer.exe", image.Pt(960, 540)},
		{"action-both", settings.ActionEntries, settings.ActionAuto, "Open entries", "tool.exe", image.Pt(960, 540)},
		{"action-short", settings.ActionFolders, settings.ActionAuto, "Open in Explorer", "explorer.exe", image.Pt(800, 220)},
	} {
		v := newPreviewView()
		row := customActionRow("preview", tc.name, tc.target, settings.EntryToken)
		row["mode"], row["path"] = tc.mode, tc.program
		v.settingsValues.SetCollection(settings.ActionCollection, []map[string]any{row})
		v.startCollectionWidget(settings.ActionCollection, 0, tc.name)
		if err := renderSizedViewPNG(filepath.Join(dir, tc.file+".png"), v, tc.size); err != nil {
			t.Fatal(err)
		}
	}
	v := newPreviewView()
	v.settingsValues.SetCollection(settings.ActionCollection, []map[string]any{customActionRow("archive", "Open with 7-Zip", settings.ActionFiles, settings.EntryToken)})
	v.active = 1
	selectName(t, &v.panels[1], "go-mono.zip")
	v.panels[1].marks = nil
	v.loader = &directoryLoader{requests: make(chan loadRequest, 4), done: make(chan struct{})}
	v.openPaletteQuery(layout.Context{}, "Open with 7-Zip")
	if s := v.paletteInteraction(); s == nil || len(s.results) != 1 || !s.results[0].command.allowed(v) {
		t.Fatal("action preview is unavailable on the sample file")
	}
	if err := renderSizedViewPNG(filepath.Join(dir, "action-palette.png"), v, image.Pt(960, 540)); err != nil {
		t.Fatal(err)
	}
}
