package app

import (
	"image"
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
		rowHeight := max(1, textHeight+2*gtx.Dp(max(0, v.appearance.RowPaddingVertical)))
		searchHeight := textHeight + gtx.Dp(2*max(0, v.appearance.SearchPaddingVertical))
		footerHeight := v.textHeight(gtx, v.appearance.SmallTextSize) + gtx.Dp(2*max(0, v.appearance.HeaderFooterPaddingVertical))
		// Leave the panel footers visible. Tall text still gets one usable row
		// when space permits; the outer clip bounds tiny windows safely.
		available := max(0, gtx.Constraints.Max.Y-searchHeight-footerHeight-gtx.Dp(32))
		listHeight := min(max(1, len(s.results))*rowHeight, max(rowHeight, available*3/4))
		listHeight = min(listHeight, max(0, gtx.Constraints.Max.Y-searchHeight-footerHeight))
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
					top := s.list.Position.First*rowHeight + s.list.Position.Offset
					if y := s.selected * rowHeight; y < top {
						s.list.ScrollTo(s.selected)
					} else if bottom := (s.selected + 1) * rowHeight; bottom > top+listHeight {
						top = max(0, bottom-listHeight)
						s.list.Position.First, s.list.Position.Offset = top/rowHeight, top%rowHeight
					}
					s.reveal = false
				}
				return list.Layout(gtx, len(s.results), func(gtx layout.Context, row int) layout.Dimensions {
					if s.rows[row] == nil {
						s.rows[row] = new(clickTarget)
					}
					return s.rows[row].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						size := image.Pt(gtx.Constraints.Max.X, rowHeight)
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
								result := s.results[row]
								shortcut := result.command.shortcut()
								if shortcut == "" {
									return v.highlightedLabel(gtx, result.command.label(), result.match.Indices, v.appearance.TextSize, fg)
								}
								shortcutWidth := min(gtx.Constraints.Max.X, shortcutGap+v.textWidth(gtx, shortcut, v.appearance.SmallTextSize))
								return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										return v.highlightedLabel(gtx, result.command.label(), result.match.Indices, v.appearance.TextSize, fg)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min.X, gtx.Constraints.Max.X = shortcutWidth, shortcutWidth
										return v.label(gtx, shortcut, v.appearance.SmallTextSize, fg, text.End)
									}),
								)
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
