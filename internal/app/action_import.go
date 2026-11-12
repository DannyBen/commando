package app

import (
	"image"
	"image/color"
	"time"

	"gioui.org/layout"
	"gioui.org/text"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/windowshell"
)

const actionImportCommand = "settings.programs.actions.import"

func (v *view) startActionImport() {
	if v.shellExecutor == nil || !v.canStartInteraction(v.active, actionMutation) {
		return
	}
	v.startCollectionWidget(settings.ActionCollection, -1, "New from context menu")
	if s := v.settingsInteraction(); s != nil && s.collection == settings.ActionCollection && s.item == -1 {
		s.importing, s.selecting = true, true
	}
}

func (v *view) actionImportMenuAvailable(index, row int, action shellAction) bool {
	if action != shellMenu || v.shellExecutor == nil || !v.canOfferInteraction(index, actionBrowse) || v.transferPending || v.mutationsBusy() {
		return false
	}
	p := &v.panels[index]
	return !p.drives && p.nativeFolder() && p.path != "" && p.markable(row)
}

func (v *view) captureActionMenu(gtx layout.Context, s *settingsSession, index, row int, mouse bool, action shellAction) {
	if !v.actionImportMenuAvailable(index, row, action) {
		return
	}
	p := &v.panels[index]
	native := &nativeSession{panel: index, request: shellRequest{
		action: shellMenu, paths: []string{p.entryPath(row)},
		options: windowshell.MenuOptions{Capture: true}, queued: time.Now(),
	}}
	if !mouse {
		native.request.options.ClientPoint = new(image.Point)
	}
	if !v.startInteraction(native) {
		return
	}
	s.errorText = ""
	v.dismissFeedback()
	v.active = index
	p.selected, p.prefix, p.reveal = row, "", true
	v.focusInteraction(gtx)
}

func (v *view) completeActionImport(completion shellCompletion) {
	s := v.settingsInteraction()
	if s == nil || !s.importing || completion.result.Cancelled {
		return
	}
	s.errorText = "Cannot import this action"
	if completion.err != nil || completion.result.Captured == nil {
		return
	}
	captured := completion.result.Captured
	row, err := settings.ImportAction(captured.Name, captured.Command, captured.Directory, captured.Extensions)
	if err != nil {
		s.errorText = err.Error()
		return
	}
	s.draft.SetItem(settings.ActionCollection, row)
	s.importing, s.selecting, s.dirty = false, false, true
	s.title, s.errorText = settingsGroupLabel("programs")+" › Actions › New", ""
	s.panel = completion.session.panel
	s.projectActionFields()
}

func (v *view) layoutActionImport(gtx layout.Context, s *settingsSession) layout.Dimensions {
	rowHeight := v.textHeight(gtx, v.appearance.TextSize) + 2*gtx.Dp(max(0, v.appearance.RowPaddingVertical))
	headerHeight := rowHeight + gtx.Dp(2)
	footerHeight := v.textHeight(gtx, v.appearance.SmallTextSize) + gtx.Dp(2*max(0, v.appearance.HeaderFooterPaddingVertical))
	return s.surface.layout(gtx, footerHeight, gtx.Dp(v.appearance.SettingsWidth), headerHeight+3*rowHeight+footerHeight, v.appearance.Colors, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.headerBand(gtx, headerHeight, v.appearance.Colors.Header, func(gtx layout.Context) layout.Dimensions {
				return v.label(gtx, s.title, v.appearance.TextSize, v.appearance.Colors.Foreground, text.Start)
			}, true)
		})}
		for _, line := range []string{"Right-click a file or folder.", "Choose the action you wish to import."} {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return v.band(gtx, rowHeight, v.appearance.Colors.Surface, func(gtx layout.Context) layout.Dimensions {
					return v.label(gtx, line, v.appearance.TextSize, v.appearance.Colors.Foreground, text.Start)
				}, false)
			}))
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if v.settingsInteraction() != s {
				gtx = gtx.Disabled()
			}
			return s.actions.layout(gtx, []string{"Cancel"}, -1, rowHeight, v.appearance, func(gtx layout.Context, label string, fg, bg color.NRGBA) layout.Dimensions {
				return v.rawLabel(gtx, label, v.appearance.TextSize, v.resolvedTextOn(fg, bg), text.Middle)
			})
		}), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.layoutWidgetFooter(gtx, footerHeight, widgetFooter{hint: "Browse panels · Alt+/ Menu · Esc Cancel", errorText: s.errorText})
		}))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}
