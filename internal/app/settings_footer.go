package app

import "github.com/dannyben/commando/internal/settings"

func settingsFooter(s *settingsSession) widgetFooter {
	field := s.fields[s.selected]
	hint := "Enter Type · Ctrl+Enter Save"
	if field.Control == "stepper" || field.Control == "stepper-progress" {
		hint = "←/→ Adjust · Enter Type · Ctrl+Enter Save"
		if field.Key == settingThemeGenerator+".seed" {
			hint = "←/→ Adjust · End Random seed · Enter Type · Ctrl+Enter Save"
		}
	}
	if field.Control == "toggle" {
		hint = "←/→ Choose · Enter Toggle · Ctrl+Enter Save"
	}
	if field.Control == "choice" {
		hint = "←/→ Choose · Enter Cycle · Ctrl+Enter Save"
	}
	if field.IsPath() {
		hint = "←/→ Choose folder · Enter Type · Ctrl+Enter Save"
	}
	if field.Control == "file" {
		hint = "←/→ Choose file · Enter Type · Ctrl+Enter Save"
	}
	if field.Control == "shortcut" {
		hint = "Enter Capture · Del Clear · Ctrl+Enter Save"
		if !shortcutFieldEditable(s, s.selected) {
			hint = "Protected shortcut · Ctrl+Enter Save"
		}
		if s.capturing {
			hint = "Press shortcut · Esc Cancel"
		}
	}
	if field.Key == settings.EditorCollection+".ext" {
		hint = "Space-separated extensions · * Fallback"
	}
	if field.Key == settings.EditorCollection+".args" {
		hint = "%SelectedFiles% · One launch per rule"
	}
	if s.selecting {
		hint = "Browse panels · Ctrl+Enter Save · Esc Return"
		if field.Control == "file" {
			hint = "Enter Choose file · Esc Return"
		}
	}
	if s.editing {
		hint = "Enter Preview · Ctrl+Enter Save · Esc Revert"
		if s.completesField() && s.completion.suffix() != "" {
			hint = "Tab Complete · ↑/↓ Suggest · Enter Preview"
		}
	} else if s.completesField() {
		hint = "←/→ Font · Enter Type · Ctrl+Enter Save"
		if field.Control == "autocomplete-stepper" {
			hint = "←/→ Choose · Enter Type · Ctrl+Enter Save"
		}
		if field.Key == settingThemeSelect {
			hint = "←/→ Theme · Enter Type · Ctrl+Enter Save"
		}
	}
	if s.actionsFocused && !s.selecting && !s.editing && !s.capturing {
		hint = "Enter Save changes"
		if s.action == 1 {
			hint = "Enter Cancel changes"
		}
		if s.action == 2 {
			hint = "Enter Preview defaults"
			if s.collection != "" {
				hint = "Enter Delete rule"
			}
		}
		hint += " · ←/→ Action · Ctrl+Enter Save"
	}
	if s.deleted {
		hint = "Ctrl+Enter Save deletion · Cancel Discard"
	}
	if isThemeWidget(s) && s.selected == 0 && !s.editing && !s.actionsFocused {
		hint = "←/→ Element · Ctrl+C Copy · Ctrl+V Paste"
	}
	return widgetFooter{hint: hint, errorText: s.errorText}
}
