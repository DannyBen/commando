package app

import (
	"image"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/text"
	"gioui.org/widget"
)

// inlineCompletion owns suggestions, independently of their source and editor.
// The suffix is only a preview; accepting the full candidate uses normal undo.
type inlineCompletion struct {
	candidates []string
	query      string
	matches    []string
	selected   int
}

func completionCandidates(names []string) []string {
	names = slices.Clone(names)
	slices.SortFunc(names, func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})
	return slices.CompactFunc(names, strings.EqualFold)
}

func (c *inlineCompletion) update(editor *widget.Editor) {
	query := editor.Text()
	start, end := editor.Selection()
	if query == "" || start != end || end != utf8.RuneCountInString(query) {
		query = ""
	}
	if query != c.query {
		c.selected = 0
	}
	c.query, c.matches = query, c.matches[:0]
	if query == "" {
		return
	}
	n := utf8.RuneCountInString(query)
	for _, name := range c.candidates {
		if strings.EqualFold(name, query) {
			// A complete name stays literal, even if longer names share it.
			c.matches = c.matches[:0]
			return
		}
		runes := []rune(name)
		if len(runes) > n && strings.EqualFold(string(runes[:n]), query) {
			c.matches = append(c.matches, name)
		}
	}
	if c.selected >= len(c.matches) {
		c.selected = 0
	}
}

func (c *inlineCompletion) suffix() string {
	if len(c.matches) == 0 {
		return ""
	}
	return string([]rune(c.matches[c.selected])[utf8.RuneCountInString(c.query):])
}

func (c *inlineCompletion) cycle(direction int) bool {
	if len(c.matches) < 2 {
		return false
	}
	c.selected = (c.selected + direction + len(c.matches)) % len(c.matches)
	return true
}

func (c *inlineCompletion) accept(editor *widget.Editor) bool {
	c.update(editor)
	suffix := c.suffix()
	if suffix == "" {
		return false
	}
	name := c.matches[c.selected]
	editor.SetCaret(0, utf8.RuneCountInString(editor.Text()))
	editor.Insert(name)
	c.update(editor)
	return true
}

func (v *view) completionEditor(gtx layout.Context, editor *widget.Editor, c *inlineCompletion, hint string) layout.Dimensions {
	dims := v.textEditor(editor, hint).Layout(gtx)
	c.update(editor)
	if suffix := c.suffix(); suffix != "" {
		defer clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, dims.Size.Y)}.Push(gtx.Ops).Pop()
		fg := v.appearance.Colors.Muted
		fg.A /= 2
		macro := op.Record(gtx.Ops)
		labelGTX := gtx
		labelGTX.Constraints.Min = image.Point{}
		label := v.labelFor(contrastSecondary, labelGTX, suffix, v.appearance.TextSize, fg, text.Start)
		call := macro.Stop()
		caret := editor.CaretCoords()
		at := image.Pt(int(math.Round(float64(caret.X))), int(math.Round(float64(caret.Y)))-label.Size.Y+label.Baseline)
		defer op.Offset(at).Push(gtx.Ops).Pop()
		call.Add(gtx.Ops)
	}
	return dims
}
