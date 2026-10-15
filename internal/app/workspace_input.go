package app

import (
	"gioui.org/io/key"
	"gioui.org/layout"
)

func (v *view) update(gtx layout.Context) {
	defer v.updateSettingsSelection()
	defer v.updateAutoRefresh(gtx)
	v.updateCutClipboard(gtx)
	v.updateFeedbackInput(gtx)
	defer v.dismissUnhandledFeedbackKeys(gtx)
	v.pollNative(gtx)
	v.pollLoads()
	if v.interaction.focusPending {
		v.focusInteraction(gtx)
		v.interaction.focusPending = false
	}
	v.offerNameDraft(gtx)
	if !v.started {
		v.focusInteraction(gtx)
		v.started = gtx.Focused(&v.focus) || v.activeEditor() != nil && gtx.Focused(v.activeEditor())
	}
	for pi := range v.panels {
		p := &v.panels[pi]
		for ri, row := range p.rows {
			context, dragged := row.updatePointer(gtx)
			if dragged {
				v.startDrag(gtx, pi, v.storedRow(pi, ri), false)
			}
			if context {
				v.startContextMenu(gtx, pi, v.storedRow(pi, ri), true)
			}
			for {
				click, ok := row.Update(gtx)
				if !ok {
					break
				}
				if v.interaction.panelsBlocked() {
					continue
				}
				v.dismissFeedback()
				if v.inlineActionRow(pi, ri) {
					v.clickInlineAction(gtx, pi, ri)
					continue
				}
				storedRow := v.storedRow(pi, ri)
				if storedRow < 0 {
					continue
				}
				if !v.cancelInteraction(gtx) {
					continue
				}
				v.panels[v.active].prefix = ""
				v.active = pi
				p.clickRow(storedRow, click.Modifiers&key.ModShift != 0, click.Modifiers&key.ModCtrl != 0)
				p.prefix = ""
				p.reveal, p.err = true, nil
				gtx.Execute(key.FocusCmd{Tag: &v.focus})
			}
		}
	}
	v.expirePrefix(gtx)
	// Windows Insert uses the native bridge; route Shift+Insert to editor paste.
	for _, ke := range v.nativeKeys.drain() {
		if ke.State == key.Press {
			v.dismissFeedback()
		}
		v.browseKey(gtx, ke)
	}
	if v.updateInteraction(gtx) {
		return
	}
	v.offerNameDraft(gtx)
	for {
		e, ok := gtx.Event(v.browsingKeyFilters()...)
		if !ok {
			break
		}
		if edit, ok := e.(key.EditEvent); ok {
			v.dismissFeedback()
			if !v.interaction.idle() && !v.settingsSelecting() {
				continue
			}
			if v.suppressSpaceText {
				v.suppressSpaceText = false
				if edit.Text == " " {
					continue
				}
			}
			// Consume the invocation text before moving focus to the editor.
			// Opening on its key event would send this slash to the new field.
			if v.dispatchTextShortcut(gtx, edit.Text) {
				return
			}
			v.typeToSelect(edit.Text, gtx.Now)
			continue
		}
		ke, ok := e.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		v.browseKey(gtx, ke)
		if v.activeEditor() != nil {
			return
		}
	}
	v.expirePrefix(gtx)
}

func (v *view) browseKey(gtx layout.Context, ke key.Event) {
	if ke.State != key.Press {
		return
	}
	v.dismissFeedback()
	v.suppressSpaceText = false
	if v.settingsSelecting() && (ke.Name == key.NameReturn || ke.Name == key.NameEnter) && ke.Modifiers == key.ModCtrl {
		v.finishSettingsWidget(gtx, true)
		return
	}
	if (ke.Name == key.NameReturn || ke.Name == key.NameEnter) && ke.Modifiers == 0 && v.chooseSettingsFile(gtx) {
		return
	}
	if v.interactionKey(gtx, ke) {
		return
	}
	for _, command := range v.commandCatalogue() {
		if !command.matches(ke) {
			continue
		}
		// Space remains a marking gesture on non-markable rows too; its text
		// event must not start a filename prefix when the action is unavailable.
		if command.id == "selection.toggle_mark" && v.panels[v.active].prefix == "" {
			v.suppressSpaceText = true
		}
		v.executeCommand(gtx, command)
		if command.id == "application.command_palette" {
			if s := v.paletteInteraction(); s != nil {
				s.openingBacktick = ke.Name == "`" && ke.Modifiers == 0
			}
		}
		return
	}
	// Row movement and range marking remain fixed browsing interactions.
	p := &v.panels[v.active]
	next := p.selected
	switch ke.Name {
	case key.NameUpArrow:
		next--
	case key.NameDownArrow:
		next++
	default:
		return
	}
	v.selectBrowsingRow(next, ke.Modifiers&key.ModShift != 0, true)
}

func (v *view) selectBrowsingRow(next int, extendMarks, navigationSound bool) {
	p := &v.panels[v.active]
	p.prefix = ""
	if len(p.items) == 0 {
		return
	}
	next = max(0, min(next, len(p.items)-1))
	if next != p.selected && navigationSound {
		v.playNavigationSound()
	}
	if extendMarks {
		p.markTo(next)
	} else {
		p.markRange = nil
	}
	p.selected = next
	p.reveal, p.err = true, nil
}
