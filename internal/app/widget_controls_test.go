package app

import (
	"image"
	"testing"

	"gioui.org/io/key"
	"gioui.org/unit"
)

func TestWidgetFooterPriority(t *testing.T) {
	for _, tc := range []struct {
		name    string
		footer  widgetFooter
		want    string
		invalid bool
	}{
		{"hint", widgetFooter{hint: "Keys"}, "Keys", false},
		{"help", widgetFooter{hint: "Keys", help: "Example"}, "Example", false},
		{"error", widgetFooter{hint: "Keys", help: "Example", errorText: "Invalid expression"}, "Invalid expression", true},
		{"focused help", widgetFooter{hint: "Keys", help: "Example", errorText: "Invalid expression", helpFocused: true}, "Example", false},
		{"missing help", widgetFooter{hint: "Keys", errorText: "Invalid expression", helpFocused: true}, "Invalid expression", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, invalid := tc.footer.message(); got != tc.want || invalid != tc.invalid {
				t.Fatalf("got %q error=%v", got, invalid)
			}
		})
	}
}

func TestRenameArrowRowsAndFooterPriority(t *testing.T) {
	v := multiRenameView()
	send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	send(pressed(key.NameF2))
	s := v.multiRenameInteraction()
	send(replaceExpression(v, "<name"))
	s.editor.SetCaret(2, 2)
	send()
	if got, invalid := s.footer().message(); got != "Invalid expression" || !invalid {
		t.Fatal("editing did not show validation error")
	}
	send(pressed(key.NameDownArrow))
	if s.footer().rightHint != "Enter to insert" {
		t.Fatal("focused inserter did not show its insertion hint")
	}
	if s.focus != renameFocusInsert || !router.Source().Focused(&v.focus) {
		t.Fatal("Down did not leave the expression for Insert")
	}
	send(pressed(key.NameRightArrow))
	if got, invalid := s.footer().message(); got != renameExamples[1].meaning || invalid {
		t.Fatal("example meaning did not override validation while browsing")
	}
	send(pressed(key.NameDownArrow))
	if s.footer().rightHint != "" {
		t.Fatal("insertion hint remained after leaving the inserter")
	}
	if s.focus != renameFocusApply {
		t.Fatal("Down did not move to Apply")
	}
	if got, invalid := s.footer().message(); got != "Invalid expression" || !invalid {
		t.Fatal("Apply did not restore validation error")
	}
	send(pressed(key.NameRightArrow))
	send(pressed(key.NameDownArrow))
	if s.focus != renameFocusCancel {
		t.Fatal("Down at the final row changed the selected button")
	}
	send(pressed(key.NameUpArrow))
	send(pressed(key.NameUpArrow))
	if s.focus != renameFocusExpression || !router.Source().Focused(&s.editor) {
		t.Fatal("Up did not return to the expression")
	}
	if start, end := s.editor.Selection(); start != 2 || end != 2 || s.editor.Text() != "<name" {
		t.Fatal("row navigation changed the expression or caret")
	}
	send(pressed(key.NameUpArrow))
	if s.focus != renameFocusExpression {
		t.Fatal("Up at the first row moved focus")
	}
}

func TestWidgetEditorArrowNavigationValidatesDraft(t *testing.T) {
	for _, direction := range []key.Name{key.NameUpArrow, key.NameDownArrow} {
		t.Run(string(direction), func(t *testing.T) {
			v := newPreviewView()
			v.startSettingsWidget("appearance.font")
			s := v.settingsInteraction()
			s.selected = 1 // Text size, between font family and small text size.
			send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
			send(pressed(key.NameReturn))
			s.editor.SetText("invalid")
			send()
			send(pressed(direction))
			if !s.editing || s.selected != 1 || s.errorText == "" || !router.Source().Focused(&s.editor) {
				t.Fatal("arrow discarded an invalid draft")
			}
			s.editor.SetText("22")
			send()
			send(pressed(direction))
			want := 0
			if direction == key.NameDownArrow {
				want = 2
			}
			if s.editing || s.selected != want || s.actionsFocused || s.errorText != "" || !router.Source().Focused(&v.focus) || v.settingText(settingFontTextSize) != "22" {
				t.Fatal("arrow did not preview the draft and focus the adjacent row")
			}
		})
	}
}

func TestWidgetEditorDownToActionsAndUpFromActions(t *testing.T) {
	v := newPreviewView()
	v.startSettingsWidget("application.data_folder")
	s := v.settingsInteraction()
	send, _ := panelInputFrames(v)
	send(pressed(key.NameReturn))
	before := s.editor.Text()
	start, end := s.editor.Selection()
	send(pressed(key.NameUpArrow))
	if !s.editing || s.actionsFocused {
		t.Fatal("Up at the first row left the editor")
	}
	if gotStart, gotEnd := s.editor.Selection(); gotStart != start || gotEnd != end {
		t.Fatal("boundary navigation changed the editor selection")
	}
	send(key.Event{Name: key.NameDownArrow, State: key.Release})
	if !s.editing {
		t.Fatal("key release moved focus")
	}
	send(pressed(key.NameDownArrow))
	if s.editing || !s.actionsFocused || s.action != 0 || v.settingText(settingDataFolder) != before {
		t.Fatal("Down did not preview the last field and focus Save")
	}
	send(pressed(key.NameUpArrow))
	if s.actionsFocused || s.selected != 0 {
		t.Fatal("Up did not return from Save to the field")
	}
}
