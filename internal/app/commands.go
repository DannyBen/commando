package app

import (
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
)

// IDs are persistent identities, independent of labels and catalogue order.
// Availability is checked at execution for both palette and keyboard routes.
type command struct {
	id           string
	group        string
	name         string
	bindings     [2]commandBinding
	overrides    [2]bindingOverride
	available    func(*view) bool
	execute      func(*view) // Workspace actions can run without a keyboard binding.
	paletteQuery string      // Replace the search text while retaining the palette session.
}

// Workspace-dependent commands join their category in catalogue order. The
// palette snapshots this list while its session protects the panel context.
func (v *view) commandCatalogue() []command {
	catalogue := make([]command, 0, len(commands)+len(v.panels))
	for i, c := range commands {
		catalogue = append(catalogue, v.resolveCommandBindings(c))
		if c.group == "Panel" && (i == len(commands)-1 || commands[i+1].group != "Panel") {
			for index := range v.panels {
				catalogue = append(catalogue, v.resolveCommandBindings(panelFocusCommand(index+1)))
			}
		}
	}
	return catalogue
}

func cmd(id, group, name string, nameKey key.Name, modifiers key.Modifiers, available func(*view) bool) command {
	return command{id: id, group: group, name: name, bindings: [2]commandBinding{{Default: shortcut{Name: nameKey, Modifiers: modifiers}}}, available: available}
}

// Catalogue order is the browsing order. Searches rank the complete label.
var commands = defaultCommands()

func defaultCommands() []command {
	catalogue := []command{
		cmd("file.edit", "File", "Edit", key.NameF4, 0, editorReady),
		cmd("entry.open", "Entry", "Open", key.NameReturn, 0, selectedReady),
		cmd("entry.rename", "Entry", "Rename", key.NameF2, 0, func(v *view) bool {
			return selectedReady(v) && v.panels[v.active].markable(v.panels[v.active].selected) && v.actionAllowed(actionMutation)
		}),
		cmd("entry.copy", "Entry", "Copy", "C", key.ModCtrl, clipboardReady),
		cmd("entry.copy_paths", "Entry", "Copy paths", "C", key.ModCtrl|key.ModShift, textClipboardReady),
		cmd("entry.cut", "Entry", "Cut", "X", key.ModCtrl, clipboardReady),
		cmd("entry.paste", "Entry", "Paste", "V", key.ModCtrl, mutationReady),
		cmd("entry.duplicate", "Entry", "Duplicate", "D", key.ModCtrl, selectedMutationReady),
		cmd("entry.copy_to_panel", "Entry", "Copy to panel", key.NameF5, 0, func(v *view) bool { return len(v.panels) > 1 && selectedMutationReady(v) }),
		cmd("entry.move_to_panel", "Entry", "Move to panel", key.NameF6, 0, func(v *view) bool { return len(v.panels) > 1 && selectedMutationReady(v) }),
		cmd("entry.copy_left", "Entry", "Copy to left", "[", 0, func(v *view) bool { return panelTransferReady(v, -1) }),
		cmd("entry.copy_right", "Entry", "Copy to right", "]", 0, func(v *view) bool { return panelTransferReady(v, 1) }),
		cmd("entry.move_left", "Entry", "Move to left", "{", 0, func(v *view) bool { return panelTransferReady(v, -1) }),
		cmd("entry.move_right", "Entry", "Move to right", "}", 0, func(v *view) bool { return panelTransferReady(v, 1) }),
		cmd("entry.recycle", "Entry", "Recycle", key.NameDeleteForward, 0, selectedMutationReady),
		cmd("entry.new_file", "Entry", "New file", "N", key.ModCtrl, writableFolder),
		cmd("entry.new_folder", "Entry", "New folder", "N", key.ModCtrl|key.ModShift, writableFolder),
		cmd("entry.context_menu", "Entry", "Context menu", "/", key.ModAlt, func(v *view) bool { return v.nativeItemsAvailable(v.active, v.panels[v.active].selected, false) }),

		cmd("entry.arm_drag", "Entry", "Arm drag", "D", key.ModCtrl|key.ModShift, func(v *view) bool { return v.dragAvailable(v.active, v.panels[v.active].selected, true) }),

		cmd("navigation.parent_folder", "Navigation", "Parent folder", key.NameDeleteBackward, 0, func(v *view) bool { return folderReady(v) && v.loader != nil }),
		cmd("navigation.drives", "Navigation", "Drives", key.NameHome, key.ModCtrl, func(v *view) bool { return folderReady(v) && v.loader != nil }),
		cmd("navigation.edit_location", "Navigation", "Edit location", "D", key.ModAlt, nil),
		cmd("navigation.next_panel", "Navigation", "Next panel", key.NameTab, 0, nil),
		cmd("navigation.previous_panel", "Navigation", "Previous panel", key.NameTab, key.ModShift, nil),
		cmd("navigation.panel_left", "Navigation", "Panel left", key.NameLeftArrow, 0, func(v *view) bool { return v.active > 0 }),
		cmd("navigation.panel_right", "Navigation", "Panel right", key.NameRightArrow, 0, func(v *view) bool { return v.active+1 < len(v.panels) }),
		cmd("navigation.open_in_left_panel", "Navigation", "Open in left panel", key.NameLeftArrow, key.ModCtrl, func(v *view) bool { return adjacentReady(v, -1) }),
		cmd("navigation.open_in_right_panel", "Navigation", "Open in right panel", key.NameRightArrow, key.ModCtrl, func(v *view) bool { return adjacentReady(v, 1) }),
		cmd("navigation.first_entry", "Navigation", "First entry", key.NameHome, 0, nil),
		cmd("navigation.last_entry", "Navigation", "Last entry", key.NameEnd, 0, nil),
		cmd("navigation.page_up", "Navigation", "Page up", key.NamePageUp, 0, nil),
		cmd("navigation.page_down", "Navigation", "Page down", key.NamePageDown, 0, nil),

		cmd("view.refresh", "View", "Refresh", "R", key.ModCtrl, func(v *view) bool { return panelReady(v) && v.loader != nil }),
		cmd("view.filter_files", "View", "Filter files", "/", 0, folderReady),
		cmd("view.hidden_files", "View", "Hidden files", "H", key.ModCtrl, folderReady),
		cmd("view.next_metadata_column", "View", "Next metadata column", "M", key.ModCtrl, nil),
		cmd("view.previous_metadata_column", "View", "Previous metadata column", "M", key.ModCtrl|key.ModShift, nil),
		cmd("view.sort_by_metadata", "View", "Sort by metadata", key.NameF3, key.ModCtrl, folderReady),

		cmd("panel.move_left", "Panel", "Move left", key.NameLeftArrow, key.ModShift, func(v *view) bool { return v.active > 0 && panelsCanSwap(v) }),
		cmd("panel.move_right", "Panel", "Move right", key.NameRightArrow, key.ModShift, func(v *view) bool { return v.active+1 < len(v.panels) && panelsCanSwap(v) }),

		cmd("panel.add_left", "Panel", "Add left", key.NameLeftArrow, key.ModAlt, panelsCanChange),
		cmd("panel.add_right", "Panel", "Add right", key.NameRightArrow, key.ModAlt, panelsCanChange),
		cmd("panel.close", "Panel", "Close", "W", key.ModCtrl, func(v *view) bool { return len(v.panels) > 1 && panelsCanChange(v) }),
		cmd("panel.reopen_closed", "Panel", "Reopen closed", "W", key.ModCtrl|key.ModShift, func(v *view) bool { return len(v.closedPanels) > 0 && panelsCanChange(v) }),

		cmd("selection.toggle_mark", "Selection", "Toggle mark", key.NameSpace, 0, func(v *view) bool {
			return v.panels[v.active].markable(v.panels[v.active].selected) && v.panels[v.active].prefix == ""
		}),
		cmd("selection.toggle_mark_and_advance", "Selection", "Toggle mark and advance", insertKey, 0, selectedReady),
		cmd("selection.mark_all", "Selection", "Mark all", "A", key.ModCtrl, nil),
		cmd("selection.clear_filter_or_marks", "Selection", "Clear filter or marks", key.NameEscape, 0, nil),

		cmd("feedback.toggle_sound", "Feedback", "Toggle sound", "", 0,
			func(v *view) bool { return v.canOfferInteraction(v.active, actionMutation) }),

		cmd("application.command_palette", "Application", "Command palette", "`", 0, nil),
		{id: "application.open_data_folder", group: "Application", name: "Open data folder",
			available: func(v *view) bool {
				return v.activeDataFolder != "" && v.loader != nil && v.canOfferInteraction(v.active, actionBrowse)
			},
			execute: func(v *view) {
				v.queueLoad(loadRequest{panel: v.active, path: v.activeDataFolder, successMessage: "Data folder"})
			}},
		{id: "application.settings", group: "Application", name: "Settings", bindings: [2]commandBinding{{Default: shortcut{Name: "@"}}}, paletteQuery: "@"},
	}
	for i := range catalogue {
		c := &catalogue[i]
		switch c.id {
		case "entry.paste":
			c.bindings[1].Default = shortcut{Name: insertKey, Modifiers: key.ModShift}
		case "entry.context_menu":
			c.bindings[1].Default = shortcut{Name: contextMenuKey}
		case "application.command_palette":
			c.bindings[1].Default = shortcut{Name: "P", Modifiers: key.ModCtrl | key.ModShift}
		case "selection.clear_filter_or_marks":
			c.bindings[0].Protected = true
		}
	}
	return catalogue
}

func (c command) label() string {
	if c.group == "" {
		return c.name
	}
	return c.group + " › " + c.name
}

func (c command) allowed(v *view) bool { return c.available == nil || c.available(v) }

func (c command) shortcut() string {
	var labels []string
	for slot := range c.bindings {
		if label := c.effectiveShortcut(slot).String(); label != "" {
			labels = append(labels, label)
		}
	}
	return strings.Join(labels, " / ")
}

func (c command) matches(e key.Event) bool {
	for slot := range c.bindings {
		if c.effectiveShortcut(slot).matches(e) {
			return true
		}
	}
	return false
}

func (c command) effectiveShortcut(slot int) shortcut {
	s, err := c.bindings[slot].resolve(c.overrides[slot])
	if err != nil {
		return c.bindings[slot].Default
	}
	return s
}

func (c command) defaultShortcut(s shortcut) bool {
	for _, binding := range c.bindings {
		if binding.Default == s {
			return true
		}
	}
	return false
}

// Palette and keyboard execution share actions, never synthetic key events.
func (v *view) executeCommand(gtx layout.Context, c command) {
	// Opening and going up finish filename-prefix input even when no directory
	// read can start (for example at a root or in a preview).
	if c.id == "entry.open" || c.id == "navigation.parent_folder" || c.id == "navigation.drives" {
		v.panels[v.active].prefix = ""
	}
	if !c.allowed(v) {
		return
	}
	if c.paletteQuery != "" {
		v.openPaletteQuery(gtx, c.paletteQuery)
		return
	}
	if c.execute != nil {
		c.execute(v)
	} else {
		v.executeBuiltinCommand(gtx, c.id)
	}
}

func panelReady(v *view) bool { return !v.panels[v.active].loading }

func folderReady(v *view) bool {
	p := &v.panels[v.active]
	return panelReady(v) && !p.drives && p.path != ""
}

func selectedReady(v *view) bool {
	p := &v.panels[v.active]
	return panelReady(v) && p.selected >= 0 && p.selected < len(p.items)
}

func writableFolder(v *view) bool { return folderReady(v) && v.actionAllowed(actionMutation) }

func mutationReady(v *view) bool { return writableFolder(v) && v.loader != nil }

func selectedMutationReady(v *view) bool {
	return mutationReady(v) && len(v.panels[v.active].selectedPaths()) > 0
}

func clipboardReady(v *view) bool {
	return folderReady(v) && v.loader != nil && v.actionAllowed(actionClipboard) && len(v.panels[v.active].selectedPaths()) > 0
}

func textClipboardReady(v *view) bool {
	return folderReady(v) && v.actionAllowed(actionClipboard) && len(v.panels[v.active].selectedPaths()) > 0
}

func panelsCanSwap(v *view) bool {
	if !v.actionAllowed(actionMutation) {
		return false
	}
	for i := range v.panels {
		if v.panels[i].loading || v.panels[i].refreshing {
			return false
		}
	}
	return true
}

func adjacentReady(v *view, direction int) bool {
	target := v.active + direction
	return selectedReady(v) &&
		v.loader != nil && (target >= 0 && target < len(v.panels) && !v.panels[target].loading || (target < 0 || target >= len(v.panels)) && panelsCanChange(v))
}
