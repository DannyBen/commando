package app

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

func compareView(t *testing.T, panels int) (*view, string) {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"alpha file.txt", "beta file.md", "gamma file.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "directory"), 0700); err != nil {
		t.Fatal(err)
	}
	v := newBrowserView(slices.Repeat([]string{root}, panels), nil)
	t.Cleanup(v.finishOperations)
	for pi := range v.panels {
		waitLoaded(t, v, pi)
		selectName(t, &v.panels[pi], "beta file.md")
	}
	selectName(t, &v.panels[0], "alpha file.txt")
	v.settingsValues.SetCollection(settings.CompareCollection, []map[string]any{
		{"id": "fallback", "ext": []string{"*"}, "path": "fallback.exe", "args": settings.FirstFileToken + " " + settings.SecondFileToken},
		{"id": "text", "ext": []string{"txt"}, "path": "text.exe", "args": "--left %FirstFile% --right %SecondFile%"},
	})
	return v, root
}

func TestCompareUsesCurrentFileAndSamePanelTargetWithAnyPanelCount(t *testing.T) {
	for _, panels := range []int{1, 2, 3} {
		t.Run(fmt.Sprint(panels), func(t *testing.T) {
			v, root := compareView(t, panels)
			p := &v.panels[0]
			selectName(t, p, "gamma file.txt")
			p.toggleMark(p.selected)
			selectName(t, p, "alpha file.txt")
			marks, selected := maps.Clone(p.marks), p.selected
			calls := make(chan settings.EditorLaunch, 1)
			v.loader.launchEditor = func(program string, args []string, dir string) error {
				if dir != root {
					t.Errorf("wrong working directory: %q", dir)
				}
				calls <- settings.EditorLaunch{Program: program, Arguments: args}
				return nil
			}
			send, _ := panelInputFrames(v)
			send(modified("V", key.ModCtrl|key.ModAlt))
			if v.inlineInteraction() == nil || len(calls) != 0 {
				t.Fatal("Compare bypassed target selection")
			}
			if panels > 1 {
				send(pressed(key.NameLeftArrow))
			}
			a := v.inlineInteraction()
			if a == nil || a.panel != 0 || p.items[a.subjectRow].file.Name != "beta file.md" || !v.interaction.protectsListing(0) {
				t.Fatal("source panel could not target a different file")
			}
			recorder := &recordedSounds{}
			v.sounds = recorder
			send(pressed(key.NameReturn))
			waitLoaded(t, v, 0)
			send()
			call := <-calls
			want := []string{"--left", filepath.Join(root, "alpha file.txt"), "--right", filepath.Join(root, "beta file.md")}
			if call.Program != "text.exe" || !slices.Equal(call.Arguments, want) || v.inlineInteraction() != nil {
				t.Fatal("Compare lost source, rule, or pair order", call)
			}
			if !maps.Equal(marks, p.marks) || selected != p.selected || !slices.Equal(recorder.played, []sound.Name{sound.MessageSuccess}) {
				t.Fatal("Compare changed marks/selection or doubled its result sound", recorder.played)
			}
		})
	}
}

func TestCompareTargetNavigationClicksAndCancellation(t *testing.T) {
	v, _ := compareView(t, 3)
	send, _ := panelInputFrames(v)
	send(modified("V", key.ModCtrl|key.ModAlt))
	send(pressed(key.NameEnd))
	a := v.inlineInteraction()
	if v.panels[a.panel].items[a.subjectRow].file.Name != "gamma file.txt" || a.row != a.subjectRow+1 {
		t.Fatal("End did not move the target and floater together")
	}
	send(pressed(key.NameHome))
	if v.panels[a.panel].items[a.subjectRow].file.Name != "beta file.md" {
		t.Fatal("Home included a folder, parent or the source file")
	}
	send(pressed(key.NameDownArrow), pressed(key.NameUpArrow), pressed(key.NamePageDown), pressed(key.NamePageUp))
	if v.panels[a.panel].items[a.subjectRow].file.Name != "beta file.md" {
		t.Fatal("vertical target movement lost its file")
	}
	// A click in another panel retargets without opening, launching, or marking.
	v.panels[0].rows[len(v.panels[0].items)-1].Click()
	send()
	a = v.inlineInteraction()
	if a == nil || a.panel != 0 || v.panels[0].items[a.subjectRow].file.Name != "gamma file.txt" {
		t.Fatal("file click cancelled instead of retargeting")
	}
	send(pressed(key.NameTab), modified(key.NameTab, key.ModShift))
	if a = v.inlineInteraction(); a == nil || a.panel != 0 || v.panels[0].items[a.subjectRow].file.Name != "gamma file.txt" {
		t.Fatal("panel cycling forgot the second cursor")
	}
	send(pressed(key.NameEscape))
	if !v.interaction.idle() || v.active != 0 || v.panels[0].items[v.panels[0].selected].file.Name != "alpha file.txt" || len(v.panels[0].marks) != 0 {
		t.Fatal("cancellation changed source selection or marks")
	}
}

func TestCompareActionClickAndLaunchFailure(t *testing.T) {
	v, _ := compareView(t, 2)
	v.loader.launchEditor = func(string, []string, string) error { return os.ErrPermission }
	send, _ := panelInputFrames(v)
	send(modified("V", key.ModCtrl|key.ModAlt))
	a := v.inlineInteraction()
	v.panels[a.panel].rows[a.row].Click()
	send()
	waitLoaded(t, v, 0)
	send()
	if v.inlineInteraction() != nil || v.panels[0].feedback.Current().Text != "Compare program access denied" || v.mutationsBusy() {
		t.Fatal("action click did not launch through the worker and reconcile failure")
	}
}

func TestCompareMissingRuleAndUnavailableTargetStaySafe(t *testing.T) {
	v, _ := compareView(t, 1)
	v.settingsValues.SetCollection(settings.CompareCollection, nil)
	send, _ := panelInputFrames(v)
	send(modified("V", key.ModCtrl|key.ModAlt))
	if !v.interaction.idle() || v.panels[0].feedback.Current().Text != "No matching compare program" {
		t.Fatal("missing program did not explain the failure")
	}
	selectName(t, &v.panels[0], "directory")
	if compareReady(v) {
		t.Fatal("folder offered file comparison")
	}
	v.settingsValues.SetCollection(settings.CompareCollection, []map[string]any{{"ext": []string{"*"}, "path": "diff.exe", "args": "%FirstFile% %SecondFile%"}})
	p := &v.panels[0]
	p.items = []entry{{file: browser.Entry{Name: "only.txt"}}}
	p.selected = 0
	send()
	send(modified("V", key.ModCtrl|key.ModAlt), pressed(key.NameReturn))
	if v.inlineInteraction() == nil || v.mutationsBusy() || p.feedback.Current().Text != "Choose another file" {
		t.Fatal("self-comparison or missing target was submitted")
	}
	send(pressed(key.NameEscape))
}

func TestCompareWidgetUsesSharedPickerSaveAndRuleBindings(t *testing.T) {
	v, root := compareView(t, 2)
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(root, values) }
	}
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: "@compare new"})
	send(pressed(key.NameReturn))
	s := v.settingsInteraction()
	if s == nil || s.collection != settings.CompareCollection || len(s.fields) != 3 {
		t.Fatal("Compare did not reuse program-rule settings")
	}
	send(pressed(key.NameReturn))
	send(key.EditEvent{Text: "png"})
	send(pressed(key.NameTab))
	send(pressed(key.NameRightArrow))
	if !s.selecting {
		t.Fatal("compare executable picker did not open")
	}
	send(pressed(key.NameReturn))
	if s.selecting || v.settingText(settings.CompareCollection+".path") == "" {
		t.Fatal("compare program was not chosen")
	}
	send(modified(key.NameReturn, key.ModCtrl))
	waitSettingsSave(t, v, s)
	loaded, err := settings.Load(root, bindingSettingsCatalogue())
	if err != nil || len(loaded.ProgramRules(settings.CompareCollection)) != 3 {
		t.Fatal("compare rule did not save", err)
	}
	rows := loaded.Collection(settings.CompareCollection)
	id := rows[2]["id"].(string)
	field := bindingSettingKey("settings."+settings.CompareCollection+".rules."+id, 0)
	if err := loaded.Set(bindingSettingsCatalogue(), field, settings.Value{Text: "Ctrl+Alt+Y"}); err != nil {
		t.Fatal("compare rule cannot have a configurable binding", err)
	}
	if err := settings.Save(root, loaded); err != nil {
		t.Fatal(err)
	}
	loaded, err = settings.Load(root, bindingSettingsCatalogue())
	if err != nil || !loaded.Overridden(field) {
		t.Fatal("compare rule binding did not round-trip", err)
	}
	loaded.SetCollection(settings.CompareCollection, rows[:2])
	pruneRuleBindings(&loaded)
	if loaded.Overridden(field) {
		t.Fatal("deleted compare rule retained its binding")
	}
}

func TestComparePreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_COMPARE")
	if dir == "" {
		t.Skip("optional compare preview")
	}
	v := newPreviewView()
	v.loader = &directoryLoader{}
	v.active = 1
	selectName(t, &v.panels[1], "commando-reference.png")
	v.chooseFileTarget("Compare", func(int, int) {})
	if err := renderViewPNG(filepath.Join(dir, "compare-panels.png"), v); err != nil {
		t.Fatal(err)
	}
	v.navigateInlineActions(-1, false)
	if err := renderViewPNG(filepath.Join(dir, "compare-same-panel.png"), v); err != nil {
		t.Fatal(err)
	}
	v.cancelInlineActions()
	v.startCollectionWidget(settings.CompareCollection, -1, "New")
	if err := renderViewPNG(filepath.Join(dir, "compare-settings.png"), v); err != nil {
		t.Fatal(err)
	}
}
