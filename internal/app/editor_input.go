package app

import (
	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"image/color"
	"unicode"
)

// Request the same clipboard transfer used by Gio's Ctrl+V handling. The editor
// then inserts it at its selection with its usual caret and undo behavior.
func pasteWithInsert(gtx layout.Context, editor *widget.Editor, ke key.Event) {
	if ke.Name == insertKey && ke.State == key.Press && ke.Modifiers == key.ModShift &&
		!editor.ReadOnly && gtx.Focused(editor) {
		gtx.Execute(clipboard.ReadCmd{Tag: editor})
	}
}

// Delete through the public editor API so Unicode selection and Undo remain
// native. Ctrl+Backspace removes a selection, otherwise the preceding word
// together with trailing whitespace.
func deletePreviousWord(editor *widget.Editor) bool {
	if editor.ReadOnly {
		return false
	}
	start, end := editor.Selection()
	if start == end {
		runes := []rune(editor.Text())
		for start > 0 && unicode.IsSpace(runes[start-1]) {
			start--
		}
		for start > 0 && !unicode.IsSpace(runes[start-1]) {
			start--
		}
		if start == end {
			return false
		}
		editor.SetCaret(start, end)
	}
	return editor.Delete(1) != 0
}

func (v *view) updateEditorInput(gtx layout.Context, editor *widget.Editor) {
	for {
		e, ok := gtx.Event(key.Filter{Focus: editor, Name: insertKey, Required: key.ModShift},
			key.Filter{Focus: editor, Name: key.NameDeleteBackward, Required: key.ModCtrl})
		if !ok {
			return
		}
		if ke, ok := e.(key.Event); ok {
			if ke.State == key.Press {
				v.dismissFeedback()
			}
			if ke.Name == key.NameDeleteBackward {
				if ke.State == key.Press && ke.Modifiers == key.ModCtrl && gtx.Focused(editor) {
					deletePreviousWord(editor)
				}
			} else {
				pasteWithInsert(gtx, editor, ke)
			}
		}
	}
}

func (v *view) textEditor(editor *widget.Editor, hint string) material.EditorStyle {
	style := material.Editor(v.theme, editor, hint)
	style.Font.Weight = v.appearance.FontWeight
	style.TextSize = v.appearance.TextSize
	style.Color = v.resolvedText(v.appearance.Colors.EditorText)
	style.SelectionColor = v.resolvedTextSelection(style.Color, v.currentTextBackground())
	style.HintColor = v.hintColor()
	return style
}

// Share Gio's editor-placeholder dimming with passive listing hints.
func (v *view) hintColor() color.NRGBA {
	return v.resolvedText(material.Editor(v.theme, nil, "").HintColor)
}
