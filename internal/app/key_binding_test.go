package app

import (
	"testing"

	"gioui.org/io/key"
)

func TestBindingInheritanceAssignmentAndClear(t *testing.T) {
	defaultKey := shortcut{Name: "V", Modifiers: key.ModCtrl}
	replacement := shortcut{Name: insertKey, Modifiers: key.ModShift}
	for _, tc := range []struct {
		name     string
		override bindingOverride
		want     shortcut
	}{
		{"inherited", bindingOverride{}, defaultKey},
		{"assigned", bindingOverride{Mode: bindingAssigned, Shortcut: replacement}, replacement},
		{"unassigned", bindingOverride{Mode: bindingUnassigned}, shortcut{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binding := commandBinding{Default: defaultKey}
			got, err := binding.resolve(tc.override)
			if err != nil || got != tc.want {
				t.Fatalf("resolved shortcut = %v, %v; want %v", got, err, tc.want)
			}
			got, err = binding.resolve(bindingOverride{})
			if err != nil || got != defaultKey {
				t.Fatal("reset did not restore default", got, err)
			}
		})
	}
	if _, err := (commandBinding{}).resolve(bindingOverride{Mode: bindingAssigned}); err == nil {
		t.Fatal("empty assignment was accepted instead of explicit unassignment")
	}
}

func TestProtectedBindingRetainsDefault(t *testing.T) {
	binding := commandBinding{Default: shortcut{Name: key.NameEscape}, Protected: true}
	for _, override := range []bindingOverride{
		{Mode: bindingUnassigned},
		{Mode: bindingAssigned, Shortcut: shortcut{Name: "Q", Modifiers: key.ModCtrl}},
	} {
		if _, err := binding.resolve(override); err == nil {
			t.Fatal("protected binding accepted a change")
		}
	}
	if got, err := binding.resolve(bindingOverride{}); err != nil || got != binding.Default {
		t.Fatal("protected binding did not inherit its default", got, err)
	}
}

func TestShortcutMatchingUsesExactModifiersAndPresses(t *testing.T) {
	s := shortcut{Name: "V", Modifiers: key.ModCtrl}
	for _, tc := range []struct {
		event key.Event
		want  bool
	}{
		{modified("V", key.ModCtrl), true},
		{modified("V", key.ModCtrl|key.ModShift), false},
		{pressed("V"), false},
		{modified("C", key.ModCtrl), false},
		{key.Event{Name: "V", Modifiers: key.ModCtrl, State: key.Release}, false},
	} {
		if got := s.matches(tc.event); got != tc.want {
			t.Fatalf("%v matches %v = %v; want %v", s, tc.event, got, tc.want)
		}
	}
	enter := shortcut{Name: key.NameReturn}
	if !enter.matches(pressed(key.NameEnter)) || !enter.matches(pressed(key.NameReturn)) {
		t.Fatal("main and keypad Enter did not share one binding")
	}
	if (shortcut{}).matches(key.Event{State: key.Press}) {
		t.Fatal("unassigned shortcut matched an event")
	}
}
