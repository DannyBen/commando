package app

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/layout"
	"gioui.org/text"
)

func (v *view) renameField(gtx layout.Context, height int, label string, content layout.Widget) layout.Dimensions {
	return v.band(gtx, height, v.appearance.Colors.Surface, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				width := min(gtx.Constraints.Max.X/3, v.textWidth(gtx, "Expression", v.appearance.TextSize)+gtx.Dp(8))
				gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
				return v.label(gtx, label, v.appearance.TextSize, v.appearance.Colors.Foreground, text.Start)
			}),
			layout.Flexed(1, content),
		)
	}, false)
}

func (v *view) layoutRenameExamples(gtx layout.Context, s *multiRenameSession, height int) layout.Dimensions {
	gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, height))
	return s.exampleTarget.layout(gtx, true, func(gtx layout.Context) layout.Dimensions {
		focused := s.focus == renameFocusInsert
		return v.widgetValueContent(gtx, &s.exampleTarget, focused, height, func(gtx layout.Context) layout.Dimensions {
			fg := v.appearance.Colors.Filename
			if s.exampleTarget.state(gtx, focused).selectedAppearance() {
				fg = v.appearance.Colors.SelectedName
			}
			return v.label(gtx, renameExamples[s.example].expression, v.appearance.TextSize, fg, text.Start)
		})
	})
}

func (v *view) layoutMultiRename(gtx layout.Context, s *multiRenameSession) layout.Dimensions {
	textHeight := v.textHeight(gtx, v.appearance.TextSize)
	smallHeight := v.textHeight(gtx, v.appearance.SmallTextSize)
	rowHeight := max(1, textHeight+2*gtx.Dp(max(0, v.appearance.RowPaddingVertical)))
	footerHeight := smallHeight + gtx.Dp(2*max(0, v.appearance.HeaderFooterPaddingVertical))
	return s.surface.layout(gtx, footerHeight, gtx.Dp(v.appearance.SettingsWidth), 4*rowHeight+gtx.Dp(2)+footerHeight, v.appearance.Colors, func(gtx layout.Context) layout.Dimensions {
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
				return v.renameField(gtx, rowHeight, "Expression", func(gtx layout.Context) layout.Dimensions {
					return v.textEditor(&s.editor, "<name>").Layout(gtx)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return v.renameField(gtx, rowHeight, "Insert", func(gtx layout.Context) layout.Dimensions {
					return v.layoutRenameExamples(gtx, s, textHeight)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				selected := s.focus - renameFocusApply
				return s.actions.layout(gtx, []string{"Apply", "Cancel"}, selected, rowHeight, v.appearance,
					func(gtx layout.Context, label string, fg, bg color.NRGBA) layout.Dimensions {
						return v.rawLabel(gtx, label, v.appearance.TextSize, v.resolvedTextOn(fg, bg), text.Middle)
					})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return v.layoutWidgetFooter(gtx, footerHeight, s.footer())
			}),
		)
	})
}

// Preview renders the resulting name in place. Only inserted characters use
// secondary text; deleted characters have no display representation.
func (v *view) multiRenameRow(gtx layout.Context, panel int, item entry, nameColor, metadataColor color.NRGBA, roles fileTextRoles, scroll *textMarquee) layout.Dimensions {
	s := v.multiRenameInteraction()
	if s == nil || s.panel != panel {
		item = v.panels[panel].displayEntry(item)
		if d := v.panels[panel].deep; d != nil && strings.Contains(strings.ToLower(item.file.Name), strings.ToLower(d.query)) {
			return v.deepRowContents(gtx, item, nameColor, metadataColor, v.panels[panel].metadata, roles, d.query, scroll)
		}
		return v.rowContents(gtx, item, nameColor, metadataColor, v.panels[panel].metadata, roles, v.panels[panel].drives, scroll)
	}
	bad := s.invalid[item.file.Name]
	if bad {
		nameColor = v.appearance.Colors.Error
	}
	spans, changed := s.spans[item.file.Name]
	if !changed {
		return v.rowContents(gtx, item, nameColor, metadataColor, metadataNone, roles, false, nil)
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
