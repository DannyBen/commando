package app

import (
	"strings"
	"testing"

	"gioui.org/io/key"
)

func TestRenamePreviewCharacters(t *testing.T) {
	for _, example := range []struct{ before, after, added string }{
		{"hello.jpg", "01 hello.jpg", "01 "},
		{"hello - Copy.jpg", "hello.jpg", ""},
		{"Old.txt", "New.txt", "New"},
		{"abcdef", "abXdeYf", "XY"},
		{"你好.txt", "你们好.txt", "们"},
		{"😀🚀.txt", "🚀.txt", ""},
		{"a.txt", "a.txt", ""},
		{"aaaa", "aaaaa", "a"},
		{"abc", "cab", "c"},
		{"name", "", ""},
		{"", "name", "name"},
		{"", "", ""},
		{strings.Repeat("a", 300), strings.Repeat("b", 300), strings.Repeat("b", 300)},
	} {
		t.Run(example.before+" / "+example.after, func(t *testing.T) {
			var result, added strings.Builder
			for _, span := range renameSpans(example.before, example.after) {
				result.WriteString(span.text)
				if span.added {
					added.WriteString(span.text)
				}
			}
			if result.String() != example.after || added.String() != example.added {
				t.Fatalf("got result %q, additions %q; want %q, %q", result.String(), added.String(), example.after, example.added)
			}
		})
	}
}

func TestMultiRenameTypingValidationStaysSilentAndPersistent(t *testing.T) {
	v := multiRenameView()
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	recorded := &recordedSounds{}
	v.sounds = recorded
	s := v.multiRenameInteraction()
	send(replaceExpression(v, `<`))
	check := func(want string) {
		t.Helper()
		if s.message != want {
			t.Fatalf("message %q, want %q", s.message, want)
		}
		if v.panels[0].feedback.Current() != nil || len(recorded.played) != 0 {
			t.Fatal("typing validation published or sounded a notification")
		}
	}
	check("Invalid expression")
	send(key.EditEvent{Range: key.Range{Start: 1, End: 1}, Text: "n"})
	check("Invalid expression")
	send()
	check("Invalid expression")
	send(key.EditEvent{Range: key.Range{Start: 2, End: 2}, Text: ">"})
	check("")
	s.editor.SetCaret(3, 3)
	for range 3 {
		send(pressed(key.NameDeleteBackward))
		check("Invalid expression")
	}
	send(replaceExpression(v, `<counter> <name>`))
	check("")
	if s.actions.isDisabled(0) {
		t.Fatal("valid expression did not enable Apply")
	}
	// Programmatic edits and editor undo need not produce a ChangeEvent on the
	// same frame; compare the actual text to the last validated expression.
	s.editor.SetText(`<name from:0>`)
	send()
	check("Invalid expression")
}
