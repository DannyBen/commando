package app

import (
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/settings"
)

func TestBooleanToggleCommandsApplyPersistAndBind(t *testing.T) {
	for _, tc := range []struct{ id, label, field string }{
		{"view.toggle_parent_entry", "View › Toggle parent entry", settingParentEntry},
		{"view.toggle_protected_system_files", "View › Toggle protected system files", settingProtectedSystem},
		{"appearance.toggle_icons", "Appearance › Toggle icons", settingIconsEnabled},
		{"appearance.toggle_auto_color", "Theme › Toggle file colors", settingAutoColorEnabled},
		{"appearance.toggle_contrast_guard", "Theme › Toggle contrast guard", settingContrastGuard},
		{"feedback.toggle_notification_animation", "Feedback › Toggle notification animation", settingNotificationAnimation},
	} {
		t.Run(tc.id, func(t *testing.T) {
			v := newPreviewView()
			dir := t.TempDir()
			catalogue := bindingSettingsCatalogue()
			v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
				return func() (string, error) { return "", catalogue.Save(dir, values) }
			}
			before := v.settingsValues.Clone()
			send, _ := panelInputFrames(v)
			send(pressed("`"))
			send(key.EditEvent{Text: tc.label})
			p := v.paletteInteraction()
			if len(p.results) == 0 || p.results[0].command.id != tc.id || p.results[0].command.shortcut() != "" {
				t.Fatal("toggle must be discoverable and initially unbound")
			}
			send(pressed(key.NameReturn))
			loaded, err := settings.LoadAndRepair(dir, catalogue)
			if err != nil {
				t.Fatal(err)
			}
			for _, definition := range settings.CatalogueDefault.Settings {
				want := before.Get(catalogue, definition.Key)
				if definition.Key == tc.field {
					want.Bool = !want.Bool
				}
				if v.settingsValues.Get(catalogue, definition.Key) != want || loaded.Get(catalogue, definition.Key) != want {
					t.Fatal("toggle changed an unrelated preference or did not save", definition.Key)
				}
			}
			setTestShortcut(t, v, tc.id, 0, "Ctrl+Y")
			v.applySettings()
			send(modified("Y", key.ModCtrl))
			if v.settingsValues.Get(catalogue, tc.field) != before.Get(catalogue, tc.field) || !v.interaction.idle() {
				t.Fatal("bound toggle did not restore the setting")
			}
		})
	}
}
