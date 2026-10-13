package app

import (
	"image"
	"math"
)

const minWindowWidth = 240 // Enough for one panel; a larger count can fit a constrained window.

// The base width is independent of pane models. Future per-panel presets can
// multiply this base while retaining the actual divider count and manual intent.
type workspaceSizing struct {
	count     int
	baseWidth float64 // dp, excluding the 1 dp dividers.
}

func (s *workspaceSizing) remember(width float64) {
	if s.count > 0 && width > float64(s.count-1) {
		s.baseWidth = (width - float64(s.count-1)) / float64(s.count)
	}
}

func (s workspaceSizing) width() float64 {
	return s.baseWidth*float64(s.count) + float64(max(0, s.count-1))
}

func windowAlignment(window, work image.Rectangle) float64 {
	space := work.Dx() - window.Dx()
	if space <= 0 {
		return 0.5
	}
	return max(0, min(1, float64(window.Min.X-work.Min.X)/float64(space)))
}

func resizedWindow(window, work image.Rectangle, width int, alignment float64) image.Rectangle {
	width = max(1, min(width, work.Dx()))
	x := work.Min.X + int(math.Round(max(0, min(1, alignment))*float64(work.Dx()-width)))
	return image.Rect(x, window.Min.Y, x+width, window.Max.Y)
}

func (t *windowTracker) sizingSnapshot() workspaceSizing {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sizing
}

func (t *windowTracker) rememberPanelWidth(width float64, manual bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if manual || t.sizing.baseWidth <= 0 {
		t.sizing.remember(width)
	}
}

func (t *windowTracker) panelCount(count int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	changed := t.sizing.count != count
	t.sizing.count = count
	return changed
}
