package app

import (
	"time"

	"gioui.org/layout"
	"gioui.org/op"
)

type textMarquee struct {
	content          string
	width, fullWidth int
	start            time.Time
}

func (m *textMarquee) offset(gtx layout.Context, content string, width, fullWidth int) float32 {
	if m.content != content || m.width != width || m.fullWidth != fullWidth {
		*m = textMarquee{content: content, width: width, fullWidth: fullWidth, start: gtx.Now}
	}
	if width <= 0 || fullWidth <= width {
		return 0
	}
	due := m.start.Add(textScrollDelay)
	if gtx.Now.Before(due) {
		gtx.Execute(op.InvalidateCmd{At: due})
		return 0
	}
	end := float32(fullWidth - width)
	travel := time.Duration(float64(fullWidth-width) * float64(time.Second) / float64(gtx.Dp(textScrollSpeed)))
	cycle := 2*travel + textScrollEndPause + textScrollStartPause
	phase := gtx.Now.Sub(due) % cycle
	next := gtx.Now.Add(time.Second / 60)
	var offset float32
	switch {
	case phase < travel:
		offset = end * float32(phase) / float32(travel)
	case phase < travel+textScrollEndPause:
		offset = end
		next = gtx.Now.Add(travel + textScrollEndPause - phase)
	case phase < 2*travel+textScrollEndPause:
		offset = end * (1 - float32(phase-travel-textScrollEndPause)/float32(travel))
	default:
		next = gtx.Now.Add(cycle - phase)
	}
	// Pauses need just one future wake-up, rather than continuous redraws.
	gtx.Execute(op.InvalidateCmd{At: next})
	return offset
}
