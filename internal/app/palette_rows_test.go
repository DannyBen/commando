package app

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestPaletteWrapPreservesLabelsAndPrefersBreadcrumbs(t *testing.T) {
	for _, tc := range []struct {
		value string
		width int
		want  []string
	}{
		{"Actions › Rename…", 30, []string{"Actions › Rename…"}},
		{"Settings › Programs › Actions › New from context menu", 26, []string{"Settings › Programs ›", "Actions ›", "New from context menu"}},
		{"Actions › Very long command name", 18, []string{"Actions ›", "Very long command", "name"}},
		{"abcdefghij", 4, []string{"abcd", "efgh", "ij"}},
		{"e\u0301e\u0301e\u0301", 3, []string{"e\u0301", "e\u0301", "e\u0301"}},
		{"🇮🇱🇮🇱", 3, []string{"🇮🇱", "🇮🇱"}},
		{"תיקייה › פעולה ארוכה", 12, []string{"תיקייה ›", "פעולה ארוכה"}},
	} {
		t.Run(tc.value, func(t *testing.T) {
			lines := wrapPaletteText(tc.value, tc.width, utf8.RuneCountInString)
			var got []string
			for _, line := range lines {
				got = append(got, line.text)
				if utf8.RuneCountInString(line.text) > tc.width ||
					string([]rune(tc.value)[line.offset:line.offset+utf8.RuneCountInString(line.text)]) != line.text {
					t.Fatal("line exceeded width or lost its highlight offsets", line)
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("wrapped lines = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestPaletteOverflowKeepsCompleteCommandsAndBindings(t *testing.T) {
	v := newPreviewView()
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	textHeight, smallHeight := v.textHeight(gtx, v.appearance.TextSize), v.textHeight(gtx, v.appearance.SmallTextSize)
	gap := v.textWidth(gtx, "   ", v.appearance.TextSize)
	short := cmd("short", "Actions", "Open", key.NameF4, 0, nil)
	if row := v.measurePaletteRow(gtx, short, 320, gap, textHeight, smallHeight, 2); len(row.label) != 1 || row.height != max(textHeight, smallHeight)+4 {
		t.Fatal("fitting command no longer uses the compact row", row)
	}
	long := cmd("long", "Settings › Programs › Actions", "New from context menu…", "F12", key.ModCtrl|key.ModAlt|key.ModShift, nil)
	long.bindings[1].Default = shortcut{Name: "F11", Modifiers: key.ModCtrl | key.ModAlt | key.ModShift}
	for _, width := range []int{160, 300, 480} {
		row := v.measurePaletteRow(gtx, long, width, gap, textHeight, smallHeight, 2)
		if len(row.label) < 2 || row.height != row.headerHeight+(len(row.label)-1)*textHeight+4 {
			t.Fatal("overflow did not measure its header and continuation lines", width, row)
		}
		if row.firstWidth+gap+v.textWidth(gtx, row.shortcut[0].text, v.appearance.SmallTextSize) > width {
			t.Fatal("first line did not leave room for the binding", width, row)
		}
		for _, part := range []struct {
			lines []paletteLine
			value string
			size  unit.Sp
		}{{row.label, long.label(), v.appearance.TextSize}, {row.shortcut, long.shortcut(), v.appearance.SmallTextSize}} {
			var joined strings.Builder
			for i, line := range part.lines {
				lineWidth := width
				if i == 0 && part.size == v.appearance.TextSize {
					lineWidth = row.firstWidth
				}
				if v.textWidth(gtx, line.text, part.size) > lineWidth {
					t.Fatal("wrapped text still exceeds its row", width, line)
				}
				joined.WriteString(line.text)
			}
			withoutSpaces := func(s string) string {
				return strings.Map(func(r rune) rune {
					if unicode.IsSpace(r) {
						return -1
					}
					return r
				}, s)
			}
			if withoutSpaces(joined.String()) != withoutSpaces(part.value) {
				t.Fatal("wrapping hid or replaced command content", joined.String(), part.value)
			}
		}
	}
}

func TestPaletteBindingSharesFirstBreadcrumbLine(t *testing.T) {
	v := newPreviewView()
	gtx := layout.Context{Ops: &op.Ops{}, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	textHeight, smallHeight := v.textHeight(gtx, v.appearance.TextSize), v.textHeight(gtx, v.appearance.SmallTextSize)
	gap := v.textWidth(gtx, "   ", v.appearance.TextSize)
	c := cmd("metadata", "View", "Previous metadata column", "M", key.ModCtrl|key.ModShift, nil)
	row := v.measurePaletteRow(gtx, c, 240, gap, textHeight, smallHeight, 2)
	if len(row.label) != 2 || row.label[0].text != "View ›" || row.label[1].text != c.name || len(row.shortcut) != 1 {
		t.Fatal("binding did not share the breadcrumb line above the full-width command", row)
	}
	if row.headerHeight != max(textHeight, smallHeight) || row.height != row.headerHeight+textHeight+4 {
		t.Fatal("binding added an unnecessary third row", row)
	}
	if v.textWidth(gtx, row.label[1].text, v.appearance.TextSize) <= row.firstWidth {
		t.Fatal("fixture did not require the continuation to regain the full width")
	}
}

func TestPaletteWrappedRowsRevealAndPageByHeight(t *testing.T) {
	v := newPreviewView()
	send, router := paletteFrames(v, image.Pt(320, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	send(pressed("`"))
	s := v.paletteInteraction()
	assertVisible := func() {
		t.Helper()
		pos := s.list.Position
		top := s.rowOffsets[pos.First] + pos.Offset
		start, bottom := s.rowOffsets[s.selected], s.rowOffsets[s.selected+1]
		if start < top || bottom-start <= s.viewportHeight && bottom > top+s.viewportHeight || !router.Source().Focused(&v.commandSearch) {
			t.Fatal("wrapped selection is clipped or search lost focus", s.selected, pos)
		}
	}
	for range 10 {
		send(pressed(key.NamePageDown))
		assertVisible()
	}
	for range 10 {
		send(pressed(key.NamePageUp))
		assertVisible()
	}
	send(pressed(key.NameEnd))
	assertVisible()
	send(replacePalette(v, "Settings Programs Actions New from context menu"))
	assertVisible()
	if len(s.results) != 1 || s.rowOffsets[1] <= v.textHeight(layout.Context{Ops: &op.Ops{}}, v.appearance.TextSize) {
		t.Fatal("narrow settings search did not wrap")
	}
	send(pressed(key.NameEscape))
	if !router.Source().Focused(&v.focus) {
		t.Fatal("closing wrapped results did not restore browsing focus")
	}
}

func TestPaletteOversizedRowCanBeReadWithPageKeys(t *testing.T) {
	v := newPreviewView()
	send, router := paletteFrames(v, image.Pt(320, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	send(pressed("`"))
	s := v.paletteInteraction()
	s.commands = []command{{id: "long", group: "Actions", name: strings.Repeat("Very long command ", 50)}}
	s.filter("")
	send()
	if s.rowOffsets[1] <= s.viewportHeight {
		t.Fatal("fixture did not produce an oversized row")
	}
	send(pressed(key.NamePageDown))
	if s.selected != 0 || s.list.Position.Offset <= 0 || !router.Source().Focused(&v.commandSearch) {
		t.Fatal("Page Down did not reveal the rest of the selected row")
	}
	send(pressed(key.NamePageUp))
	if s.list.Position.Offset != 0 {
		t.Fatal("Page Up did not return to the beginning of the row")
	}
	for range s.rowOffsets[1]/s.viewportHeight + 2 {
		send(pressed(key.NamePageDown))
	}
	if s.list.Position.Offset+s.viewportHeight < s.rowOffsets[1] {
		t.Fatal("Page Down could not reach the end of a long command")
	}
}

func TestPaletteWrappingPreviews(t *testing.T) {
	directory := os.Getenv("COMMANDO_PREVIEW_ACTIONS")
	if directory == "" {
		t.Skip("optional headless wrapping previews")
	}
	for _, theme := range builtInThemes {
		for _, width := range []int{320, 480, 800} {
			for _, fixture := range []struct{ name, query string }{
				{"settings", "Settings Programs Actions New from context menu"},
				{"bindings", "Window increase"},
				{"metadata", "View metadata"},
				{"mixed", ""},
				{"actions", ";"},
			} {
				v := newPreviewView()
				v.panels = v.panels[:1]
				v.settingsValues = theme.values.Clone()
				v.applySettings()
				v.startPalette(layout.Context{Ops: &op.Ops{}})
				v.commandSearch.SetText(fixture.query)
				v.paletteInteraction().filter(fixture.query)
				name := fmt.Sprintf("palette-wrap-%s-%d-%s.png", theme.name, width, fixture.name)
				if err := renderSizedViewPNG(filepath.Join(directory, name), v, image.Pt(width, 480)); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
