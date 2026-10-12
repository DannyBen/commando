package app

import (
	"errors"
	"image"
	"maps"
	"slices"
	"testing"
	"time"

	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"github.com/dannyben/commando/internal/browser"
)

func TestMetadataColumnCyclePreservesBrowsing(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(pressed(key.NameEnd), key.EditEvent{Text: "c"})
	p := &v.panels[0]
	err := errors.New("retained read error")
	p.err = err
	items, marks := slices.Clone(p.items), maps.Clone(p.marks)
	selected, position, prefix := p.selected, p.list.Position, p.prefix
	for _, tc := range []struct {
		modifiers key.Modifiers
		want      metadataColumn
	}{
		{key.ModCtrl, metadataModified}, {key.ModCtrl, metadataNone},
		{key.ModCtrl, metadataExtension}, {key.ModCtrl, metadataSize},
		{key.ModCtrl | key.ModShift, metadataExtension}, {key.ModCtrl | key.ModShift, metadataNone},
		{key.ModCtrl | key.ModShift, metadataModified}, {key.ModCtrl | key.ModShift, metadataSize},
	} {
		send(modified("M", tc.modifiers))
		if p.metadata != tc.want {
			t.Fatalf("mode=%v want %v", p.metadata, tc.want)
		}
		if p.selected != selected || p.list.Position != position || p.prefix != prefix || p.err != err ||
			!maps.Equal(p.marks, marks) || !slices.Equal(p.items, items) {
			t.Fatal("changing the column changed browsing state or listing order")
		}
		send(key.Event{Name: "M", Modifiers: tc.modifiers, State: key.Release})
		if p.metadata != tc.want {
			t.Fatal("key release cycled column")
		}
	}
	send(modified("M", key.ModCtrl), pressed(key.NameTab))
	if v.panels[1].metadata != metadataSize {
		t.Fatal("column choice leaked to another panel")
	}
	send(modified("M", key.ModCtrl|key.ModShift), modified(key.NameTab, key.ModShift))
	if p.metadata != metadataModified || v.panels[1].metadata != metadataExtension {
		t.Fatal("switching panels lost column choices")
	}
	v.acceptLoad(loadResult{request: loadRequest{panel: 0}, directory: browser.Directory{Path: "/new"}})
	if p.metadata != metadataModified {
		t.Fatal("opening a directory reset the column")
	}
}

func TestMetadataKeysKeepLocationEditor(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(modified("D", key.ModAlt))
	before := v.location.Text()
	send(modified("M", key.ModCtrl), modified("M", key.ModCtrl|key.ModShift))
	if !v.locationEditing() || v.location.Text() != before || v.panels[0].metadata != metadataSize {
		t.Fatal("column shortcuts changed the location editor or browsing mode")
	}
}

func TestMetadataValues(t *testing.T) {
	modifiedAt := time.Date(2026, time.October, 11, 12, 30, 0, 0, time.Local)
	for _, tc := range []struct {
		name string
		mode metadataColumn
		item entry
		want string
	}{
		{"none", metadataNone, entry{name: "notes.txt", size: "128 B"}, ""},
		{"extension", metadataExtension, entry{name: "notes.TXT"}, "TXT"},
		{"last extension", metadataExtension, entry{name: "source.tar.gz"}, "gz"},
		{"no extension", metadataExtension, entry{name: "README"}, ""},
		{"dotfile", metadataExtension, entry{name: ".gitignore"}, ""},
		{"dotfile extension", metadataExtension, entry{name: ".config.json"}, "json"},
		{"directory", metadataExtension, entry{name: "folder.ext", file: browser.Entry{Directory: true}}, ""},
		{"size", metadataSize, entry{name: "notes.txt", size: "128 B"}, "128 B"},
		{"unknown size", metadataSize, entry{name: "notes.txt", size: "?"}, "?"},
		{"modified", metadataModified, entry{file: browser.Entry{Modified: modifiedAt}}, "2026-10-11"},
		{"unknown date", metadataModified, entry{name: "notes.txt"}, "?"},
		{"parent", metadataModified, entry{name: "..", file: browser.Entry{Parent: true}}, ""},
		{"drive", metadataSize, entry{name: `C:\`, size: "—", file: browser.Entry{Path: `C:\`}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.mode.value(tc.item); got != tc.want {
				t.Fatalf("value=%q want %q", got, tc.want)
			}
		})
	}
}

func TestTrailingMetadataUsesEachRowsContentWidth(t *testing.T) {
	for _, proportional := range []bool{false, true} {
		for _, size := range []unit.Sp{15, 28} {
			v := newPreviewView()
			if proportional {
				v.theme.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
			}
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(400, 60))}
			previous := gtx.Constraints.Max.X + 1
			for _, value := range []string{"", "?", "png", "56.7 KB", "2026-10-11"} {
				var trailing layout.Widget
				if value != "" {
					trailing = func(gtx layout.Context) layout.Dimensions {
						dims, info := (widget.Label{MaxLines: 1}).LayoutDetailed(gtx, v.theme.Shaper, font.Font{Typeface: v.theme.Face}, size, value, op.CallOp{})
						if info.Truncated != 0 {
							t.Fatalf("metadata %q truncated at font size %v", value, size)
						}
						return dims
					}
				}
				available := 0
				v.trailingLayout(gtx, func(gtx layout.Context) layout.Dimensions {
					available = gtx.Constraints.Max.X
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}, trailing)
				if value == "" && available != 400 {
					t.Fatal("empty metadata reserved filename space")
				}
				if available >= previous {
					t.Fatalf("%q did not leave less filename space than shorter metadata: %d >= %d", value, available, previous)
				}
				previous = available
			}
		}
	}
}
