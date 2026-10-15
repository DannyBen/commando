package app

import (
	"fmt"
	"strconv"
	"strings"

	"gioui.org/io/key"
)

func parseBindingOverride(text string) (bindingOverride, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return bindingOverride{}, nil
	}
	if strings.EqualFold(text, "none") {
		return bindingOverride{Mode: bindingUnassigned}, nil
	}
	parts := strings.Split(text, "+")
	var s shortcut
	for _, part := range parts[:len(parts)-1] {
		var modifier key.Modifiers
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "ctrl":
			modifier = key.ModCtrl
		case "shift":
			modifier = key.ModShift
		case "alt":
			modifier = key.ModAlt
		case "super":
			modifier = key.ModSuper
		case "command":
			modifier = key.ModCommand
		default:
			return bindingOverride{}, fmt.Errorf("unknown modifier %q", part)
		}
		if s.Modifiers&modifier != 0 {
			return bindingOverride{}, fmt.Errorf("repeated modifier %q", part)
		}
		s.Modifiers |= modifier
	}
	name := strings.TrimSpace(parts[len(parts)-1])
	switch strings.ToLower(name) {
	case "enter":
		s.Name = key.NameReturn
	case "esc":
		s.Name = key.NameEscape
	case "backspace":
		s.Name = key.NameDeleteBackward
	case "delete":
		s.Name = key.NameDeleteForward
	case "left":
		s.Name = key.NameLeftArrow
	case "right":
		s.Name = key.NameRightArrow
	case "up":
		s.Name = key.NameUpArrow
	case "down":
		s.Name = key.NameDownArrow
	case "home":
		s.Name = key.NameHome
	case "end":
		s.Name = key.NameEnd
	case "pageup":
		s.Name = key.NamePageUp
	case "pagedown":
		s.Name = key.NamePageDown
	case "tab":
		s.Name = key.NameTab
	case "space":
		s.Name = key.NameSpace
	case "insert":
		s.Name = insertKey
	case "menu":
		s.Name = contextMenuKey
	case "plus":
		s.Name = "+"
	default:
		name = strings.ToUpper(name)
		if len(name) == 1 && name[0] >= '!' && name[0] <= '~' {
			s.Name = key.Name(name)
		} else if strings.HasPrefix(name, "F") {
			number, err := strconv.Atoi(name[1:])
			if err == nil && number >= 1 && number <= 12 && name == "F"+strconv.Itoa(number) {
				s.Name = key.Name(name)
			}
		}
	}
	if s.Name == "" {
		return bindingOverride{}, fmt.Errorf("unknown key %q", name)
	}
	return bindingOverride{Mode: bindingAssigned, Shortcut: s}, nil
}
