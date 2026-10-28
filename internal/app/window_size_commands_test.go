package app

import (
	"slices"
	"testing"

	"gioui.org/io/key"
)

func TestWindowSizeCommandsBindingsAndPalette(t *testing.T) {
	v := newPreviewView()
	var requests []windowResizeRequest
	v.resizeWindow = func(axis windowAxis, direction int) {
		requests = append(requests, windowResizeRequest{axis, direction})
	}
	send, _ := panelInputFrames(v)
	for _, tc := range []struct {
		id      string
		keys    []key.Event
		request windowResizeRequest
	}{
		{"application.increase_width", []key.Event{modified("=", key.ModCtrl), modified("+", key.ModCtrl)}, windowResizeRequest{windowWidthAxis, 1}},
		{"application.decrease_width", []key.Event{modified("-", key.ModCtrl)}, windowResizeRequest{windowWidthAxis, -1}},
		{"application.increase_height", []key.Event{modified("=", key.ModCtrl|key.ModShift), modified("+", key.ModCtrl|key.ModShift)}, windowResizeRequest{windowHeightAxis, 1}},
		{"application.decrease_height", []key.Event{modified("-", key.ModCtrl|key.ModShift)}, windowResizeRequest{windowHeightAxis, -1}},
	} {
		for _, press := range tc.keys {
			before := len(requests)
			send(press)
			if len(requests) != before+1 || requests[before] != tc.request {
				t.Fatalf("%s via %v: requests = %v", tc.id, press, requests)
			}
		}
		c := commandByID(v, tc.id)
		if c.group != "Application" || !c.allowed(v) {
			t.Fatalf("%s missing from Application", tc.id)
		}
	}
	if !slices.Equal(requests, []windowResizeRequest{
		{windowWidthAxis, 1}, {windowWidthAxis, 1}, {windowWidthAxis, -1},
		{windowHeightAxis, 1}, {windowHeightAxis, 1}, {windowHeightAxis, -1},
	}) {
		t.Fatal("shortcut request order changed", requests)
	}
	send(pressed("`"))
	send(replacePalette(v, "Application Increase window width"))
	if s := v.paletteInteraction(); len(s.results) == 0 || s.results[0].command.id != "application.increase_width" {
		t.Fatal("window width command missing from palette")
	}
	send(pressed(key.NameReturn))
	if requests[len(requests)-1] != (windowResizeRequest{windowWidthAxis, 1}) {
		t.Fatal("palette did not request window resize")
	}
}
