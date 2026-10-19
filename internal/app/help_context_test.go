package app

import (
	"os"
	"path/filepath"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
)

func TestHelpFollowsPaletteWidgetAndFocusedControl(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	wantHelp := func(want string) {
		t.Helper()
		id, _ := defaultHelp.resolve(v.helpContextIDs()...)
		if id != want {
			t.Fatalf("help = %q, want %q; candidates = %v", id, want, v.helpContextIDs())
		}
	}
	widgetID := "settings." + settingThemeGenerator
	modeID := widgetID + ".mode"
	dynamicsID := widgetID + ".dynamics"
	wantHelp("browsing.introduction")
	height := aboveHelpFooter(v, f32.Point{}).Y
	send(pressed("`"))
	wantHelp("palette.file.edit")
	send(replacePalette(v, "@no such setting"))
	wantHelp("palette.settings")
	send(replacePalette(v, "@generator"))
	wantHelp("palette." + widgetID)
	// Even if the palette-specific entry is missing, widget instructions must
	// never appear before the widget has actually opened.
	withoutPaletteTip := helpCatalogue{
		widgetID:           defaultHelp[widgetID],
		"palette.settings": defaultHelp["palette.settings"],
		"help":             defaultHelp["help"],
	}
	if id, _ := withoutPaletteTip.resolve(v.helpContextIDs()...); id != "palette.settings" {
		t.Fatal("palette selection fell back to widget instructions", id)
	}
	send(pressed(key.NameReturn))
	s := v.settingsInteraction()
	if s == nil || s.fields[s.selected].Key != settingThemeGenerator+".mode" {
		t.Fatal("palette did not open generator at Mode")
	}
	wantHelp(modeID)
	send(pressed(key.NameDownArrow), pressed(key.NameDownArrow), pressed(key.NameDownArrow))
	if s.fields[s.selected].Key != settingThemeGenerator+".dynamics" {
		t.Fatal("keyboard did not focus Dynamics")
	}
	wantHelp(dynamicsID)
	send(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(20, 100)})
	wantHelp(dynamicsID) // Hover is not help ownership.
	send(pressed(key.NameReturn))
	if !s.editing {
		t.Fatal("Dynamics editor did not open")
	}
	wantHelp(dynamicsID) // No editing-specific entry: retain the control explanation.
	send(pressed(key.NameEscape))
	send(pressed(key.NameTab), pressed(key.NameTab))
	if !s.actionsFocused {
		t.Fatal("Tab did not focus the widget actions")
	}
	wantHelp("settings.actions.save") // Actions must not retain the selected field's explanation.
	send(pressed(key.NameUpArrow), pressed(key.NameUpArrow))
	wantHelp(dynamicsID)
	question := &confirmationSession{message: "Continue?", acceptLabel: "Continue"}
	if !v.startConfirmation(layout.Context{}, question) {
		t.Fatal("confirmation did not suspend the widget")
	}
	send()
	wantHelp("confirmation") // The question must not describe the suspended field.
	send(pressed(key.NameEscape), key.Event{Name: key.NameEscape, State: key.Release})
	wantHelp(dynamicsID)
	if v.settingsInteraction() != s {
		t.Fatal("confirmation replaced the widget")
	}
	if aboveHelpFooter(v, f32.Point{}).Y != height {
		t.Fatal("context changes altered the footer height")
	}
	send(pressed(key.NameF1), key.Event{Name: key.NameF1, State: key.Release})
	send(pressed(key.NameUpArrow))
	wantHelp(widgetID + ".vibrance")
	send(pressed(key.NameF1), key.Event{Name: key.NameF1, State: key.Release})
	if v.helpHidden {
		t.Fatal("help did not reopen")
	}
	wantHelp(widgetID + ".vibrance")
	send(pressed(key.NameEscape)) // Return to the originating settings palette.
	send(pressed(key.NameEscape)) // Dismiss the originating settings palette.
	wantHelp("browsing.introduction")
}

func TestHelpContextFallbackOrder(t *testing.T) {
	field := settings.Field{Key: settingThemeGenerator + ".dynamics", Widget: settingThemeGenerator}
	for _, tc := range []struct {
		name  string
		owner interactionSession
		first string
	}{
		{"field", &settingsSession{fields: []settings.Field{field}}, "settings." + field.Key},
		{"editing", &settingsSession{fields: []settings.Field{field}, editing: true}, "settings." + field.Key + ".editing"},
		{"save", &settingsSession{fields: []settings.Field{field}, actionsFocused: true}, "settings.actions.save"},
		{"cancel", &settingsSession{fields: []settings.Field{field}, actionsFocused: true, action: 1}, "settings.actions.cancel"},
		{"reset", &settingsSession{fields: []settings.Field{field}, actionsFocused: true, action: 2}, "settings.actions.reset"},
		{"delete", &settingsSession{fields: []settings.Field{field}, actionsFocused: true, action: 2, collection: settings.EditorCollection}, "settings.actions.delete"},
		{"saving", &settingsSession{fields: []settings.Field{field}, pending: true}, "settings.saving"},
		{"selecting", &settingsSession{fields: []settings.Field{field}, selecting: true}, "settings.selecting"},
		{"capture", &settingsSession{fields: []settings.Field{field}, capturing: true}, "settings.shortcuts.capture"},
		{"empty widget", &settingsSession{}, "settings"},
		{"selected command", &paletteSession{results: []commandMatch{{command: command{id: "entry.rename"}}}}, "palette.entry.rename"},
		{"empty palette", &paletteSession{}, "palette"},
		{"location", &locationSession{}, "editing.location"},
		{"name", &nameSession{}, "editing.name"},
		{"filter", &filterSession{}, "editing.filter"},
		{"inline action", &inlineActionSession{}, "actions"},
		{"target panel", &inlineActionSession{navigate: func(int, bool) {}}, "actions.target_panel"},
		{"target panel choices", &inlineActionSession{navigate: func(int, bool) {}, options: []string{"Here", "New folder"}}, "actions.target_panel.choices"},
		{"confirmation", &confirmationSession{}, "confirmation"},
		{"native", &nativeSession{}, "browsing.introduction"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newPreviewView()
			v.interaction.current = tc.owner
			ids := v.helpContextIDs()
			if ids[0] != tc.first || ids[len(ids)-1] != "help" {
				t.Fatal("unexpected fallback candidates", ids)
			}
			catalogue := helpCatalogue{}
			for _, id := range ids {
				catalogue[id] = helpTip{}
			}
			// Removing each entry should reveal exactly its next fallback.
			for _, want := range ids {
				got, _ := catalogue.resolve(ids...)
				if got != want {
					t.Fatalf("fallback = %q, want %q", got, want)
				}
				delete(catalogue, want)
			}
		})
	}
}

func TestHelpContextPreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_HELP")
	if dir == "" {
		t.Skip("optional contextual help previews")
	}
	controls := map[string]struct{ widget, field string }{
		"font":       {"appearance.font", "text_size"},
		"sounds":     {"feedback.sounds", "profile"},
		"effects":    {"appearance.theme.effects", "backdrop_wash"},
		"metadata":   {"view.metadata_columns", "modified"},
		"text-color": {"appearance.theme.colors.text", "lightness"},
	}
	for _, context := range []string{"palette", "palette-generator", "generator", "seed", "vibrance", "dynamics", "color-shift", "font", "sounds", "effects", "metadata", "text-color", "location", "name", "filter", "actions", "confirmation"} {
		v := newPreviewView()
		if context == "palette" {
			v.startPalette(layout.Context{})
		} else if context == "palette-generator" {
			v.openPaletteQuery(layout.Context{}, "@generator")
		} else if context == "location" {
			v.startLocation(layout.Context{})
		} else if context == "name" {
			v.startName(layout.Context{}, nameRename)
		} else if context == "filter" {
			v.startFilter(layout.Context{})
		} else if context == "actions" {
			v.startInlineActions(0, 4, "", "", []string{"Copy here"}, nil, nil)
			v.inlineInteraction().navigate = func(int, bool) {}
		} else if context == "confirmation" {
			v.startConfirmation(layout.Context{}, &confirmationSession{message: "Continue?", acceptLabel: "Continue"})
		} else {
			control, ok := controls[context]
			if !ok {
				control.widget = settingThemeGenerator
				control.field = map[string]string{"generator": "mode", "seed": "seed", "vibrance": "vibrance", "dynamics": "dynamics", "color-shift": "color_shift"}[context]
			}
			v.startSettingsWidget(control.widget)
			for i, spec := range v.settingsInteraction().fields {
				if spec.Key == control.widget+"."+control.field {
					v.settingsInteraction().selected = i
				}
			}
			if got, _ := defaultHelp.resolve(v.helpContextIDs()...); got != "settings."+control.widget+"."+control.field {
				t.Fatalf("preview %s resolved help %s", context, got)
			}
		}
		if err := renderViewPNG(filepath.Join(dir, "help-"+context+".png"), v); err != nil {
			t.Fatal(err)
		}
	}
}
