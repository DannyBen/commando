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
		commandID := strings.TrimPrefix(group, "keybindings.")
		if commandID == strings.TrimSuffix(editorRuleCommandPrefix, ".") {
			return true
		}
		if _, ok := editorRuleBindingID(commandID); ok {
			return true
		}
		_, ok := panelFocusNumber(commandID)
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
				if s.Modifiers&(key.ModCtrl|key.ModAlt|key.ModSuper|key.ModCommand) == 0 && s.textTrigger() == "" && !c.defaultShortcut(s) {
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

// Saved assignments take precedence over inherited defaults introduced by an
// upgrade. Explicit conflicts still recover as a group, preserving valid swaps.
// The shortcut editor checks conflicts before reaching this normalization.
func validateBindingOverrides(values *settings.Values) error {
	pruneRuleBindings(values)
	if err := unassignConflictingDefaults(values); err != nil {
		return err
	}
	if conflict := findBindingConflict(*values); conflict != nil {
		values.Reset("keybindings")
		return conflict
	}
	return nil
}

func unassignConflictingDefaults(values *settings.Values) error {
	v := view{settingsValues: *values}
	catalogue := bindingCommands(*values)
	assigned := map[shortcut]bool{}
	for i, c := range catalogue {
		c = v.resolveCommandBindings(c)
		catalogue[i] = c
		for _, override := range c.overrides {
			if override.Mode == bindingAssigned {
				assigned[override.Shortcut] = true
			}
		}
	}
	definitions := bindingSettingsCatalogue()
	for _, c := range catalogue {
		for slot, binding := range c.bindings {
			if c.overrides[slot].Mode != bindingInherited || binding.Protected || !assigned[binding.Default] {
				continue
			}
			if err := values.Set(definitions, bindingSettingKey(c.id, slot), settings.Value{Text: "none"}); err != nil {
				return err
			}
		}
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
