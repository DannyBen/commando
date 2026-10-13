package app

import (
	"image"
	"testing"
)

func TestPanelWidthIntentSurvivesAutomaticConstraints(t *testing.T) {
	tkr := windowTracker{sizing: workspaceSizing{count: 3}}
	tkr.rememberPanelWidth(902, false)
	if tkr.sizingSnapshot().baseWidth != 300 {
		t.Fatal("initial width did not exclude dividers")
	}
	tkr.panelCount(4)
	if tkr.sizingSnapshot().width() != 1203 {
		t.Fatal("adding a pane changed its intended width")
	}
	tkr.rememberPanelWidth(1000, false)
	tkr.panelCount(3)
	if tkr.sizingSnapshot().width() != 902 {
		t.Fatal("screen clamping overwrote intended panel width")
	}
	tkr.rememberPanelWidth(1202, true)
	if tkr.sizingSnapshot().baseWidth != 400 {
		t.Fatal("manual width change did not establish a new base")
	}
	tkr.rememberPanelWidth(1920, false) // Snap/maximize/restore/programmatic events.
	if tkr.sizingSnapshot().baseWidth != 400 {
		t.Fatal("non-manual changes overwrote the preferred width")
	}
}

func TestWindowGrowthAlignmentAndClamping(t *testing.T) {
	work := image.Rect(-1200, 0, 0, 1000)
	for _, tc := range []struct{ left, expected int }{{-1200, -1200}, {-1000, -1100}, {-800, -1000}} {
		window := image.Rect(tc.left, 100, tc.left+800, 700)
		alignment := windowAlignment(window, work)
		grown := resizedWindow(window, work, 1000, alignment)
		if grown.Min.X != tc.expected || grown.Dx() != 1000 || grown.Min.Y != 100 || grown.Max.Y != 700 {
			t.Fatalf("incorrect growth: %v", grown)
		}
		clamped := resizedWindow(window, work, 2000, alignment)
		if clamped.Min.X != work.Min.X || clamped.Max.X != work.Max.X {
			t.Fatal("window escaped the current monitor")
		}
		if restored := resizedWindow(clamped, work, 800, alignment); restored != window {
			t.Fatal("clamping lost the remembered relative alignment")
		}
	}
}
