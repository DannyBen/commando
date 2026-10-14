package app

import (
	"strconv"
	"strings"

	"gioui.org/io/key"
)

// Commands dispatch through browsing's existing handlers. Availability is
// checked again at execution; a worker may complete while the palette is open.
type command struct {
	group        string
	name         string
	binding      key.Event
	available    func(*view) bool
	execute      func(*view) // Workspace actions can run without a keyboard binding.
	paletteQuery string      // Replace the search text while retaining the palette session.
}

// Workspace-dependent commands join their category in catalogue order. The
// palette snapshots this list while its session protects the panel context.
func (v *view) commandCatalogue() []command {
	catalogue := make([]command, 0, len(commands)+len(v.panels))
	for i, c := range commands {
		catalogue = append(catalogue, c)
		if c.group == "Panel" && (i == len(commands)-1 || commands[i+1].group != "Panel") {
			for index := range v.panels {
				number := strconv.Itoa(index + 1)
				focus := command{group: "Panel", name: "Focus " + number,
					available: func(v *view) bool { return index < len(v.panels) },
					execute:   func(v *view) { v.switchPanelTo(index) }}
				if index < 9 {
					focus.binding = key.Event{Name: key.Name(number), Modifiers: key.ModAlt, State: key.Press}
				}
				catalogue = append(catalogue, focus)
			}
		}
	}
	return catalogue
}

func cmd(group, name string, nameKey key.Name, modifiers key.Modifiers, available func(*view) bool) command {
	return command{group: group, name: name, binding: key.Event{Name: nameKey, Modifiers: modifiers, State: key.Press}, available: available}
}

// Catalogue order is the browsing order. Searches rank the complete label.
var commands = []command{
	cmd("File", "Edit", key.NameF4, 0, editorReady),
	cmd("Entry", "Open", key.NameReturn, 0, selectedReady),
	cmd("Entry", "Rename", key.NameF2, 0, func(v *view) bool {
		return selectedReady(v) && v.panels[v.active].markable(v.panels[v.active].selected) && v.actionAllowed(actionMutation)
	}),
	cmd("Entry", "Copy", "C", key.ModCtrl, clipboardReady),
	cmd("Entry", "Cut", "X", key.ModCtrl, clipboardReady),
	cmd("Entry", "Paste", "V", key.ModCtrl, mutationReady),
	cmd("Entry", "Duplicate", "D", key.ModCtrl, selectedMutationReady),
	cmd("Entry", "Copy to panel", key.NameF5, 0, func(v *view) bool { return len(v.panels) > 1 && selectedMutationReady(v) }),
	cmd("Entry", "Move to panel", key.NameF6, 0, func(v *view) bool { return len(v.panels) > 1 && selectedMutationReady(v) }),
	cmd("Entry", "Recycle", key.NameDeleteForward, 0, selectedMutationReady),
	cmd("Entry", "New file", "N", key.ModCtrl, writableFolder),
	cmd("Entry", "New folder", "N", key.ModCtrl|key.ModShift, writableFolder),
	cmd("Entry", "Context menu", "/", key.ModAlt, func(v *view) bool { return v.nativeItemsAvailable(v.active, v.panels[v.active].selected, false) }),

	cmd("Entry", "Arm drag", "D", key.ModCtrl|key.ModShift, func(v *view) bool { return v.dragAvailable(v.active, v.panels[v.active].selected, true) }),

	cmd("Navigation", "Parent folder", key.NameDeleteBackward, 0, func(v *view) bool { return folderReady(v) && v.loader != nil }),
	cmd("Navigation", "Edit location", "D", key.ModAlt, nil),
	cmd("Navigation", "Next panel", key.NameTab, 0, nil),
	cmd("Navigation", "Previous panel", key.NameTab, key.ModShift, nil),
	cmd("Navigation", "Panel left", key.NameLeftArrow, 0, func(v *view) bool { return v.active > 0 }),
	cmd("Navigation", "Panel right", key.NameRightArrow, 0, func(v *view) bool { return v.active+1 < len(v.panels) }),
	cmd("Navigation", "Open in left panel", key.NameLeftArrow, key.ModCtrl, func(v *view) bool { return adjacentReady(v, -1) }),
	cmd("Navigation", "Open in right panel", key.NameRightArrow, key.ModCtrl, func(v *view) bool { return adjacentReady(v, 1) }),
	cmd("Navigation", "First entry", key.NameHome, 0, nil),
	cmd("Navigation", "Last entry", key.NameEnd, 0, nil),
	cmd("Navigation", "Page up", key.NamePageUp, 0, nil),
	cmd("Navigation", "Page down", key.NamePageDown, 0, nil),

	cmd("View", "Refresh", "R", key.ModCtrl, func(v *view) bool { return panelReady(v) && v.loader != nil }),
	cmd("View", "Filter files", "/", 0, folderReady),
	cmd("View", "Hidden files", "H", key.ModCtrl, folderReady),
	cmd("View", "Next metadata column", "M", key.ModCtrl, nil),
	cmd("View", "Previous metadata column", "M", key.ModCtrl|key.ModShift, nil),
	cmd("View", "Sort by metadata", key.NameF3, key.ModCtrl, folderReady),

	cmd("Panel", "Move left", key.NameLeftArrow, key.ModShift, func(v *view) bool { return v.active > 0 && panelsCanSwap(v) }),
	cmd("Panel", "Move right", key.NameRightArrow, key.ModShift, func(v *view) bool { return v.active+1 < len(v.panels) && panelsCanSwap(v) }),

	cmd("Panel", "Add left", key.NameLeftArrow, key.ModAlt, panelsCanChange),
	cmd("Panel", "Add right", key.NameRightArrow, key.ModAlt, panelsCanChange),
	cmd("Panel", "Close", "W", key.ModCtrl, func(v *view) bool { return len(v.panels) > 1 && panelsCanChange(v) }),
	cmd("Panel", "Reopen closed", "W", key.ModCtrl|key.ModShift, func(v *view) bool { return len(v.closedPanels) > 0 && panelsCanChange(v) }),

	cmd("Selection", "Toggle mark", key.NameSpace, 0, func(v *view) bool {
		return v.panels[v.active].markable(v.panels[v.active].selected) && v.panels[v.active].prefix == ""
	}),
	cmd("Selection", "Toggle mark and advance", insertKey, 0, func(v *view) bool { return panelReady(v) && v.panels[v.active].markable(v.panels[v.active].selected) }),
	cmd("Selection", "Mark all", "A", key.ModCtrl, nil),
	cmd("Selection", "Clear filter or marks", key.NameEscape, 0, nil),

	cmd("Application", "Command palette", "`", 0, nil),
	{group: "Application", name: "Settings", binding: key.Event{Name: "@", State: key.Press}, paletteQuery: "@"},
}

func (c command) label() string {
	if c.group == "" {
		return c.name
	}
	return c.group + " › " + c.name
}

func (c command) allowed(v *view) bool { return c.available == nil || c.available(v) }

func (c command) shortcut() string {
	if c.binding.Name == "`" {
		return "` / Ctrl+Shift+P"
	}
	name := string(c.binding.Name)
	switch c.binding.Name {
	case key.NameReturn:
		name = "Enter"
	case key.NameDeleteBackward:
		name = "Backspace"
	case key.NameDeleteForward:
		name = "Delete"
	case key.NameLeftArrow:
		name = "Left"
	case key.NameRightArrow:
		name = "Right"
	case key.NameHome:
		name = "Home"
	case key.NameEnd:
		name = "End"
	case key.NamePageUp:
		name = "PageUp"
	case key.NamePageDown:
		name = "PageDown"
	case key.NameTab:
		name = "Tab"
	case key.NameSpace:
		name = "Space"
	case key.NameEscape:
		name = "Esc"
	}
	var parts []string
	if c.binding.Modifiers&key.ModCtrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if c.binding.Modifiers&key.ModAlt != 0 {
		parts = append(parts, "Alt")
	}
	if c.binding.Modifiers&key.ModShift != 0 {
		parts = append(parts, "Shift")
	}
	return strings.Join(append(parts, name), "+")
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
	return selectedReady(v) && v.panels[v.active].items[v.panels[v.active].selected].file.Directory &&
		v.loader != nil && (target >= 0 && target < len(v.panels) && !v.panels[target].loading || (target < 0 || target >= len(v.panels)) && panelsCanChange(v))
}
