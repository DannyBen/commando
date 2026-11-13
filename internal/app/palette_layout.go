package app

import (
	"image"
	"slices"
	"strings"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/widget/material"
)

func (v *view) layoutPalette(gtx layout.Context) layout.Dimensions {
	// Reconcile opening text before Editor.Layout can paint it in this frame.
	v.updatePaletteSearch(gtx)
	s := v.paletteInteraction()
	textHeight := v.textHeight(gtx, v.appearance.TextSize)
	smallHeight := v.textHeight(gtx, v.appearance.SmallTextSize)
	list := material.List(v.theme, &s.list)
	list.AnchorStrategy = material.Occupy
	list.Indicator.MinorWidth = 3
	list.Indicator.Color, list.Indicator.HoverColor = v.appearance.Colors.Scrollbar, v.appearance.Colors.ScrollbarHover
	list.Track.Color = v.appearance.Colors.ScrollbarTrack
	// Measure the full catalogue so filtering never changes the surface width.
	contentWidth := 0
	shortcutGap := v.textWidth(gtx, "   ", v.appearance.TextSize)
	catalogue := s.commands
	if s.settingsMode {
		catalogue = s.settingsCommands
	}
	for _, command := range catalogue {
		if command.mask.hidden {
			continue
		}
		width := v.textWidth(gtx, command.label(), v.appearance.TextSize)
		if shortcut := command.shortcut(); shortcut != "" {
			width += shortcutGap + v.textWidth(gtx, shortcut, v.appearance.SmallTextSize)
		}
		contentWidth = max(contentWidth, width)
	}
	// Row insets, the border and the list's occupied scrollbar track.
	width := contentWidth + gtx.Dp(2*max(0, v.appearance.ContentPaddingHorizontal)+2) + gtx.Dp(list.Width())
	width = max(gtx.Dp(v.appearance.OverlayMinWidth), min(gtx.Dp(v.appearance.OverlayMaxWidth), width))
	return s.surface.layout(gtx, v.locationBarHeight(gtx, textHeight), width, v.appearance.Colors, v.appearance.OverlayBackdropOpacity, func(gtx layout.Context) layout.Dimensions {
		padding := gtx.Dp(max(0, v.appearance.RowPaddingVertical))
		rowHeight := max(1, textHeight+2*padding)
		rowWidth := max(1, gtx.Constraints.Max.X-gtx.Dp(list.Width())-2*gtx.Dp(max(0, v.appearance.ContentPaddingHorizontal)))
		rows := make([]paletteRow, len(s.results))
		offsets := make([]int, len(rows)+1)
		for i, result := range s.results {
			rows[i] = v.measurePaletteRow(gtx, result.command, rowWidth, shortcutGap, textHeight, smallHeight, padding)
			offsets[i+1] = offsets[i] + rows[i].height
		}
		if !slices.Equal(s.rowOffsets, offsets) {
			s.reveal = true
		}
		s.rowOffsets = offsets
		searchHeight := textHeight + gtx.Dp(2*max(0, v.appearance.SearchPaddingVertical))
		footerHeight := v.textHeight(gtx, v.appearance.SmallTextSize) + gtx.Dp(2*max(0, v.appearance.HeaderFooterPaddingVertical))
		// Leave the panel footers visible. Tall text still gets one usable row
		// when space permits; the outer clip bounds tiny windows safely.
		available := max(0, gtx.Constraints.Max.Y-searchHeight-footerHeight-gtx.Dp(32))
		listHeight := min(max(rowHeight, offsets[len(rows)]), max(rowHeight, available*3/4))
		listHeight = min(listHeight, max(0, gtx.Constraints.Max.Y-searchHeight-footerHeight))
		if s.viewportHeight != listHeight {
			s.reveal = true
		}
		s.viewportHeight = listHeight
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return v.band(gtx, searchHeight, v.appearance.Colors.Surface, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return v.label(gtx, "> ", v.appearance.TextSize, v.appearance.Colors.EditorText, text.Start)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return v.textEditor(&v.commandSearch, "Search commands").Layout(gtx)
						}),
					)
				}, true)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = listHeight, listHeight
				if len(s.results) == 0 {
					return v.contentInset().Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = 0
							message := "No commands found"
							if s.settingsMode {
								message = "No settings found"
							}
							return v.label(gtx, message, v.appearance.TextSize, v.appearance.Colors.Foreground, text.Start)
						})
					})
				}
				if s.reveal {
					top := offsets[min(s.list.Position.First, len(rows))] + s.list.Position.Offset
					start, bottom := offsets[s.selected], offsets[s.selected+1]
					if start < top || bottom-start > listHeight {
						top = start
					} else if bottom > top+listHeight {
						top = bottom - listHeight
					}
					s.scrollToOffset(top)
					s.reveal = false
				}
				return list.Layout(gtx, len(s.results), func(gtx layout.Context, row int) layout.Dimensions {
					if s.rows[row] == nil {
						s.rows[row] = new(clickTarget)
					}
					return s.rows[row].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						size := image.Pt(gtx.Constraints.Max.X, rows[row].height)
						state := s.rows[row].state(gtx, row == s.selected)
						available := s.results[row].command.allowed(v)
						state.Enabled = state.Enabled && available
						bg, fg := v.appearance.Colors.Surface, v.appearance.Colors.Foreground
						role := contrastPrimary
						if state.selectedAppearance() {
							bg, fg = v.appearance.Colors.Selected, v.appearance.Colors.SelectedName
						}
						if !available {
							fg = v.dimmedColor(fg)
							role = contrastDimmedText
						}
						bg = state.background(bg, v.appearance.HoverStrength)
						paint.FillShape(gtx.Ops, bg, clip.Rect{Max: size}.Op())
						gtx.Constraints = layout.Exact(size)
						return v.contentInset().Layout(gtx, v.textAs(role, v.textOn(bg, func(gtx layout.Context) layout.Dimensions {
							return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.Y = 0
								return v.layoutPaletteRow(gtx, rows[row], s.results[row], fg, textHeight, smallHeight)
							})
						})))
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return v.band(gtx, footerHeight, v.appearance.Colors.Footer, func(gtx layout.Context) layout.Dimensions {
					hint := "Enter Run   Esc Close"
					if len(s.results) > 0 && strings.HasPrefix(s.results[s.selected].command.id, "settings.") {
						hint = "Enter Edit   Esc Close"
					}
					if len(s.results) > 0 && !s.results[s.selected].command.allowed(v) {
						hint = "Unavailable here   Esc Close"
					}
					if len(s.results) > 0 && v.canEditShortcuts(s.results[s.selected].command) {
						hint += "   Ctrl+E Shortcuts"
					}
					return v.label(gtx, hint, v.appearance.SmallTextSize, v.appearance.Colors.Foreground, text.Start)
				}, false)
			}),
		)
	})
}
