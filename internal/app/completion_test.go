package app

import (
	"testing"
	"unicode/utf8"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
)

func TestInlineCompletionPrefixSelectionAndCycling(t *testing.T) {
	c := inlineCompletion{candidates: completionCandidates([]string{"Alpha Sans", "Alpha Mono", "alpha mono", "Élan Mono"})}
	var editor widget.Editor
	for _, tc := range []struct{ query, suffix string }{
		{"", ""}, {"aL", "pha Mono"}, {"missing", ""}, {"Alpha Mono", ""}, {"é", "lan Mono"},
	} {
		editor.SetText(tc.query)
		end := utf8.RuneCountInString(tc.query)
		editor.SetCaret(end, end)
		c.update(&editor)
		if got := c.suffix(); got != tc.suffix || editor.Text() != tc.query {
			t.Fatalf("%q: suffix=%q, text=%q", tc.query, got, editor.Text())
		}
	}
	editor.SetText("al")
	editor.SetCaret(2, 2)
	c.update(&editor)
	if !c.cycle(1) || c.suffix() != "pha Sans" || editor.Text() != "al" {
		t.Fatal("cycling changed the query or selected the wrong candidate")
	}
	if !c.accept(&editor) || editor.Text() != "Alpha Sans" || c.accept(&editor) {
		t.Fatal("acceptance did not insert the correctly spelled candidate")
	}
	editor.SetText("Al")
	editor.SetCaret(0, 2)
	c.update(&editor)
	if c.suffix() != "" || c.accept(&editor) {
		t.Fatal("completion overwrote a native selection")
	}
	editor.SetCaret(1, 1)
	c.update(&editor)
	if c.suffix() != "" {
		t.Fatal("completion appeared while editing inside the query")
	}
}

func TestFontCompletionKeyboardPreviewUndoAndCancel(t *testing.T) {
	v := newPreviewView()
	v.fontFamilies = []string{"Alpha Sans", "Alpha Mono"}
	v.startSettingsWidget("appearance.font")
	send, _ := panelInputFrames(v)
	send(pressed(key.NameReturn))
	s := v.settingsInteraction()
	send(key.EditEvent{Range: key.Range{Start: 0, End: len(s.editor.Text())}, Text: "al"}, key.SelectionEvent{Start: 2, End: 2})
	if s.completion.suffix() != "pha Mono" || v.appearance.FontFamily != defaultTheme.FontFamily {
		t.Fatalf("typing did not suggest without applying: text=%q suffix=%q family=%q selection=%d", s.editor.Text(), s.completion.suffix(), v.appearance.FontFamily, s.editor.SelectionLen())
	}
	send(pressed(key.NameDownArrow))
	if s.completion.suffix() != "pha Sans" || s.editor.Text() != "al" {
		t.Fatal("Down did not cycle the inline suggestion")
	}
	send(pressed(key.NameTab))
	if !s.editing || s.selected != 0 || s.editor.Text() != "Alpha Sans" {
		t.Fatal("Tab moved to another field instead of accepting")
	}
	send(modified("Z", key.ModCtrl))
	if s.editor.Text() != "al" {
		t.Fatal("accepting a suffix broke native Undo")
	}
	// Native Undo selects the restored prefix; moving to its end enables
	// another suggestion without changing that restored spelling.
	send(pressed(key.NameEnd))
	send(pressed(key.NameTab))
	send(pressed(key.NameReturn))
	if s.editing || v.appearance.FontFamily != "Alpha Mono" {
		t.Fatal("Enter did not preview the accepted name")
	}
	v.finishSettingsWidget(layout.Context{}, false)
	if v.appearance.FontFamily != defaultTheme.FontFamily {
		t.Fatal("Cancel did not restore the font after autocomplete")
	}

	v.startSettingsWidget("appearance.font")
	send(pressed(key.NameReturn))
	s = v.settingsInteraction()
	send(key.EditEvent{Range: key.Range{Start: 0, End: len(s.editor.Text())}, Text: "Al"}, key.SelectionEvent{Start: 2, End: 2})
	send(pressed(key.NameReturn))
	if v.appearance.FontFamily != "Al" {
		t.Fatal("Enter silently accepted an unconfirmed suggestion")
	}
	send(pressed(key.NameReturn))
	send(pressed(key.NameEscape))
	if s.editing || v.appearance.FontFamily != "Al" {
		t.Fatal("Escape did not discard only the unfinished edit")
	}
}

func TestFontFamilyStepperAndArrivingCandidatesPreserveEditor(t *testing.T) {
	v := newPreviewView()
	v.fontFamilies = []string{"Alpha Sans", "Alpha Mono"}
	v.startSettingsWidget("appearance.font")
	send, _ := panelInputFrames(v)
	s := v.settingsInteraction()
	send(pressed(key.NameRightArrow))
	if v.appearance.FontFamily != "Alpha Mono" {
		t.Fatal("Right did not cycle past the last family")
	}
	send(pressed(key.NameLeftArrow))
	if v.appearance.FontFamily != defaultTheme.FontFamily {
		t.Fatal("Left did not cycle back to the bundled family")
	}
	send(pressed(key.NameReturn))
	send(key.EditEvent{Range: key.Range{Start: 0, End: len(s.editor.Text())}, Text: "Al"}, key.SelectionEvent{Start: 2, End: 2})
	send(pressed(key.NameLeftArrow))
	start, end := s.editor.Selection()
	if start != 1 || end != 1 || v.appearance.FontFamily != defaultTheme.FontFamily {
		t.Fatalf("Left in the editor: text=%q selection=%d,%d family=%q", s.editor.Text(), start, end, v.appearance.FontFamily)
	}
	v.acceptLoad(loadResult{request: loadRequest{fontFamilies: true}, fontFamilies: []string{"Alpha New"}})
	nextStart, nextEnd := s.editor.Selection()
	if s.editor.Text() != "Al" || nextStart != start || nextEnd != end || !s.editing || s.completion.suffix() != "" {
		t.Fatal("arriving font names disturbed the active editor")
	}
	send(pressed(key.NameEnd))
	if s.completion.suffix() != "pha New" {
		t.Fatal("arriving font names did not become available after moving to the end")
	}
}
