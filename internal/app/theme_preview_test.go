package app

import (
	"fmt"
	"image"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/feedback"
)

func TestThemePreviewUsesOnlyFirstPanel(t *testing.T) {
	for count := 1; count <= 3; count++ {
		v := newView(samplePanels()[:count])
		v.startSettingsWidget(settingThemeGenerator)
		for i := range v.panels {
			preview := v.themePreview(i)
			switch {
			case i == 0:
				if !preview.selection || !preview.marks || !reflect.DeepEqual(preview.messages, []feedback.Kind{feedback.Info, feedback.Success, feedback.Warning, feedback.Error}) {
					t.Fatal("first panel missing priority samples")
				}
			default:
				if preview.selection || preview.marks || len(preview.messages) != 0 {
					t.Fatal("additional panel has preview overrides")
				}
			}
		}
		v.finishSettingsWidget(layout.Context{}, false)
		for i := range v.panels {
			preview := v.themePreview(i)
			if preview.selection || preview.marks || len(preview.messages) != 0 {
				t.Fatal("preview survived dismissal")
			}
		}
	}
}

func TestThemePreviewMarksVisibleEligibleRowsWithoutChangingMarks(t *testing.T) {
	v := newView(samplePanels())
	p := &v.panels[0]
	p.list.Position.First = 5
	p.toggleMark(10)
	before := maps.Clone(p.marks)
	v.startSettingsWidget(settingThemeGenerator)
	preview := v.themePreview(0)
	for row := range p.items {
		if got, want := preview.marked(p, row), row >= 5 && row <= 7; got != want {
			t.Fatalf("row %d marked=%v want %v", row, got, want)
		}
	}
	p.list.Position.First = 0
	if preview.marked(p, 0) {
		t.Fatal("parent row received a sample mark")
	}
	if !maps.Equal(before, p.marks) {
		t.Fatal("sample marks changed real marks")
	}
	p.items = nil
	if preview.marked(p, 0) {
		t.Fatal("empty listing received a sample mark")
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

func TestThemePreviewCurrentRowFallbackAndViewport(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		count, first, visible, want int
	}{
		{"empty", 0, 0, 0, -1},
		{"parent only", 1, 0, 1, 0},
		{"two rows", 2, 0, 2, 1},
		{"three rows", 3, 0, 3, 2},
		{"long listing", 10, 0, 10, 2},
		{"scrolled listing", 10, 4, 6, 6},
		{"two visible rows", 10, 4, 2, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newView(samplePanels())
			p := &v.panels[0]
			p.items = p.items[:tc.count]
			p.list.Position.First, p.list.Position.Count = tc.first, tc.visible
			p.selected = 0
			v.active = 1
			v.startSettingsWidget(settingThemeGenerator)
			if row := v.themePreview(0).currentRow(p); row != tc.want {
				t.Fatalf("preview current row %d, want %d", row, tc.want)
			}
			if p.selected != 0 || v.active != 1 {
				t.Fatal("preview changed real selection or panel focus")
			}
			v.finishSettingsWidget(layout.Context{}, false)
			if v.themePreview(0).currentRow(p) != p.selected {
				t.Fatal("preview dismissal did not restore real current-row presentation")
			}
		})
	}
}
