package app

import (
	"github.com/dannyben/commando/internal/diagnostics"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
)

// Presentation changes palette discovery without changing global command identity.
type commandPresentation struct {
	name   string
	hidden bool // Palette visibility does not affect bindings or execution.
}

// IDs are persistent identities, independent of labels and catalogue order.
// Availability is checked at execution for both palette and keyboard routes.
type command struct {
	presentation  func(*view) commandPresentation
	mask          commandPresentation
	id            string
	group         string
	name          string
	bindings      [2]commandBinding
	overrides     [2]bindingOverride
	available     func(*view) bool
	execute       func(*view) // Workspace actions can run without a keyboard binding.
	paletteQuery  string      // Replace the search text while retaining the palette session.
	toggleSetting string      // Boolean setting changed through shared live application and persistence.
}

// Workspace-dependent commands join their category in catalogue order. The
// palette snapshots this list while its session protects the panel context.
func (v *view) commandCatalogue() []command {
	catalogue := make([]command, 0, len(commands)+len(v.panels))
	for i, c := range commands {
		if c.presentation != nil {
			c.mask = c.presentation(v)
		}
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

func toggleSettingCommand(id, group, name, field string) command {
	c := cmd(id, group, "Toggle "+name, "", 0,
		func(v *view) bool { return v.canOfferInteraction(v.active, actionMutation) })
	c.toggleSetting = field
	return c
}

// Catalogue order is the browsing order. Searches rank the complete label.
var commands = defaultCommands()

func defaultCommands() []command {
	catalogue := []command{
		cmd("file.edit", "File", "Edit", key.NameF4, 0, editorReady),
		cmd("entry.open", "Entry", "Open", key.NameReturn, 0, openReady),
		cmd("entry.rename", "Entry", "Rename", key.NameF2, 0, func(v *view) bool {
			return selectedReady(v) && v.panels[v.active].location().Capabilities().Rename && v.panels[v.active].markable(v.panels[v.active].selected) && v.actionAllowed(actionMutation)
		}),
		cmd("entry.copy", "Entry", "Copy", "C", key.ModCtrl, copyReady),
		cmd("entry.copy_paths", "Entry", "Copy paths", "C", key.ModCtrl|key.ModShift, textClipboardReady),
		cmd("entry.cut", "Entry", "Cut", "X", key.ModCtrl, clipboardReady),
		cmd("entry.paste", "Entry", "Paste", "V", key.ModCtrl, pasteReady),
		cmd("entry.paste_shortcut", "Entry", "Paste shortcut", "V", key.ModCtrl|key.ModShift, mutationReady),
		cmd("entry.duplicate", "Entry", "Duplicate", "D", key.ModCtrl, duplicateReady),
		cmd("entry.pack_zip", "Entry", "Pack to ZIP", key.NameF5, key.ModAlt, packReady),
		cmd("entry.unpack_zip", "Entry", "Unpack ZIP", key.NameF6, key.ModAlt, unpackReady),
		cmd("entry.copy_to_panel", "Entry", "Copy to panel", key.NameF5, 0, func(v *view) bool { return len(v.panels) > 1 && panelSourceReady(v, false) }),
		cmd("entry.move_to_panel", "Entry", "Move to panel", key.NameF6, 0, func(v *view) bool { return len(v.panels) > 1 && panelSourceReady(v, true) }),
		cmd("entry.copy_left", "Entry", "Copy to left", "[", 0, func(v *view) bool { return panelTransferReady(v, -1, false) }),
		cmd("entry.copy_right", "Entry", "Copy to right", "]", 0, func(v *view) bool { return panelTransferReady(v, 1, false) }),
		cmd("entry.move_left", "Entry", "Move to left", "{", 0, func(v *view) bool { return panelTransferReady(v, -1, true) }),
		cmd("entry.move_right", "Entry", "Move to right", "}", 0, func(v *view) bool { return panelTransferReady(v, 1, true) }),
		cmd("entry.recycle", "Entry", "Recycle", key.NameDeleteForward, 0, removalReady).withPresentation(func(v *view) commandPresentation {
			if v.panels[v.active].archiveFolder != "" {
				return commandPresentation{name: "Delete from archive"}
			}
			return commandPresentation{}
		}),
		cmd("entry.delete_permanently", "Entry", "Delete permanently", key.NameDeleteForward, key.ModShift, removalReady).withPresentation(func(v *view) commandPresentation {
			return commandPresentation{hidden: v.panels[v.active].archiveFolder != ""}
		}),
		cmd("entry.new_file", "Entry", "New file", "N", key.ModCtrl, createReady),
		cmd("entry.new_folder", "Entry", "New folder", "N", key.ModCtrl|key.ModShift, createReady),
		cmd("entry.context_menu", "Entry", "Context menu", "/", key.ModAlt, func(v *view) bool {
			return v.nativeItemsAvailable(v.active, v.panels[v.active].selected, false, shellMenu)
		}),

		cmd("entry.properties", "Entry", "Properties", key.NameF12, 0, func(v *view) bool {
			return v.nativeItemsAvailable(v.active, v.panels[v.active].selected, false, shellProperties)
		}),

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
		toggleSettingCommand("view.toggle_parent_entry", "View", "parent entry", settingParentEntry),
		toggleSettingCommand("view.toggle_protected_system_files", "View", "protected system files", settingProtectedSystem),
		cmd("view.toggle_metadata_column", "View", "Toggle metadata column", "", 0,
			func(v *view) bool { return v.canOfferInteraction(v.active, actionMutation) }),
		cmd("view.next_metadata_column", "View", "Next metadata column", "M", key.ModCtrl, nil),
		cmd("view.previous_metadata_column", "View", "Previous metadata column", "M", key.ModCtrl|key.ModShift, nil),
		cmd("view.sort_by_metadata", "View", "Toggle sort by metadata", key.NameF3, key.ModCtrl, folderReady),

		cmd("panel.move_left", "Panel", "Move left", key.NameLeftArrow, key.ModShift, func(v *view) bool { return v.active > 0 && panelsCanSwap(v) }),
		cmd("panel.move_right", "Panel", "Move right", key.NameRightArrow, key.ModShift, func(v *view) bool { return v.active+1 < len(v.panels) && panelsCanSwap(v) }),
		cmd("panel.increase_size", "Panel", "Increase size", "+", 0, panelSizingReady),
		cmd("panel.decrease_size", "Panel", "Decrease size", "-", 0, panelSizingReady),
		cmd("panel.reset_size", "Panel", "Reset size", "0", key.ModCtrl, panelSizingReady),

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

		cmd("programs.terminal_here", "Programs", "Terminal here", "T", key.ModCtrl, terminalReady),
		cmd("programs.terminal_here_admin", "Programs", "Terminal here (Admin)", "", 0, terminalReady),

		toggleSettingCommand("appearance.toggle_icons", "Appearance", "icons", settingIconsEnabled),
		toggleSettingCommand("appearance.toggle_auto_color", "Theme", "file colors", settingAutoColorEnabled),
		toggleSettingCommand("appearance.toggle_contrast_guard", "Theme", "contrast guard", settingContrastGuard),

		toggleSettingCommand("feedback.toggle_sound", "Feedback", "sound", settingSoundEnabled),
		toggleSettingCommand("feedback.toggle_notification_animation", "Feedback", "notification animation", settingNotificationAnimation),

		cmd("application.toggle_full_screen", "Application", "Toggle full screen", key.NameReturn, key.ModAlt, func(v *view) bool { return v.toggleFullScreen != nil }),
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
		case "panel.increase_size":
			c.bindings[1].Default = shortcut{Name: "="}
		case "entry.paste":
			c.bindings[1].Default = shortcut{Name: insertKey, Modifiers: key.ModShift}
		case "entry.context_menu":
			c.bindings[1].Default = shortcut{Name: contextMenuKey}
		case "application.toggle_full_screen":
			c.bindings[1].Default = shortcut{Name: key.NameF11}
			c.execute = func(v *view) { v.toggleFullScreen() }
		case "application.command_palette":
			c.bindings[1].Default = shortcut{Name: "P", Modifiers: key.ModCtrl | key.ModShift}
		case "selection.clear_filter_or_marks":
			c.bindings[0].Protected = true
		}
	}
	return catalogue
}

func (c command) withPresentation(presentation func(*view) commandPresentation) command {
	c.presentation = presentation
	return c
}

func (c command) label() string {
	if c.mask.name != "" {
		c.name = c.mask.name
	}
	return c.originalLabel()
}

// Shortcut editors name the global command, independently of its palette mask.
func (c command) originalLabel() string {
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
	diagnostics.Record(diagnostics.UI, 0, "Command", map[string]any{"command": c.id, "panel": v.active, "allowed": c.allowed(v)})
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
	if c.toggleSetting != "" {
		v.toggleBooleanSetting(c.toggleSetting)
	} else if c.execute != nil {
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

func writableFolder(v *view) bool {
	return folderReady(v) && v.panels[v.active].nativeFolder() && v.actionAllowed(actionMutation)
}

func mutationReady(v *view) bool { return writableFolder(v) && v.loader != nil }

func selectedMutationReady(v *view) bool {
	return mutationReady(v) && len(v.panels[v.active].selectedPaths()) > 0
}

func clipboardReady(v *view) bool {
	p := &v.panels[v.active]
	return folderReady(v) && v.loader != nil && v.actionAllowed(actionClipboard) && len(p.selectedNames()) > 0 && (p.nativeFolder() || p.location().Capabilities().Read && p.location().Capabilities().Delete)
}

func textClipboardReady(v *view) bool {
	return folderReady(v) && v.actionAllowed(actionClipboard) && len(v.panels[v.active].selectedNames()) > 0
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

func removalReady(v *view) bool {
	p := &v.panels[v.active]
	return folderReady(v) && v.loader != nil && v.actionAllowed(actionMutation) && p.location().Capabilities().Delete && len(p.selectedNames()) > 0
}

func copyReady(v *view) bool {
	p := &v.panels[v.active]
	return clipboardReady(v) || folderReady(v) && v.loader != nil && v.actionAllowed(actionClipboard) && p.archiveFolder != "" && p.location().Capabilities().Read && len(p.selectedNames()) > 0
}

func pasteReady(v *view) bool {
	p := &v.panels[v.active]
	return mutationReady(v) || folderReady(v) && v.loader != nil && v.actionAllowed(actionMutation) && p.archiveFolder != "" && p.location().Capabilities().Add
}

func createReady(v *view) bool {
	p := &v.panels[v.active]
	return writableFolder(v) || folderReady(v) && p.archiveFolder != "" && p.location().Capabilities().Add && v.actionAllowed(actionMutation)
}

func duplicateReady(v *view) bool {
	p := &v.panels[v.active]
	return selectedMutationReady(v) || createReady(v) && v.loader != nil && p.archiveFolder != "" && p.location().Capabilities().Read && len(p.selectedNames()) > 0
}
