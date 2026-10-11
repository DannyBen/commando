package app

import (
	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (v *view) activeEditor() *widget.Editor {
	if v.naming != nil {
		return &v.nameEditor
	}
	if v.editing {
		return &v.location
	}
	return nil
}

// Request the same clipboard transfer used by Gio's Ctrl+V handling. The editor
// then inserts it at its selection with its usual caret and undo behavior.
func pasteWithInsert(gtx layout.Context, editor *widget.Editor, ke key.Event) {
	if ke.Name == insertKey && ke.State == key.Press && ke.Modifiers == key.ModShift &&
		!editor.ReadOnly && gtx.Focused(editor) {
		gtx.Execute(clipboard.ReadCmd{Tag: editor})
	}
}

func (v *view) updateEditorPaste(gtx layout.Context, editor *widget.Editor) {
	for {
		e, ok := gtx.Event(key.Filter{Focus: editor, Name: insertKey, Required: key.ModShift})
		if !ok {
			return
		}
		if ke, ok := e.(key.Event); ok {
			if ke.State == key.Press {
				v.dismissFeedback()
			}
			pasteWithInsert(gtx, editor, ke)
		}
	}
}

func (v *view) textEditor(editor *widget.Editor, hint string) material.EditorStyle {
	style := material.Editor(v.theme, editor, hint)
	style.TextSize = v.appearance.TextSize
	style.Color, style.SelectionColor = v.appearance.Colors.EditorText, v.appearance.Colors.TextSelection
	return style
}
