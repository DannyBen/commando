package app

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"github.com/dannyben/commando/internal/settings"
)

func (v *view) layoutSettingsWidget(gtx layout.Context) layout.Dimensions {
	s := v.settingsWidget()
	textHeight := v.textHeight(gtx, v.appearance.TextSize)
	smallHeight := v.textHeight(gtx, v.appearance.SmallTextSize)
	rowHeight := max(1, textHeight+2*gtx.Dp(max(0, v.appearance.RowPaddingVertical)))
	headerHeight := rowHeight + gtx.Dp(2)
	footerHeight := smallHeight + gtx.Dp(2*max(0, v.appearance.HeaderFooterPaddingVertical))
	rows := len(s.fields)
	samples := v.settingsPreviewItems(s.preview)
	if len(samples) > 0 || isThemeWidget(s) {
		rows++
	}
	labelGap := v.textWidth(gtx, " ", v.appearance.TextSize)
	labelWidth := 0
	for _, field := range s.fields {
		labelWidth = max(labelWidth, v.textWidth(gtx, field.Label, v.appearance.TextSize))
	}
	if len(samples) > 0 || isThemeWidget(s) {
		labelWidth = max(labelWidth, v.textWidth(gtx, "Preview", v.appearance.TextSize))
	}

	editorHeight := headerHeight + (rows+1)*rowHeight + footerHeight
	return s.surface.layout(gtx, footerHeight, gtx.Dp(v.appearance.SettingsWidth), editorHeight, v.appearance.Colors, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, rows+3)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.headerBand(gtx, headerHeight, v.appearance.Colors.Header, func(gtx layout.Context) layout.Dimensions {
				return v.label(gtx, s.title, v.appearance.TextSize, v.appearance.Colors.Foreground, text.Start)
			}, true)
		}))
		for i, field := range s.fields {
			d, _ := settings.CatalogueDefault.Definition(field.Key)
			path := field.IsPath()
			focused := i == s.selected && !s.selecting && !s.actionsFocused && v.settingsInteraction() == s

			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return v.band(gtx, rowHeight, v.appearance.Colors.Surface, func(gtx layout.Context) layout.Dimensions {
					control := func(gtx layout.Context) layout.Dimensions {
						if s.capturing && s.selected == i {
							return v.label(gtx, "Press shortcut…", v.appearance.TextSize, v.appearance.Colors.SelectedName, text.Start)
						}
						if s.editing && s.selected == i {
							hint := "Value"
							if path {
								hint = "Path"
							}
							if s.completesField() && s.selected == i {
								return v.completionEditor(gtx, &s.editor, &s.completion, hint)
							}
							return v.textEditor(&s.editor, hint).Layout(gtx)
						}
						if field.Control == "toggle" {
							return v.settingsBoolean(gtx, v.widgetValues().Get(settings.CatalogueDefault, field.Key).Bool, focused, textHeight)
						}
						if field.Control == "choice" {
							return v.settingsChoices(gtx, d.Choices, v.settingText(field.Key), focused, textHeight)
						}
						value := v.settingText(field.Key)
						if value == "" {
							switch field.Control {
							case "folder":
								value = "Choose a folder"
							case "file":
								value = "Choose a program"
							default:
								value = "Enter a value"
							}
						}
						if focused {
							return v.label(gtx, value, v.appearance.TextSize, v.appearance.Colors.SelectedName, text.Start)
						}
						return v.label(gtx, value, v.appearance.TextSize, v.appearance.Colors.Filename, text.Start)
					}
					// Keep labels aligned at their measured width, reserving at least
					// a third of narrow rows for values and the inter-column gap.
					available := gtx.Constraints.Max.X
					leftWidth := min(labelWidth+labelGap, max(0, available-gtx.Dp(8))*2/3)
					valueWidth := max(0, available-leftWidth)
					return v.columnLayout(gtx, valueWidth, func(gtx layout.Context) layout.Dimensions {
						return v.label(gtx, field.Label, v.appearance.TextSize, v.appearance.Colors.Foreground, text.Start)
					}, func(gtx layout.Context) layout.Dimensions {
						valueControl := func(gtx layout.Context) layout.Dimensions {
							if focused && !s.editing && field.Control != "toggle" && field.Control != "choice" {
								paint.FillShape(gtx.Ops, v.appearance.Colors.Selected, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, textHeight)}.Op())
							}
							dims := (layout.Inset{Left: 4, Right: 4}).Layout(gtx, control)
							if field.Control == "stepper-progress" {
								ratio := (v.settingsValues.Get(settings.CatalogueDefault, field.Key).Number - d.Minimum) / (*d.Maximum - d.Minimum)
								y := textHeight - gtx.Dp(1)
								width := gtx.Constraints.Max.X
								paint.FillShape(gtx.Ops, v.appearance.Colors.Border, clip.Rect{Min: image.Pt(0, y), Max: image.Pt(width, y+gtx.Dp(1))}.Op())
								paint.FillShape(gtx.Ops, v.appearance.Colors.Filename, clip.Rect{Min: image.Pt(0, y), Max: image.Pt(int(float64(width)*ratio), y+gtx.Dp(1))}.Op())
							}
							return dims
						}
						if color, ok := v.settingSwatch(field.Key); ok {
							return colorValueControl(gtx, textHeight, color, v.appearance.Colors.Border, valueControl)
						}
						return valueControl(gtx)
					})
				}, false)
			}))
		}

		if len(samples) > 0 || isThemeWidget(s) {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return v.band(gtx, rowHeight, v.appearance.Colors.Surface, func(gtx layout.Context) layout.Dimensions {
					available := gtx.Constraints.Max.X
					leftWidth := min(labelWidth+labelGap, max(0, available-gtx.Dp(8))*2/3)
					return v.columnLayout(gtx, max(0, available-leftWidth), func(gtx layout.Context) layout.Dimensions {
						if len(samples) == 0 {
							return layout.Dimensions{}
						}
						return v.label(gtx, "Preview", v.appearance.TextSize, v.appearance.Colors.Foreground, text.Start)
					}, func(gtx layout.Context) layout.Dimensions { return samples.layout(v, gtx, rowHeight) })
				}, false)
			}))
		}

		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			selected := -1
			if s.actionsFocused && !s.selecting && !s.editing && !s.capturing && v.settingsInteraction() == s {
				selected = s.action
			}
			if v.settingsInteraction() != s || s.pending {
				gtx.Source = gtx.Source.Disabled()
			}
			return s.actions.layout(gtx, s.actionLabels(), selected, rowHeight, v.appearance.Colors,
				func(gtx layout.Context, label string, fg color.NRGBA) layout.Dimensions {
					return v.label(gtx, label, v.appearance.TextSize, fg, text.Middle)
				})
		}))
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.band(gtx, footerHeight, v.appearance.Colors.Footer, func(gtx layout.Context) layout.Dimensions {
				field := s.fields[s.selected]
				hint := "Enter Type · Ctrl+Enter Save"
				if field.Control == "stepper" || field.Control == "stepper-progress" {
					hint = "←/→ Adjust · Enter Type · Ctrl+Enter Save"
				}
				if field.Control == "toggle" {
					hint = "←/→ Choose · Enter Toggle · Ctrl+Enter Save"
				}
				if field.Control == "choice" {
					hint = "←/→ Choose · Enter Cycle · Ctrl+Enter Save"
				}
				if field.IsPath() {
					hint = "←/→ Choose folder · Enter Type · Ctrl+Enter Save"
				}
				if field.Control == "file" {
					hint = "←/→ Choose file · Enter Type · Ctrl+Enter Save"
				}
				if field.Control == "shortcut" {
					hint = "Enter Capture · Del Clear · Ctrl+Enter Save"
					if !shortcutFieldEditable(s, s.selected) {
						hint = "Protected shortcut · Ctrl+Enter Save"
					}
					if s.capturing {
						hint = "Press shortcut · Esc Cancel"
					}
				}
				if field.Key == settings.EditorCollection+".ext" {
					hint = "Space-separated extensions · * Fallback"
				}
				if field.Key == settings.EditorCollection+".args" {
					hint = "%SelectedFiles% · One launch per rule"
				}
				if s.selecting {
					hint = "Browse panels · Ctrl+Enter Save · Esc Return"
					if field.Control == "file" {
						hint = "Enter Choose file · Esc Return"
					}
				}
				if s.editing {
					hint = "Enter Preview · Ctrl+Enter Save · Esc Revert"
					if s.completesField() && s.completion.suffix() != "" {
						hint = "Tab Complete · ↑/↓ Suggest · Enter Preview"
					}
				} else if s.completesField() {
					hint = "←/→ Font · Enter Type · Ctrl+Enter Save"
				}
				if s.actionsFocused && !s.selecting && !s.editing && !s.capturing {
					hint = "Enter Save changes"
					if s.action == 1 {
						hint = "Enter Cancel changes"
					}
					if s.action == 2 {
						hint = "Enter Preview defaults"
						if s.collection != "" {
							hint = "Enter Delete rule"
						}
					}
					hint += " · ←/→ Action · Ctrl+Enter Save"
				}
				if s.deleted {
					hint = "Ctrl+Enter Save deletion · Cancel Discard"
				}
				if isThemeWidget(s) && s.selected == 0 && !s.editing && !s.actionsFocused {
					hint = "←/→ Element · Ctrl+C Copy · Ctrl+V Paste"
				}
				fg := v.appearance.Colors.Foreground
				if s.errorText != "" {
					hint = s.errorText
					fg = v.appearance.Colors.Error
				}
				return v.label(gtx, hint, v.appearance.SmallTextSize, fg, text.Start)
			}, false)
		}))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (v *view) settingsBoolean(gtx layout.Context, enabled, focused bool, height int) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.settingsChoice(gtx, " ON ", enabled, focused, height)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.settingsChoice(gtx, " OFF ", !enabled, focused, height)
		}),
	)
}
func (v *view) settingsChoices(gtx layout.Context, choices []string, value string, focused bool, height int) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(choices))
	for _, choice := range choices {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.settingsChoice(gtx, " "+choice+" ", choice == value, focused, height)
		}))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

func (v *view) settingsChoice(gtx layout.Context, label string, selected, focused bool, height int) layout.Dimensions {
	width := v.textWidth(gtx, label, v.appearance.TextSize)
	fg := v.appearance.Colors.Filename
	if focused {
		if selected {
			paint.FillShape(gtx.Ops, v.appearance.Colors.Selected, clip.Rect{Max: image.Pt(width, height)}.Op())
			fg = v.appearance.Colors.SelectedName
		}
	} else if !selected {
		fg = v.appearance.Colors.Muted
		fg.A = uint8(uint16(fg.A) * 4 / 5)
	}
	dims := v.label(gtx, label, v.appearance.TextSize, fg, text.Start)
	if selected && !focused {
		padding := v.textWidth(gtx, " ", v.appearance.TextSize)
		y := max(0, height-gtx.Dp(1))
		paint.FillShape(gtx.Ops, fg, clip.Rect{Min: image.Pt(padding, y), Max: image.Pt(max(padding, width-padding), height)}.Op())
	}
	return dims
}
