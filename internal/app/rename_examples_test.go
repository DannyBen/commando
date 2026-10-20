package app

import (
	"image"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/renameexpr"
)

func TestRenameExamplesUseSupportedExpressions(t *testing.T) {
	seen := map[string]bool{}
	for _, example := range renameExamples {
		t.Run(example.expression, func(t *testing.T) {
			if example.meaning == "" || seen[example.expression] {
				t.Fatal("missing meaning or duplicate example")
			}
			seen[example.expression] = true
			expr, err := renameexpr.Parse(example.expression)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := expr.Evaluate([]renameexpr.Entry{{Name: "Old Text - Copy.jpg"}}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRenameExampleBrowsingIsPassiveAndEnterInserts(t *testing.T) {
	v := multiRenameView()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 1)}
	send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	send(pressed(key.NameF2))
	s := v.multiRenameInteraction()
	before := s.editor.Text()
	start, end := s.editor.Selection()
	preview := s.preview
	send(pressed(key.NameTab))
	send(pressed(key.NameEnd))
	if s.focus != renameFocusInsert || s.example != len(renameExamples)-1 {
		t.Fatal("Tab/End did not browse examples")
	}
	if s.editor.Text() != before || !reflect.DeepEqual(s.preview, preview) || len(v.loader.requests) != 0 {
		t.Fatal("browsing changed the expression or applied it")
	}
	if gotStart, gotEnd := s.editor.Selection(); gotStart != start || gotEnd != end {
		t.Fatal("browsing changed the expression selection")
	}
	send(key.Event{Name: key.NameReturn, State: key.Release})
	if s.editor.Text() != before {
		t.Fatal("Enter release inserted an example")
	}
	send(pressed(key.NameEnter))
	if s.editor.Text() != renameExamples[len(renameExamples)-1].expression || s.focus != renameFocusExpression || !router.Source().Focused(&s.editor) {
		t.Fatal("Enter did not replace the selection and restore editing focus")
	}
	if len(v.loader.requests) != 0 || s.pending {
		t.Fatal("insertion executed a rename")
	}
	send(modified("Z", key.ModCtrl))
	if s.editor.Text() != before || len(s.preview) != 0 {
		t.Fatal("insertion did not support native Undo and preview restoration")
	}
}

func TestRenameExamplesPreviewPNG(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_RENAME")
	if dir == "" {
		t.Skip("optional inserter preview")
	}
	v := newPreviewView()
	v.panels = v.panels[:1]
	var ops op.Ops
	v.startMultiRename(layout.Context{Ops: &ops})
	s := v.multiRenameInteraction()
	s.editor.SetText(`<counter pad:0 min:1>. <name>`)
	v.previewMultiRename(s)
	s.focus = renameFocusInsert
	for i, example := range renameExamples {
		if example.expression == `<name from:1 to:10>` {
			s.example = i
			break
		}
	}
	if err := renderViewPNG(filepath.Join(dir, "multi-rename-inserter.png"), v); err != nil {
		t.Fatal(err)
	}
}

func TestRenameExampleInsertsAtUnicodeCaretOrSelection(t *testing.T) {
	for _, example := range []struct {
		start, end int
		want       string
	}{
		{5, 5, "שלום-<counter><name>"},
		{0, 4, "<counter>-<name>"},
		{4, 0, "<counter>-<name>"},
	} {
		v := multiRenameView()
		send, _ := panelInputFrames(v)
		send(pressed(key.NameF2))
		s := v.multiRenameInteraction()
		send(replaceExpression(v, "שלום-<name>"))
		s.editor.SetCaret(example.start, example.end)
		send(pressed(key.NameTab))
		send(pressed(key.NameRightArrow), pressed(key.NameRightArrow), pressed(key.NameRightArrow))
		send(pressed(key.NameReturn))
		if s.editor.Text() != example.want {
			t.Fatalf("selection %d/%d: got %q, want %q", example.start, example.end, s.editor.Text(), example.want)
		}
		send(modified("Z", key.ModCtrl))
		if s.editor.Text() != "שלום-<name>" {
			t.Fatal("Undo did not restore the Unicode expression")
		}
	}
}

func TestRenameExampleClickRetainsExpressionSelection(t *testing.T) {
	v := multiRenameView()
	send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	send(pressed(key.NameF2))
	s := v.multiRenameInteraction()
	if !router.Source().Focused(&s.editor) {
		t.Fatal("expression was not focused")
	}
	s.exampleTarget.Click()
	send()
	if s.focus != renameFocusInsert || !router.Source().Focused(&v.focus) || s.editor.SelectedText() != "<name>" {
		t.Fatal("click did not move focus while retaining the expression selection")
	}
	send(pressed(key.NameRightArrow))
	send(pressed(key.NameReturn))
	if s.editor.Text() != "<base>" || !router.Source().Focused(&s.editor) || s.preview["a.txt"] != "a" {
		t.Fatal("browsed example did not replace the retained selection and update preview")
	}
}

func TestRenameExampleEndpointsAndReleasesStayQuiet(t *testing.T) {
	v := multiRenameView()
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	send(pressed(key.NameTab))
	s := v.multiRenameInteraction()
	recorded := &recordedSounds{}
	v.sounds = recorded
	send(pressed(key.NameHome), key.Event{Name: key.NameEnd, State: key.Release}, modified(key.NameEnd, key.ModShift))
	if s.example != 0 || len(recorded.played) != 0 {
		t.Fatal("endpoint no-op, release, or modified key changed the example")
	}
	send(pressed(key.NameEnd), pressed(key.NameEnd))
	if s.example != len(renameExamples)-1 || len(recorded.played) != 1 {
		t.Fatal("End did not select the last example exactly once")
	}
	send(pressed(key.NameRightArrow))
	if s.example != 0 || len(recorded.played) != 2 {
		t.Fatal("Right did not cycle back to the first example")
	}
	send(pressed(key.NameTab), pressed(key.NameRightArrow))
	if s.focus != renameFocusCancel || s.example != 0 {
		t.Fatal("action navigation changed the example")
	}
	send(modified(key.NameTab, key.ModShift), modified(key.NameTab, key.ModShift))
	if s.focus != renameFocusInsert {
		t.Fatal("reverse Tab did not restore the inserter")
	}
}

// Route wheel events through the actual shared value target without tying the
// test to dock position, font metrics, or the global help footer's height.
func renameExamplePointerFrames(v *view) func(...event.Event) {
	s := v.multiRenameInteraction()
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(350, 30))}
		v.updateMultiRename(gtx, s)
		event.Op(gtx.Ops, &v.focus)
		if s.pending {
			gtx = gtx.Disabled()
		}
		v.layoutRenameExamples(gtx, s, 30)
		router.Frame(&ops)
	}
	frame()
	frame()
	return func(events ...event.Event) { router.Queue(events...); frame(); frame() }
}

func TestRenameExampleMouseBrowsesWithoutInserting(t *testing.T) {
	v := multiRenameView()
	var ops op.Ops
	v.startMultiRename(layout.Context{Ops: &ops})
	s := v.multiRenameInteraction()
	send := renameExamplePointerFrames(v)
	before := s.editor.Text()
	send(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(100, 15), Scroll: f32.Pt(0, 120)})
	if s.example != 1 || s.focus != renameFocusInsert || s.editor.Text() != before {
		t.Fatal("wheel did not browse without inserting")
	}
	send(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(100, 15), Scroll: f32.Pt(0, 120)})
	if s.example != 2 || s.editor.Text() != before {
		t.Fatal("wheel inserted or did not step")
	}
	send(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(100, 15), Scroll: f32.Pt(0, -120)})
	if s.example != 1 {
		t.Fatal("reverse wheel did not step")
	}
	s.exampleTarget.Click()
	send()
	if s.focus != renameFocusInsert || s.editor.Text() != before {
		t.Fatal("example click inserted text")
	}
	s.pending = true
	send()
	send(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(100, 15), Scroll: f32.Pt(0, 120)})
	if s.example != 1 || s.editor.Text() != before {
		t.Fatal("pending rename allowed example changes")
	}
}
