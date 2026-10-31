package app

import (
	"image"
	"math"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/settings"
)

// Shape the complete label before clipping, preserving styled text and keeping
// surrounding icons, metadata and row surfaces stationary.
func (v *view) scrollingText(gtx layout.Context, scroll *textMarquee, content, group string, label func(layout.Context, string) layout.Dimensions) layout.Dimensions {
	if scroll == nil {
		return label(gtx, content)
	}
	full := gtx
	full.Constraints = layout.Constraints{Max: image.Pt(1_000_000, gtx.Constraints.Max.Y)}
	macro := op.Record(gtx.Ops)
	dims := label(full, content)
	drawing := macro.Stop()
	width := gtx.Constraints.Max.X
	c := settings.CatalogueDefault
	ticker := dims.Size.X > width && v.settingsValues.Get(c, group+".animation").Text == "Ticker"
	if ticker {
		macro = op.Record(gtx.Ops)
		dims = label(full, content+" • ")
		drawing = macro.Stop()
	}
	speed := textScrollSpeed * unit.Dp(v.settingsValues.Get(c, group+".speed").Number/5)
	offset := scroll.offsetWith(gtx, content, width, dims.Size.X, speed, ticker)
	defer clip.Rect{Max: image.Pt(width, dims.Size.Y)}.Push(gtx.Ops).Pop()
	defer op.Affine(f32.Affine2D{}.Offset(f32.Pt(-offset, 0))).Push(gtx.Ops).Pop()
	drawing.Add(gtx.Ops)
	if ticker {
		defer op.Offset(image.Pt(dims.Size.X, 0)).Push(gtx.Ops).Pop()
		drawing.Add(gtx.Ops)
	}
	dims.Size.X = width
	return dims
}

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
