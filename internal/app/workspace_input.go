package app

import (
	"time"
	"unicode/utf8"

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
	v.expireFolderSizes(time.Now())
	v.pollFeedback()
	if v.interaction.focusPending {
		v.focusInteraction(gtx)
		v.interaction.focusPending = false
	}
	v.offerNameDraft(gtx)
	v.updateHelpInput(gtx)
	if !v.started {
		v.focusInteraction(gtx)
		v.started = gtx.Focused(&v.focus) || v.activeEditor() != nil && gtx.Focused(v.activeEditor())
	}
panelInput:
	for pi := range v.panels {
		p := &v.panels[pi]
		v.updateLocationBar(gtx, pi)
		pointerContext := gtx
		if v.interaction.panelsBlocked() {
			pointerContext = gtx.Disabled()
		}
		if p.emptySpace != nil {
			if context, _ := p.emptySpace.updatePointer(pointerContext); context {
				v.startBackgroundContextMenu(gtx, pi)
			}
		}
		if p.emptySpace != nil && p.emptySpace.Clicked(pointerContext) {
			if v.cancelInteraction(gtx) {
				v.dismissFeedback()
				v.focusPanel(pi, false)
				v.focusInteraction(gtx)
			}
		}
		for ri, row := range p.rows {
			pointerContext := gtx
			if v.interaction.panelsBlocked() {
				pointerContext = gtx.Disabled()
			}
			context, dragged := row.updatePointer(pointerContext)
			if dragged {
				v.startDrag(gtx, pi, v.storedRow(pi, ri))
			}
			if context {
				v.startContextMenu(gtx, pi, v.storedRow(pi, ri), true)
			}
			for {
				click, ok := row.Update(pointerContext)
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
				favoriteNaming := v.nameInteraction() != nil && v.nameInteraction().kind == nameFavorite
				if !v.cancelInteraction(gtx) {
					continue
				}
				if favoriteNaming {
					// Cancellation restored a different listing. Consume the Home
					// click rather than interpreting its row in the source folder.
					v.focusInteraction(gtx)
					break panelInput
				}
				v.panels[v.active].prefix = ""
				v.active = pi
				p.clickRow(storedRow, click.Modifiers&key.ModShift != 0, click.Modifiers&key.ModCtrl != 0)
				p.prefix = ""
				p.reveal, p.err = true, nil
				v.followSelection()
				gtx.Execute(key.FocusCmd{Tag: &v.focus})
				if click.NumClicks == 2 && click.Modifiers == 0 {
					if !v.chooseSettingsFile(gtx) {
						v.activateSelected()
					}
					return // Opening may replace the listing and its row controls.
				}
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
		// Native releases belong to the foreground session's held-key tracking;
		// they must never execute a browsing command.
		v.interactionKey(gtx, ke)
		return
	}
	defer v.beginSoundAction()()
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
	p := &v.panels[v.active]
	if ke.Name == key.NameDeleteBackward && ke.Modifiers == 0 && p.prefix != "" && gtx.Now.Before(p.prefixAt.Add(v.prefixResetDelay())) {
		_, size := utf8.DecodeLastRuneInString(p.prefix)
		p.prefix = p.prefix[:len(p.prefix)-size]
		p.prefixAt, p.markRange = gtx.Now, nil
		if p.prefix != "" {
			v.selectPrefix(false, 1)
		}
		return
	}
	for _, command := range v.shortcutCommands() {
		if !command.matches(ke) {
			continue
		}
		// Space remains a marking gesture on non-markable rows too; its text
		// event must not start a filename prefix when the action is unavailable.
		if command.id == "selection.toggle_mark" && v.panels[v.active].prefix == "" {
			v.suppressSpaceText = true
		}
		v.executeCommand(gtx, command)
		if decision := v.confirmationInteraction(); decision != nil {
			decision.holdKey(ke.Name)
		}
		if command.id == "application.command_palette" {
			if s := v.paletteInteraction(); s != nil {
				s.openingBacktick = ke.Name == "`" && ke.Modifiers == 0
			}
		}
		return
	}
	// Plain arrows cycle prefix matches only while the typing state is active.
	if ke.Modifiers == 0 && p.prefix != "" && gtx.Now.Before(p.prefixAt.Add(v.prefixResetDelay())) && (ke.Name == key.NameUpArrow || ke.Name == key.NameDownArrow) {
		direction := 1
		if ke.Name == key.NameUpArrow {
			direction = -1
		}
		before := p.selected
		p.prefixAt, p.markRange = gtx.Now, nil
		v.selectPrefix(true, direction)
		if p.selected != before {
			v.announceInteraction(eventSelectionChanged)
		}
		return
	}
	next := p.selected
	switch ke.Name {
	case key.NameUpArrow:
		next--
	case key.NameDownArrow:
		next++
	case key.NameHome, key.NameEnd:
		if ke.Modifiers != key.ModShift {
			return
		}
		next = 0
		if ke.Name == key.NameEnd {
			next = len(p.items) - 1
		}
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
		v.announceInteraction(eventSelectionChanged)
	}
	if extendMarks {
		p.markTo(next)
	} else {
		p.markRange = nil
	}
	p.selected = next
	p.reveal, p.err = true, nil
	v.followSelection()
}
