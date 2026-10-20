package app

import (
	"gioui.org/io/key"
	"gioui.org/layout"
)

type renameExample struct{ expression, meaning string }

// Ordered teaching examples, not a catalogue of every possible combination.
// Selection is passive; only Enter inserts into the expression editor.
var renameExamples = []renameExample{
	{`<name>`, "Full original filename"},
	{`<base>`, "Name without its final extension"},
	{`<ext>`, "Extension without its dot"},
	{`<counter>`, "Counter starting at 1"},
	{`<counter pad:0>`, "Counter with automatic zero padding"},
	{`<counter pad:0 min:1>`, "Counter with an extra leading zero"},
	{`<counter start:10 step:5>`, "Counter starting at 10, increasing by 5"},
	{`Prefix-<name>`, "Add a prefix to the full name"},
	{`<base>-final.<ext>`, "Add a suffix before the extension"},
	{`<base>[final].<ext>`, "Square brackets are literal text"},
	{`<name remove:" - Copy">`, "Remove every occurrence of ' - Copy'"},
	{`<name replace:Old with:New>`, "Replace every Old with New"},
	{`<name replace:"Old Text" with:X>`, "Quote values containing spaces"},
	{`<base case:lower>.<ext>`, "Lowercase the base; preserve the extension"},
	{`<base case:upper>.<ext>`, "Uppercase the base; preserve the extension"},
	{`<name from:1 to:10>`, "First 10 characters of the full name"},
	{`<name from:11>`, "Full name from character 11 onward"},
	{`<name to:10>`, "First 10 characters, with a shorter slice"},
	{`<n>`, "Short form of <name>"},
	{`<b>.<e>`, "Short forms of <base> and <ext>"},
	{`<c pad:0> <n>`, "Padded counter followed by the full name"},
	{`<b to:5>-<b from:6>.<e>`, "Insert a dash after 5 base characters"},
	{`<name remove:Copy | case:lower>`, "Remove Copy, then lowercase the result"},
	{`<n to:10 | replace:Old with:New>`, "Take 10 characters, then replace Old"},
	{`<n replace:Old with:New | to:10>`, "Replace Old, then take 10 characters"},
}

const (
	renameFocusExpression = iota
	renameFocusInsert
	renameFocusApply
	renameFocusCancel
	renameFocusCount
)

func (v *view) selectRenameExample(s *multiRenameSession, index int) {
	if s.pending || index < 0 || index >= len(renameExamples) || index == s.example {
		return
	}
	s.example, s.exampleBrowsed = index, true
	v.announceInteraction(eventSelectionChanged)
}

func (v *view) stepRenameExample(s *multiRenameSession, direction int) {
	v.selectRenameExample(s, (s.example+direction+len(renameExamples))%len(renameExamples))
}

func (v *view) insertRenameExample(gtx layout.Context, s *multiRenameSession) {
	if s.pending {
		return
	}
	// Editor.Insert replaces the retained selection, supports Unicode offsets,
	// and joins the editor's ordinary Undo history. No SetText round trip.
	s.editor.Insert(renameExamples[s.example].expression)
	v.previewMultiRename(s)
	s.focus = renameFocusExpression
	v.announceInteraction(eventEditConfirmed)
	v.focusInteraction(gtx)
}

func (v *view) updateRenameExamplePointer(gtx layout.Context, s *multiRenameSession) (focused bool) {
	focus := func() { focused = true; s.focus = renameFocusInsert; v.focusInteraction(gtx) }
	if s.exampleTarget.Clicked(gtx) {
		focus()
	}
	for {
		direction, ok := s.exampleTarget.scroll(gtx)
		if !ok {
			break
		}
		if direction != 0 {
			focus()
			v.stepRenameExample(s, direction)
		}
	}
	return focused
}

func (v *view) renameExampleKey(gtx layout.Context, s *multiRenameSession, ke key.Event) bool {
	if s.focus != renameFocusInsert || ke.Modifiers != 0 {
		return false
	}
	switch ke.Name {
	case key.NameLeftArrow:
		v.stepRenameExample(s, -1)
	case key.NameRightArrow:
		v.stepRenameExample(s, 1)
	case key.NameHome:
		v.selectRenameExample(s, 0)
	case key.NameEnd:
		v.selectRenameExample(s, len(renameExamples)-1)
	case key.NameReturn, key.NameEnter:
		v.insertRenameExample(gtx, s)
	default:
		return false
	}
	return true
}

func (s *multiRenameSession) footer() widgetFooter {
	if s.pending {
		return widgetFooter{hint: "Renaming"}
	}
	footer := widgetFooter{hint: "Ctrl+Enter Apply · Esc Cancel", errorText: s.message}
	if s.focus == renameFocusInsert || s.exampleBrowsed {
		footer.help = renameExamples[s.example].meaning
		footer.helpFocused = s.focus == renameFocusInsert
	}
	if s.focus == renameFocusInsert {
		footer.rightHint = "Enter to insert"
	}
	return footer
}
