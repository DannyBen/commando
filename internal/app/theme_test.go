package app

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
)

func TestThemeRowSizingAndNavigation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		size    unit.Sp
		padding unit.Dp
		metric  unit.Metric
	}{
		{"compact", 13, 0, unit.Metric{PxPerDp: 1, PxPerSp: 1}},
		{"user theme", 16, 6, unit.Metric{PxPerDp: 1, PxPerSp: 1}},
		{"large text", 32, 3, unit.Metric{PxPerDp: 1, PxPerSp: 1}},
		{"extra padding", 20, 16, unit.Metric{PxPerDp: 1, PxPerSp: 1}},
		{"scaled display", 24, 4, unit.Metric{PxPerDp: 1.5, PxPerSp: 1.5}},
		{"text scaling", 24, 6, unit.Metric{PxPerDp: 1, PxPerSp: 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newPreviewView()
			v.appearance.TextSize = tc.size
			v.appearance.RowPaddingVertical = tc.padding
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Metric: tc.metric,
				Constraints: layout.Constraints{Max: image.Pt(1000, 1000)}}
			// Compare row layout with uncropped text, including ascenders/descenders.
			item := entry{name: "Ágjpq.txt", size: "128 KB"}
			line := v.label(gtx, item.name, tc.size, v.appearance.Colors.Filename, text.Start)
			padding := gtx.Dp(tc.padding)
			height := line.Size.Y + 2*padding
			gtx.Constraints = layout.Exact(image.Pt(600, height))
			row := v.rowContents(gtx, item, v.appearance.Colors.Filename, v.appearance.Colors.Metadata, metadataSize, fileTextRoles{contrastPrimary, contrastSecondary}, false, nil)
			if row.Size.Y != height || row.Baseline != line.Baseline+padding {
				t.Fatalf("text not centered in row: row=%+v, natural text=%+v, padding=%d", row, line, padding)
			}

			var router input.Router
			frame := func() {
				ops.Reset()
				v.layout(layout.Context{Ops: &ops, Metric: tc.metric, Now: time.Now(),
					Source: router.Source(), Constraints: layout.Exact(image.Pt(1200, 800))})
				router.Frame(&ops)
			}
			frame()
			frame()
			p := &v.panels[0]
			if got := p.list.Position.Length; got != len(p.items)*height {
				t.Fatalf("list rows cropped or sized incorrectly: length=%d, want %d", got, len(p.items)*height)
			}
			for _, name := range []key.Name{key.NameEnd, key.NameHome} {
				router.Queue(key.Event{Name: name, State: key.Press})
				frame()
				frame()
				pos := p.list.Position
				if p.selected < pos.First || p.selected >= pos.First+pos.Count ||
					(p.selected == pos.First && pos.Offset > 0) ||
					(p.selected == pos.First+pos.Count-1 && pos.OffsetLast < 0) {
					t.Fatalf("%s did not reveal the whole selected row: selected=%d, position=%+v", name, p.selected, pos)
				}
			}
		})
	}
}
