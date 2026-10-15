package app

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/settings"
)

func bindingSettingKey(id string, slot int) string {
	return "keybindings." + id + ".shortcut_" + strconv.Itoa(slot+1)
}

// Bindings are derived from command identities, rather than repeating the
// command list in settings.toml. They have no ordinary settings widgets.
func bindingSettingsCatalogue() settings.Catalogue {
	catalogue := settings.CatalogueDefault
	catalogue.Settings = slices.Clone(catalogue.Settings)
	for _, c := range bindingCommands(settings.Values{}) {
		for slot := range c.bindings {
			catalogue.Settings = append(catalogue.Settings, bindingDefinition(c, slot))
		}
	}
	catalogue.DynamicDefinition = dynamicBindingDefinition
	catalogue.DynamicGroup = func(group string) bool {
		_, ok := panelFocusNumber(strings.TrimPrefix(group, "keybindings."))
		return strings.HasPrefix(group, "keybindings.") && ok
	}
	catalogue.ValidateOverrides = validateBindingOverrides
	return catalogue
}

func bindingDefinition(c command, slot int) settings.Definition {
	return settings.Definition{Key: bindingSettingKey(c.id, slot), Kind: settings.String,
		Validate: func(value settings.Value) error {
			override, err := parseBindingOverride(value.Text)
			if err != nil {
				return err
			}
			if _, err := c.bindings[slot].resolve(override); err != nil {
				return err
			}
			if override.Mode != bindingAssigned {
				return nil
			}
			s := override.Shortcut
			if s.Name == key.NameSpace || len(s.Name) == 1 {
				if s.Modifiers&(key.ModCtrl|key.ModAlt|key.ModSuper|key.ModCommand) == 0 && !c.defaultShortcut(s) {
					return errors.New("use a modified key")
				}
			}
			if s.Name == key.NameReturn && s.Modifiers&key.ModAlt != 0 || s.Name == contextMenuKey && s.Modifiers != 0 {
				return errors.New("unsupported native shortcut")
			}
			return nil
		},
	}
}

func (v *view) resolveCommandBindings(c command) command {
	for slot := range c.bindings {
		// Missing values resolve to the zero override, which inherits. The
		// settings catalogue validates explicit values before applying them.
		text := v.settingsValues.Get(settings.CatalogueDefault, bindingSettingKey(c.id, slot)).Text
		c.overrides[slot], _ = parseBindingOverride(text)
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
	for _, c := range bindingCommands(values) {
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
