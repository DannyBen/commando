package app

import (
	"fmt"
	"strings"

	"github.com/dannyben/commando/internal/settings"
)

// Resolve on a private candidate: declining the decision or a failed save must
// leave active assignments intact. Only the colliding slot is removed; an
// external command's surviving secondary becomes its primary shortcut.
func reassignBindings(candidate settings.Values, commandID string, slot int) (settings.Values, string, bool) {
	resolved := candidate.Clone()
	catalogue := bindingSettingsCatalogue()
	var messages []string
	for {
		conflict := findBindingConflict(resolved)
		if conflict == nil {
			return resolved, strings.Join(messages, "\n"), len(messages) > 0
		}
		other := conflict.first
		if other.id == commandID && (other.slot == slot || conflict.second.id != commandID) {
			other = conflict.second
		}
		if other.id == "" || conflict.first.id != commandID && conflict.second.id != commandID {
			return settings.Values{}, "", false
		}
		current := view{settingsValues: resolved}
		var owner command
		found := false
		for _, c := range bindingCommands(resolved) {
			if c.id == other.id {
				owner = current.resolveCommandBindings(c)
				found = true
				break
			}
		}
		if !found || owner.bindings[other.slot].Protected {
			return settings.Values{}, "", false
		}
		label := owner.name
		if other.id == commandID {
			label = fmt.Sprintf("Shortcut %d", other.slot+1)
		}
		messages = append(messages, fmt.Sprintf("%s is assigned to %s.", owner.effectiveShortcut(other.slot).String(), label))
		set := func(slot int, value string) bool {
			return resolved.Set(catalogue, bindingSettingKey(owner.id, slot), settings.Value{Text: value}) == nil
		}
		if !set(other.slot, "none") {
			return settings.Values{}, "", false
		}
		if other.id != commandID && other.slot == 0 && owner.effectiveShortcut(1).Name != "" && !owner.bindings[1].Protected {
			if !set(0, owner.effectiveShortcut(1).String()) || !set(1, "none") {
				return settings.Values{}, "", false
			}
		}
	}
}
