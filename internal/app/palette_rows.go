package app

import (
	"image"
	"image/color"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"github.com/go-text/typesetting/segmenter"
)

type paletteLine struct {
	text   string
	offset int // Rune offset in the original label, for search highlights.
}

type paletteRow struct {
	label, shortcut          []paletteLine
	firstWidth, headerHeight int
	height                   int
}

// Prefer complete breadcrumbs, then ordinary Unicode line breaks. An oversized
// word can break between graphemes, without splitting a composed character.
func wrapPaletteText(value string, width int, measure func(string) int) []paletteLine {
	return wrapPaletteFirstLine(value, width, width, measure)
}

func wrapPaletteFirstLine(value string, firstWidth, width int, measure func(string) int) []paletteLine {
	if measure(value) <= firstWidth {
		return []paletteLine{{text: value}}
	}
	runes := []rune(value)
	var seg segmenter.Segmenter
	seg.Init(runes)
	var graphemes, breaks []int
	for iter := seg.GraphemeIterator(); iter.Next(); {
		part := iter.Grapheme()
		graphemes = append(graphemes, part.Offset+len(part.Text))
	}
	for iter := seg.LineIterator(); iter.Next(); {
		part := iter.Line()
		breaks = append(breaks, part.Offset+len(part.Text))
	}
	var lines []paletteLine
	for start := 0; start < len(runes); {
		lineWidth := width
		if len(lines) == 0 {
			lineWidth = firstWidth
		}
		first := sort.SearchInts(graphemes, start+1)
		count := sort.Search(len(graphemes)-first, func(i int) bool {
			return measure(string(runes[start:graphemes[first+i]])) > lineWidth
		})
		if count == 0 && len(lines) == 0 && firstWidth < width {
			lines = append(lines, paletteLine{offset: start})
			continue
		}
		end := graphemes[first+max(0, count-1)]
		if end < len(runes) {
			preferred := start
			for i := start + 1; i < end; i++ {
				if runes[i] == '›' && unicode.IsSpace(runes[i-1]) {
					preferred = i + 1
				}
			}
			if preferred == start {
				for _, boundary := range breaks {
					if boundary > end {
						break
					}
					if boundary > start {
						preferred = boundary
					}
				}
			}
			if preferred > start {
				end = preferred
			}
		}
		trimmed := end
		for trimmed > start && unicode.IsSpace(runes[trimmed-1]) {
			trimmed--
		}
		if trimmed > start {
			lines = append(lines, paletteLine{text: string(runes[start:trimmed]), offset: start})
		}
		start = end
		for start < len(runes) && unicode.IsSpace(runes[start]) {
			start++
		}
	}
	return lines
}

func (v *view) measurePaletteRow(gtx layout.Context, c command, width, gap, textHeight, smallHeight, padding int) paletteRow {
	label, shortcut := c.label(), c.shortcut()
	measure := func(s string) int { return v.textWidth(gtx, s, v.appearance.TextSize) }
	row := paletteRow{firstWidth: width, headerHeight: textHeight}
	if shortcut != "" {
		shortcutWidth := min(width, v.textWidth(gtx, shortcut, v.appearance.SmallTextSize))
		// Keep the first breadcrumb intact when wrapping binding alternatives
		// can make room for it. Subsequent command lines use the full width.
		root, _, found := strings.Cut(label, " › ")
		if found {
			rootWidth := measure(root + " ›")
			if rootWidth+gap < width {
				shortcutWidth = min(shortcutWidth, width-rootWidth-gap)
			}
		}
		row.shortcut = wrapPaletteText(shortcut, shortcutWidth, func(s string) int {
			return v.textWidth(gtx, s, v.appearance.SmallTextSize)
		})
		shortcutWidth = 0
		for _, line := range row.shortcut {
			shortcutWidth = max(shortcutWidth, v.textWidth(gtx, line.text, v.appearance.SmallTextSize))
		}
		row.firstWidth = max(0, width-gap-shortcutWidth)
		row.headerHeight = max(textHeight, max(0, (textHeight-smallHeight)/2)+len(row.shortcut)*smallHeight)
	}
	row.label = wrapPaletteFirstLine(label, row.firstWidth, width, measure)
	row.height = row.headerHeight + (len(row.label)-1)*textHeight + 2*padding
	return row
}

func (v *view) layoutPaletteRow(gtx layout.Context, row paletteRow, result commandMatch, fg color.NRGBA, textHeight, smallHeight int) layout.Dimensions {
	width := gtx.Constraints.Max.X
	nameIndices, shortcutIndices := result.match.Indices, []int(nil)
	if result.shortcutMatch {
		nameIndices, shortcutIndices = nil, nameIndices
	}
	first := gtx
	first.Constraints.Max.X = row.firstWidth
	v.layoutPaletteLines(first, row.label[:1], nameIndices, v.appearance.TextSize, fg, text.Start, textHeight)
	binding := op.Offset(image.Pt(0, max(0, (textHeight-smallHeight)/2))).Push(gtx.Ops)
	v.layoutPaletteLines(gtx, row.shortcut, shortcutIndices, v.appearance.SmallTextSize, fg, text.End, smallHeight)
	binding.Pop()
	continuation := op.Offset(image.Pt(0, row.headerHeight)).Push(gtx.Ops)
	v.layoutPaletteLines(gtx, row.label[1:], nameIndices, v.appearance.TextSize, fg, text.Start, textHeight)
	continuation.Pop()
	return layout.Dimensions{Size: image.Pt(width, row.headerHeight+(len(row.label)-1)*textHeight)}
}

func (v *view) layoutPaletteLines(gtx layout.Context, lines []paletteLine, indices []int, size unit.Sp, fg color.NRGBA, alignment text.Alignment, height int) layout.Dimensions {
	width := gtx.Constraints.Max.X
	for i, line := range lines {
		position := op.Offset(image.Pt(0, i*height)).Push(gtx.Ops)
		gtx.Constraints = layout.Exact(image.Pt(width, height))
		var matches []int
		for _, index := range indices {
			if index >= line.offset && index < line.offset+utf8.RuneCountInString(line.text) {
				matches = append(matches, index-line.offset)
			}
		}
		if alignment == text.End && len(matches) == 0 {
			v.label(gtx, line.text, size, fg, alignment)
		} else if alignment == text.End {
			layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return v.highlightedLabel(gtx, line.text, matches, size, fg)
			})
		} else {
			v.highlightedLabel(gtx, line.text, matches, size, fg)
		}
		position.Pop()
	}
	return layout.Dimensions{Size: image.Pt(width, len(lines)*height)}
}

func (s *paletteSession) pageSelection(direction int) int {
	if len(s.rowOffsets) != len(s.results)+1 {
		return s.selected + direction*max(1, s.list.Position.Count-1)
	}
	target := s.rowOffsets[s.selected] + direction*s.viewportHeight
	index := sort.Search(len(s.results), func(i int) bool { return s.rowOffsets[i] >= target })
	if direction < 0 && index < len(s.results) && s.rowOffsets[index] > target {
		index--
	}
	if direction > 0 {
		return max(s.selected+1, index)
	}
	return min(s.selected-1, index)
}

func (s *paletteSession) scrollToOffset(top int) {
	first := sort.Search(len(s.results), func(i int) bool { return s.rowOffsets[i+1] > top })
	s.list.Position.First, s.list.Position.Offset = first, top-s.rowOffsets[first]
}

// A row taller than the viewport remains readable with the keyboard before
// Page Up/Down moves to another command. Wheel scrolling remains native.
func (s *paletteSession) pageWithinRow(direction int) bool {
	if len(s.results) == 0 || len(s.rowOffsets) != len(s.results)+1 || s.viewportHeight <= 0 {
		return false
	}
	start, end := s.rowOffsets[s.selected], s.rowOffsets[s.selected+1]
	if end-start <= s.viewportHeight {
		return false
	}
	top := s.rowOffsets[s.list.Position.First] + s.list.Position.Offset
	next := max(start, min(end-s.viewportHeight, top+direction*s.viewportHeight))
	if next == top {
		return direction < 0 && s.selected == 0 || direction > 0 && s.selected == len(s.results)-1
	}
	s.scrollToOffset(next)
	s.reveal = false
	return true
}
