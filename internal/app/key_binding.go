package app

import (
	"errors"
	"strings"

	"gioui.org/io/key"
)

// A shortcut is a combination, not an input event: releases and repeats do not
// belong in the command catalogue.
type shortcut struct {
	Name      key.Name
	Modifiers key.Modifiers
}

type commandBinding struct {
	Default   shortcut
	Protected bool
}

type bindingMode uint8

const (
	bindingInherited bindingMode = iota
	bindingAssigned
	bindingUnassigned
)

// The zero value inherits the default. Explicitly unassigned slots remain empty
// even when the command has a default; persistence will encode them as "none".
type bindingOverride struct {
	Mode     bindingMode
	Shortcut shortcut
}

func (b commandBinding) resolve(o bindingOverride) (shortcut, error) {
	if o.Mode == bindingInherited {
		return b.Default, nil
	}
	if b.Protected {
		return shortcut{}, errors.New("shortcut is protected")
	}
	switch o.Mode {
	case bindingAssigned:
		if o.Shortcut.Name != "" {
			return o.Shortcut, nil
		}
	case bindingUnassigned:
		return shortcut{}, nil
	}
	return shortcut{}, errors.New("invalid shortcut override")
}

func (s shortcut) matches(e key.Event) bool {
	name := e.Name
	if name == key.NameEnter {
		name = key.NameReturn
	}
	return s.Name != "" && e.State == key.Press && s.Name == name && s.Modifiers == e.Modifiers
}

func (s shortcut) String() string {
	if s.Name == "" {
		return ""
	}
	name := string(s.Name)
	switch s.Name {
	case key.NameReturn, key.NameEnter:
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
	case contextMenuKey:
		name = "Menu"
	}
	var parts []string
	for _, modifier := range []struct {
		mask key.Modifiers
		name string
	}{
		{key.ModCtrl, "Ctrl"}, {key.ModAlt, "Alt"}, {key.ModShift, "Shift"},
		{key.ModSuper, "Super"}, {key.ModCommand, "Command"},
	} {
		if s.Modifiers&modifier.mask != 0 {
			parts = append(parts, modifier.name)
		}
	}
	return strings.Join(append(parts, name), "+")
}
