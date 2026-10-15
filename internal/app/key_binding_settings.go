package app

import (
	"errors"
	"slices"
	"strconv"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/settings"
)

func bindingSettingKey(id string, slot int) string {
	return "keybindings." + id + ".shortcut_" + strconv.Itoa(slot+1)
}

// Text-triggered commands and native aliases keep their existing routes until
// those routes support editing. Generated commands join in a later stage too.
func bindingConfigurable(c command, slot int) bool {
	switch c.id {
	case "view.filter_files", "application.settings", "application.command_palette", "entry.context_menu":
		return false
	case "entry.paste":
		return slot == 0
	}
	return true
}

// Bindings are derived from command identities, rather than repeating the
// command list in settings.toml. They have no ordinary settings widgets.
func bindingSettingsCatalogue() settings.Catalogue {
	catalogue := settings.CatalogueDefault
	catalogue.Settings = slices.Clone(catalogue.Settings)
	for _, c := range commands {
		for slot, binding := range c.bindings {
			if !bindingConfigurable(c, slot) {
				continue
			}
			catalogue.Settings = append(catalogue.Settings, settings.Definition{
				Key: bindingSettingKey(c.id, slot), Kind: settings.String,
				Validate: func(value settings.Value) error {
					override, err := parseBindingOverride(value.Text)
					if err != nil {
						return err
					}
					if _, err := binding.resolve(override); err != nil {
						return err
					}
					if override.Mode != bindingAssigned {
						return nil
					}
					s := override.Shortcut
					if s.Name == key.NameSpace || len(s.Name) == 1 {
						if s.Modifiers&(key.ModCtrl|key.ModAlt|key.ModSuper|key.ModCommand) == 0 && s != binding.Default {
							return errors.New("text triggers are not configurable yet")
						}
					}
					if s.Name == key.NameReturn && s.Modifiers&key.ModAlt != 0 || s.Name == contextMenuKey && s.Modifiers != 0 {
						return errors.New("unsupported native shortcut")
					}
					return nil
				},
			})
		}
	}
	catalogue.ValidateOverrides = validateBindingOverrides
	return catalogue
}

func (v *view) resolveCommandBindings(c command) command {
	for slot := range c.bindings {
		if bindingConfigurable(c, slot) {
			// Missing values resolve to the zero override, which inherits. The
			// settings catalogue validates explicit values before applying them.
			text := v.settingsValues.Get(settings.CatalogueDefault, bindingSettingKey(c.id, slot)).Text
			c.overrides[slot], _ = parseBindingOverride(text)
		}
	}
	return c
}

// A conflicting set is one invalid group. Restore its defaults together so
// recovery cannot create a second conflict by reverting only one side of a swap.
// The existing settings recovery flow backs up the original document first.
func validateBindingOverrides(values *settings.Values) error {
	if conflict := findBindingConflict(*values); conflict != nil {
		values.Reset("keybindings")
		return conflict
	}
	return nil
}

type bindingOwner struct {
	id, name string
	slot     int
}

type bindingConflict struct {
	first, second bindingOwner
}

func (*bindingConflict) Error() string { return "conflicting key bindings" }
func (*bindingConflict) Unwrap() error { return settings.ErrValue }

func findBindingConflict(values settings.Values) *bindingConflict {
	v := view{settingsValues: values}
	used := map[shortcut]bindingOwner{}
	for _, name := range []key.Name{key.NameUpArrow, key.NameDownArrow} {
		used[shortcut{Name: name}] = bindingOwner{name: "Row navigation"}
		used[shortcut{Name: name, Modifiers: key.ModShift}] = bindingOwner{name: "Range selection"}
	}
	// Numbered panel shortcuts retain their identities even in a smaller workspace.
	for number := 1; number <= 9; number++ {
		used[shortcut{Name: key.Name(strconv.Itoa(number)), Modifiers: key.ModAlt}] = bindingOwner{name: "Focus " + strconv.Itoa(number)}
	}
	for _, c := range commands {
		c = v.resolveCommandBindings(c)
		for slot := range c.bindings {
			s := c.effectiveShortcut(slot)
			if s.Name == "" {
				continue
			}
			owner := bindingOwner{id: c.id, name: c.name, slot: slot}
			if previous, exists := used[s]; exists {
				return &bindingConflict{first: previous, second: owner}
			}
			used[s] = owner
		}
	}
	return nil
}
