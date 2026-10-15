package app

import (
	"image"
	"maps"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/settings"
)

func TestSettingsPreviewsAreSelectiveAndSpecific(t *testing.T) {
	v := newPreviewView()
	for _, role := range []string{"background", "alt_background", "text", "border", "selection"} {
		spec, _ := settings.CatalogueDefault.Widget(colorPrefix + role)
		if spec.Preview != "" {
			t.Fatalf("%s has an unnecessary preview", role)
		}
	}
	for _, role := range []string{"info", "success", "warning", "error"} {
		spec, _ := settings.CatalogueDefault.Widget(colorPrefix + role)
		samples := v.settingsPreviewItems(spec.Preview)
		if len(samples) != 1 {
			t.Fatalf("%s should show only its own sample", role)
		}
	}
	root, _ := settings.CatalogueDefault.Widget(colorPrefix + "notification")
	if len(v.settingsPreviewItems(root.Preview)) != 4 {
		t.Fatal("notification root must show all four colors")
	}
	for _, spec := range settings.CatalogueDefault.UI.Widgets {
		if spec.Preview != "" && len(v.settingsPreviewItems(spec.Preview)) == 0 {
			t.Fatalf("unknown preview %q", spec.Preview)
		}
	}
	selected := v.settingsPreviewItems("selected_text")
	if len(selected) != 1 || selected[0].foreground != v.appearance.Colors.SelectedName || selected[0].background != v.appearance.Colors.Selected {
		t.Fatal("selected text preview must show its actual background and foreground")
	}
	edge := v.settingsPreviewItems("edge")[0]
	if edge.edge != v.appearance.Colors.Edge {
		t.Fatal("edge sample does not show the actual edge mark")
	}
}

func TestSettingsPreviewAddsOnlyOneRow(t *testing.T) {
	for _, role := range []string{"edge", "notification", "error"} {
		v := newPreviewView()
		v.startSettingsWidget(colorPrefix + role)
		s := v.settingsInteraction()
		var ops op.Ops
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 480))}
		with := v.layoutSettingsWidget(gtx)
		s.preview = ""
		without := v.layoutSettingsWidget(gtx)
		rowHeight := v.textHeight(gtx, v.appearance.TextSize) + gtx.Dp(2*v.appearance.RowPaddingVertical)
		if with.Size.X != without.Size.X || with.Size.Y-without.Size.Y != rowHeight {
			t.Fatal("preview changed more than one settings row")
		}
	}
}

func TestPreviewSampleUsesNormalTextBaseline(t *testing.T) {
	for _, scale := range []float32{1, 1.5, 2} {
		for _, padding := range []unit.Dp{0, 2, 6} {
			v := newPreviewView()
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}}
			height := v.textHeight(gtx, v.appearance.TextSize) + gtx.Dp(2*padding)
			gtx.Constraints = layout.Exact(image.Pt(300, height))
			normal := layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				return v.label(gtx, "Mark", v.appearance.TextSize, v.appearance.Colors.Foreground, text.Start)
			})
			sample := v.settingsPreviewItems("edge").layout(v, gtx, height)
			if sample.Baseline != normal.Baseline {
				t.Fatalf("scale %g padding %g: sample baseline %d, normal %d", scale, padding, sample.Baseline, normal.Baseline)
			}
		}
	}
}

func TestSettingsPreviewRetainsEditingAndUpdatesLive(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	v.startSettingsWidget(colorPrefix + "error")
	s := v.settingsInteraction()
	current, marks := v.panels[0].selected, maps.Clone(v.panels[0].marks)
	send, advance := panelInputFrames(v)
	send(pressed(key.NameDownArrow)) // Link toggle -> Lightness.
	send(pressed(key.NameReturn))
	send(key.EditEvent{Range: key.Range{Start: 0, End: 4}, Text: "0.6"})
	start, end := s.editor.Selection()
	advance(time.Second)
	a, b := s.editor.Selection()
	if s.editor.Text() != "0.6" || start != a || end != b {
		t.Fatal("preview disturbed the value editor")
	}
	before := v.settingsPreviewItems(s.preview)[0].foreground
	send(pressed(key.NameReturn))
	after := v.settingsPreviewItems(s.preview)[0].foreground
	if s.editing || before == after || after != v.appearance.Colors.Error {
		t.Fatal("sample did not update or preview stole focus")
	}
	if v.panels[0].selected != current || !maps.Equal(v.panels[0].marks, marks) {
		t.Fatal("preview changed pane state")
	}
	v.finishSettingsWidget(layout.Context{}, false)
	if v.settingsPreviewItems("error")[0].foreground != before {
		t.Fatal("Cancel did not restore the sample color")
	}
}

func TestSettingsPreviewRowsRender(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional compact preview rows")
	}
	for _, role := range []string{"background", "edge", "notification", "error", "text_selection", "selected_text"} {
		v := newPreviewView()
		v.applySettings()
		v.startSettingsWidget(colorPrefix + role)
		if err := renderViewPNG(filepath.Join(dir, "theme-preview-row-"+role+".png"), v); err != nil {
			t.Fatal(err)
		}
	}
}
