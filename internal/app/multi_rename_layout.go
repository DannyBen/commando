package app

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/text"
)

func (v *view) layoutMultiRename(gtx layout.Context, s *multiRenameSession) layout.Dimensions {
	textHeight := v.textHeight(gtx, v.appearance.TextSize)
	smallHeight := v.textHeight(gtx, v.appearance.SmallTextSize)
	rowHeight := max(1, textHeight+2*gtx.Dp(max(0, v.appearance.RowPaddingVertical)))
	footerHeight := smallHeight + gtx.Dp(2*max(0, v.appearance.HeaderFooterPaddingVertical))
	return s.surface.layout(gtx, footerHeight, gtx.Dp(v.appearance.SettingsWidth), 3*rowHeight+gtx.Dp(2)+footerHeight, v.appearance.Colors, func(gtx layout.Context) layout.Dimensions {
		if s.pending {
			gtx = gtx.Disabled()
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return v.headerBand(gtx, rowHeight+gtx.Dp(2), v.appearance.Colors.Header, func(gtx layout.Context) layout.Dimensions {
					return v.label(gtx, s.title(), v.appearance.TextSize, v.appearance.Colors.Foreground, text.Start)
				}, true)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return v.band(gtx, rowHeight, v.appearance.Colors.Surface, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return v.label(gtx, "Expression  ", v.appearance.TextSize, v.appearance.Colors.Foreground, text.Start)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return v.textEditor(&s.editor, "<name>").Layout(gtx)
						}),
					)
				}, false)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				selected := s.focus - 1
				return s.actions.layout(gtx, []string{"Apply", "Cancel"}, selected, rowHeight, v.appearance,
					func(gtx layout.Context, label string, fg, bg color.NRGBA) layout.Dimensions {
						return v.rawLabel(gtx, label, v.appearance.TextSize, v.resolvedTextOn(fg, bg), text.Middle)
					})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return v.band(gtx, footerHeight, v.appearance.Colors.Footer, func(gtx layout.Context) layout.Dimensions {
					message, fg := s.message, v.appearance.Colors.Error
					if message == "" {
						message, fg = "Ctrl+Enter Apply · Esc Cancel", v.appearance.Colors.Muted
					}
					if s.pending {
						message, fg = "Renaming", v.appearance.Colors.Muted
					}
					return v.label(gtx, message, v.appearance.SmallTextSize, fg, text.Start)
				}, false)
			}),
		)
	})
}

// Preview renders the resulting name in place. Only inserted characters use
// secondary text; deleted characters have no display representation.
func (v *view) multiRenameRow(gtx layout.Context, panel int, item entry, nameColor, metadataColor color.NRGBA, roles fileTextRoles) layout.Dimensions {
	s := v.multiRenameInteraction()
	if s == nil || s.panel != panel {
		return v.rowContents(gtx, item, nameColor, metadataColor, v.panels[panel].metadata, roles, v.panels[panel].drives)
	}
	bad := s.invalid[item.file.Name]
	if bad {
		nameColor = v.appearance.Colors.Error
	}
	spans, changed := s.spans[item.file.Name]
	if !changed {
		return v.rowContents(gtx, item, nameColor, metadataColor, metadataNone, roles, false)
	}
	return v.contentInset().Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			children := make([]layout.FlexChild, 0, len(spans))
			for _, span := range spans {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					fg, role := nameColor, roles.filename
					if span.added && !bad {
						fg, role = v.appearance.Colors.Muted, contrastSecondary
					}
					return v.labelStyle(span.text, v.appearance.TextSize, v.resolvedTextFor(role, fg, v.currentTextBackground()), text.Start, item.file.Directory).Layout(gtx)
				}))
			}
			return layout.Flex{Alignment: layout.Baseline}.Layout(gtx, children...)
		})
	})
}
