package app

import (
	"fmt"
	"image"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
)

func TestSettingsPanelPreviewProfilesAndIsolation(t *testing.T) {
	for _, group := range []string{settingThemeGenerator, settingThemeSelect, settingThemeCustomize, "appearance.contrast_guard", "appearance.autocolor", "appearance.autocolor.selection", "appearance.autocolor.special_files"} {
		for count := 1; count <= 3; count++ {
			v := newView(samplePanels()[:count])
			v.startSettingsWidget(group)
			preview := v.settingsPanelPreview(0)
			if len(preview.rows) == 0 {
				t.Fatal("missing synthetic preview", group)
			}
			theme := group == settingThemeGenerator || group == settingThemeSelect || group == settingThemeCustomize || group == "appearance.contrast_guard"
			if theme {
				if !preview.textSelection || !reflect.DeepEqual(preview.messages, []feedback.Kind{feedback.Info, feedback.Success, feedback.Warning, feedback.Error}) {
					t.Fatal("theme preview lost full samples", group)
				}
			} else if preview.textSelection || len(preview.messages) != 0 {
				t.Fatal("Auto Colors preview contains unrelated samples", group)
			}
			for i := 1; i < count; i++ {
				if other := v.settingsPanelPreview(i); len(other.rows) != 0 || other.textSelection || len(other.messages) != 0 {
					t.Fatal("preview leaked to another panel", group)
				}
			}
			v.finishSettingsWidget(layout.Context{}, false)
			if preview := v.settingsPanelPreview(0); len(preview.rows) != 0 || preview.textSelection || len(preview.messages) != 0 {
				t.Fatal("preview survived dismissal", group)
			}
		}
	}
	for _, group := range []string{"appearance.font", "appearance.padding.vertical", "appearance.padding.horizontal"} {
		v := newPreviewView()
		v.startSettingsWidget(group)
		if len(v.settingsPanelPreview(0).rows) != 0 {
			t.Fatal("unnecessary Font/Padding preview", group)
		}
	}
}

func TestThemePreviewSamplesCoverDirectoryStates(t *testing.T) {
	var folder, normal, selected, cut, marked, hidden, dot bool
	for _, row := range themePreviewRows() {
		folder = folder || row.file.Directory
		normal = normal || (!row.selected && !row.cut)
		selected = selected || row.selected
		cut = cut || row.cut
		marked = marked || row.marked
		hidden = hidden || row.file.Hidden
		dot = dot || strings.HasPrefix(row.file.Name, ".")
	}
	if !folder || !normal || !selected || !cut || !marked || !hidden || !dot {
		t.Fatal("theme preview lacks representative directory states")
	}
}

func TestThemePreviewRenderingPreservesBrowsingAndEditorState(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	p := &v.panels[0]
	p.filter = "existing query"
	p.feedback.Show(feedback.Warning, "Real warning")
	message := p.feedback.Current()
	selected, marks := p.selected, maps.Clone(p.marks)
	items := append([]entry(nil), p.items...)
	v.startSettingsWidget(settingThemeGenerator)
	send, _ := panelInputFrames(v)
	send(pressed(key.NameDownArrow), pressed(key.NameReturn))
	send(key.EditEvent{Range: key.Range{End: 1}, Text: "123"})
	s := v.settingsInteraction()
	start, end := s.editor.Selection()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 480))}
	v.layout(gtx)
	a, b := s.editor.Selection()
	if v.settingsInteraction() != s || !s.editing || s.editor.Text() != "123" || a != start || b != end {
		t.Fatal("preview stole or altered generator editing")
	}
	if p.filter != "existing query" || p.selected != selected || !maps.Equal(p.marks, marks) || !reflect.DeepEqual(p.items, items) || p.feedback.Current() != message {
		t.Fatal("preview mutated real browsing state")
	}
	v.finishSettingsWidget(layout.Context{}, false)
	if p.filter != "existing query" || p.selected != selected || !maps.Equal(p.marks, marks) || p.feedback.Current() != message {
		t.Fatal("dismissal did not reveal original state")
	}
}

func TestThemePanelPreviewsRender(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional panel previews")
	}
	for count := 1; count <= 3; count++ {
		v := newView(samplePanels()[:count])
		v.applySettings()
		v.startSettingsWidget(settingThemeGenerator)
		if err := renderViewPNG(filepath.Join(dir, fmt.Sprintf("theme-panels-%d.png", count)), v); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAutoColorPreviewComparesNormalAndSelectedExtensions(t *testing.T) {
	normal, selected := map[string]bool{}, map[string]bool{}
	for _, row := range autoColorPreviewRows() {
		if row.marked || row.cut {
			t.Fatal("main Auto Colors preview contains unrelated states")
		}
		if row.selected {
			selected[filepath.Ext(row.file.Name)] = true
		} else {
			normal[filepath.Ext(row.file.Name)] = true
		}
	}
	if len(normal) < 3 || !reflect.DeepEqual(normal, selected) {
		t.Fatal("missing comparable normal and selected extension samples")
	}
}

func TestAutoColorSelectionPreviewSeparatesSelectionAndCutOffsets(t *testing.T) {
	rows := selectionPreviewRows()
	if len(rows) != 8 {
		t.Fatal("expected four selection samples, two cut samples and two ordinary samples")
	}
	extensions := map[string]bool{}
	for i, row := range rows {
		if row.marked || row.file.Directory || row.file.Parent {
			t.Fatal("unexpected sample styling")
		}
		if i < 4 {
			if !row.selected || row.cut {
				t.Fatal("first four rows must show ordinary selection backgrounds")
			}
			extensions[filepath.Ext(row.file.Name)] = true
		} else if i < 6 {
			if row.selected || !row.cut {
				t.Fatal("next two rows must show cut filename colors without backgrounds")
			}
			if !extensions[filepath.Ext(row.file.Name)] {
				t.Fatal("cut sample lacks a matching selection extension")
			}
		} else {
			if row.selected || row.cut {
				t.Fatal("last two rows must show ordinary filename colors without backgrounds")
			}
			if filepath.Ext(row.file.Name) != filepath.Ext(rows[i-2].file.Name) {
				t.Fatal("ordinary sample lacks a matching cut extension")
			}
		}
	}
	if len(extensions) != 4 {
		t.Fatal("selected samples should compare four different extension colors")
	}
}

func TestSpecialFilePreviewSeparatesHiddenDotfilesAndCombinedModifiers(t *testing.T) {
	names := map[string]bool{}
	for _, row := range specialFilePreviewRows() {
		names[row.file.Name] = row.file.Hidden
	}
	for _, spec := range []struct {
		name   string
		hidden bool
	}{
		{"normal-file.txt", false}, {"hidden-file.txt", true}, {".dotfile.txt", false}, {".hidden-dotfile.txt", true}, {"normal-noext", false}, {".dotfile-noext", false},
	} {
		hidden, exists := names[spec.name]
		if !exists || hidden != spec.hidden {
			t.Fatal("sample name does not describe its attributes", spec.name)
		}
	}
	states := map[string]bool{}
	for _, row := range specialFilePreviewRows() {
		if row.marked || row.selected || row.cut {
			t.Fatal("special file sample has unrelated styling")
		}
		dot := strings.HasPrefix(row.file.Name, ".")
		ext := filenameColorKey(row.file)
		states[fmt.Sprintf("%s/%t/%t", ext, row.file.Hidden, dot)] = true
	}
	for _, state := range []string{"extension:.txt/false/false", "extension:.txt/true/false", "extension:.txt/false/true", "extension:.txt/true/true", "category:._extless/false/false", "category:._extless_dotfile/false/true"} {
		if !states[state] {
			t.Fatal("missing comparison sample", state)
		}
	}
}

func TestSettingsPanelPreviewPreservesListingViewportClipboardAndEditor(t *testing.T) {
	for _, group := range []string{settingThemeGenerator, settingThemeSelect, settingThemeCustomize, "appearance.autocolor", "appearance.autocolor.selection", "appearance.autocolor.special_files"} {
		for _, empty := range []bool{false, true} {
			v := newPreviewView()
			v.applySettings()
			v.active = len(v.panels) - 1
			origin := v.active
			p := &v.panels[0]
			if empty {
				p.items = nil
				p.drives = true
			}
			p.filter = "query"
			p.list.Position.First, p.list.Position.Offset = 5, 3
			p.feedback.Show(feedback.Warning, "Real warning")
			if !empty {
				p.toggleMark(10)
			}
			v.cutClipboard = browser.FileClipboard{Move: true, Sequence: 7, Paths: []string{"existing-cut"}}
			v.clipboardSequence = func() uint32 { return 7 }
			selected, marks, items, position, message := p.selected, maps.Clone(p.marks), append([]entry(nil), p.items...), p.list.Position, p.feedback.Current()
			clipboard := v.cutClipboard
			before := v.settingsValues.Clone()
			v.startSettingsWidget(group)
			session := v.settingsInteraction()
			for i, field := range session.fields {
				if field.Control != "toggle" && field.Control != "choice" {
					session.selected = i
					break
				}
			}
			send, _ := panelInputFrames(v)
			send(pressed(key.NameReturn))
			s := v.settingsInteraction()
			send(key.EditEvent{Range: key.Range{End: len(s.editor.Text())}, Text: "12"})
			a, b := s.editor.Selection()
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 480))}
			v.layout(gtx)
			for _, metric := range []unit.Metric{{PxPerDp: 1, PxPerSp: 1}, {PxPerDp: 2, PxPerSp: 2}} {
				ops.Reset()
				v.layout(layout.Context{Ops: &ops, Metric: metric, Constraints: layout.Exact(image.Pt(360, 260))})
			}
			nextA, nextB := s.editor.Selection()
			if !s.editing || s.editor.Text() != "12" || nextA != a || nextB != b {
				t.Fatal("preview disturbed editor")
			}
			if v.active != origin || p.selected != selected || !maps.Equal(marks, p.marks) || !reflect.DeepEqual(items, p.items) || p.list.Position != position || p.filter != "query" || p.feedback.Current() != message || !reflect.DeepEqual(v.cutClipboard, clipboard) {
				t.Fatal("preview changed browsing, viewport, feedback or clipboard state")
			}
			v.activateSettingsAction(layout.Context{}, 1)
			if !reflect.DeepEqual(v.settingsValues.Clone(), before) || p.list.Position != position || !v.interaction.idle() || v.active != origin {
				t.Fatal("Cancel did not restore settings and browsing")
			}
		}
	}
}

func TestSettingsPanelPreviewRenders(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional settings panel previews")
	}
	for _, theme := range builtInThemes {
		for _, group := range []string{settingThemeGenerator, settingThemeSelect, settingThemeCustomize, "appearance.autocolor", "appearance.autocolor.selection", "appearance.autocolor.special_files"} {
			for _, enabled := range []bool{false, true} {
				v := newPreviewView()
				v.settingsValues = theme.values.Clone()
				_ = v.settingsValues.Set(settings.CatalogueDefault, settingAutoColorEnabled, settings.Value{Bool: enabled})
				v.applySettings()
				v.startSettingsWidget(group)
				name := fmt.Sprintf("%s-preview-%s-%t.png", theme.name, group, enabled)
				if err := renderViewPNG(filepath.Join(dir, name), v); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
