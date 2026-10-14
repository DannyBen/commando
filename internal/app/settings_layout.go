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
	footerHeight := max(gtx.Dp(30), smallHeight+gtx.Dp(12))
	rows := len(s.fields)

	return s.surface.layout(gtx, footerHeight, gtx.Dp(v.appearance.SettingsWidth), (rows+2)*rowHeight+footerHeight, v.appearance.Colors, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, rows+3)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.band(gtx, rowHeight, v.appearance.Colors.Header, func(gtx layout.Context) layout.Dimensions {
				return v.label(gtx, s.title, v.appearance.TextSize, v.appearance.Colors.Foreground, text.Start)
			}, true)
		}))
		for i, field := range s.fields {
			d, _ := settings.CatalogueDefault.Definition(field.Key)
			path := field.IsPath()

			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return v.band(gtx, rowHeight, v.appearance.Colors.Surface, func(gtx layout.Context) layout.Dimensions {
					control := func(gtx layout.Context) layout.Dimensions {
						if s.editing && s.selected == i {
							hint := "Value"
							if path {
								hint = "Path"
							}
							return v.textEditor(&s.editor, hint).Layout(gtx)
						}
						if field.Control == "toggle" {
							return v.settingsBoolean(gtx, v.settingsValues.Get(settings.CatalogueDefault, field.Key).Bool, textHeight)
						}
						value := v.settingText(field.Key)
						if value == "" {
							value = "Choose a folder"
						}
						if i == s.selected && !s.selecting && !s.actionsFocused && v.settingsInteraction() == s {
							size := image.Pt(gtx.Constraints.Max.X, textHeight)
							paint.FillShape(gtx.Ops, v.appearance.Colors.Selected, clip.Rect{Max: size}.Op())
							return v.label(gtx, value, v.appearance.TextSize, v.appearance.Colors.SelectedName, text.Start)
						}
						return v.label(gtx, value, v.appearance.TextSize, v.appearance.Colors.Filename, text.Start)
					}
					valueWidth := min(gtx.Dp(250), gtx.Constraints.Max.X*3/5)
					if field.Control == "toggle" {
						valueWidth = v.textWidth(gtx, " ON  OFF ", v.appearance.TextSize) + gtx.Dp(8)
					}
					return v.columnLayout(gtx, valueWidth, func(gtx layout.Context) layout.Dimensions {
						return v.label(gtx, field.Label, v.appearance.TextSize, v.appearance.Colors.Foreground, text.Start)
					}, func(gtx layout.Context) layout.Dimensions {
						dims := control(gtx)
						if field.Control == "stepper-progress" {
							ratio := (v.settingsValues.Get(settings.CatalogueDefault, field.Key).Number - d.Minimum) / (*d.Maximum - d.Minimum)
							y := textHeight - gtx.Dp(1)
							width := gtx.Constraints.Max.X
							paint.FillShape(gtx.Ops, v.appearance.Colors.Border, clip.Rect{Min: image.Pt(0, y), Max: image.Pt(width, y+gtx.Dp(1))}.Op())
							paint.FillShape(gtx.Ops, v.appearance.Colors.Filename, clip.Rect{Min: image.Pt(0, y), Max: image.Pt(int(float64(width)*ratio), y+gtx.Dp(1))}.Op())
						}
						return dims
					})
				}, false)
			}))
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			selected := -1
			if s.actionsFocused && !s.selecting && !s.editing && v.settingsInteraction() == s {
				selected = s.action
			}
			if v.settingsInteraction() != s {
				gtx.Source = gtx.Source.Disabled()
			}
			return s.actions.layout(gtx, []string{"Save", "Cancel", "Reset"}, selected, rowHeight, v.appearance.Colors,
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
				if field.IsPath() {
					hint = "←/→ Choose folder · Enter Type · Ctrl+Enter Save"
				}
				if s.selecting {
					hint = "Browse panels · Ctrl+Enter Save · Esc Return"
				}
				if s.editing {
					hint = "Enter Preview · Ctrl+Enter Save · Esc Revert"
				}
				if s.actionsFocused && !s.selecting && !s.editing {
					hint = "Enter Save changes"
					if s.action == 1 {
						hint = "Enter Cancel changes"
					}
					if s.action == 2 {
						hint = "Enter Preview defaults"
					}
					hint += " · ←/→ Action · Ctrl+Enter Save"
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

func (v *view) settingsBoolean(gtx layout.Context, enabled bool, height int) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.settingsBooleanChoice(gtx, " ON ", enabled, height)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.settingsBooleanChoice(gtx, " OFF ", !enabled, height)
		}),
	)
}
func (v *view) settingsBooleanChoice(gtx layout.Context, label string, selected bool, height int) layout.Dimensions {
	width := v.textWidth(gtx, label, v.appearance.TextSize)
	fg := v.appearance.Colors.Filename
	if selected {
		paint.FillShape(gtx.Ops, v.appearance.Colors.Selected, clip.Rect{Max: image.Pt(width, height)}.Op())
		fg = v.appearance.Colors.SelectedName
	}
	return v.label(gtx, label, v.appearance.TextSize, fg, text.Start)
}
