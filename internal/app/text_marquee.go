package app

import (
	"math"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

type textMarquee struct {
	content          string
	width, fullWidth int
	start            time.Time
	speed            unit.Dp
	ticker           bool
}

func (m *textMarquee) offset(gtx layout.Context, content string, width, fullWidth int) float32 {
	return m.offsetWith(gtx, content, width, fullWidth, textScrollSpeed, false)
}

func (m *textMarquee) offsetWith(gtx layout.Context, content string, width, fullWidth int, speed unit.Dp, ticker bool) float32 {
	if m.content != content || m.width != width || m.fullWidth != fullWidth || m.speed != speed || m.ticker != ticker {
		*m = textMarquee{content: content, width: width, fullWidth: fullWidth, start: gtx.Now, speed: speed, ticker: ticker}
	}
	if width <= 0 || fullWidth <= width {
		return 0
	}
	due := m.start.Add(textScrollDelay)
	if gtx.Now.Before(due) {
		gtx.Execute(op.InvalidateCmd{At: due})
		return 0
	}
	pixelsPerSecond := float64(max(1, gtx.Dp(speed)))
	if ticker {
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(time.Second / 60)})
		return float32(math.Mod(gtx.Now.Sub(due).Seconds()*pixelsPerSecond, float64(fullWidth)))
	}
	end := float32(fullWidth - width)
	travel := time.Duration(float64(fullWidth-width) * float64(time.Second) / pixelsPerSecond)
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
