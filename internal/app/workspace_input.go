package app

import (
	"gioui.org/io/key"
	"gioui.org/layout"
	"strings"
	"unicode/utf8"
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
		e, ok := gtx.Event(
			key.FocusFilter{Target: &v.focus},
			// Consume Tab alongside browsing keys to retain their event order.
			key.Filter{Name: key.NameTab, Optional: key.ModShift},
			key.Filter{Focus: &v.focus, Name: "D", Required: key.ModAlt},
			key.Filter{Focus: &v.focus, Name: "/", Required: key.ModAlt},
			key.Filter{Focus: &v.focus, Name: contextMenuKey},
			key.Filter{Focus: &v.focus, Name: "D", Required: key.ModCtrl, Optional: key.ModShift},
			key.Filter{Focus: &v.focus, Name: "`"},
			key.Filter{Focus: &v.focus, Name: "P", Required: key.ModCtrl | key.ModShift},
			key.Filter{Focus: &v.focus, Name: key.NameF2},
			key.Filter{Focus: &v.focus, Name: key.NameF3, Required: key.ModCtrl},
			key.Filter{Focus: &v.focus, Name: key.NameF5},
			key.Filter{Focus: &v.focus, Name: key.NameF6},
			key.Filter{Focus: &v.focus, Name: key.NameEscape},
			key.Filter{Focus: &v.focus, Name: key.NameUpArrow, Optional: key.ModShift},
			key.Filter{Focus: &v.focus, Name: key.NameDownArrow, Optional: key.ModShift},
			key.Filter{Focus: &v.focus, Name: insertKey},
			key.Filter{Focus: &v.focus, Name: key.NameSpace},
			key.Filter{Focus: &v.focus, Name: "A", Required: key.ModCtrl},
			key.Filter{Focus: &v.focus, Name: "C", Required: key.ModCtrl},
			key.Filter{Focus: &v.focus, Name: "X", Required: key.ModCtrl},
			key.Filter{Focus: &v.focus, Name: "V", Required: key.ModCtrl},
			key.Filter{Focus: &v.focus, Name: "M", Required: key.ModCtrl, Optional: key.ModShift},
			key.Filter{Focus: &v.focus, Name: "W", Required: key.ModCtrl, Optional: key.ModShift},
			key.Filter{Focus: &v.focus, Name: "R", Required: key.ModCtrl},
			key.Filter{Focus: &v.focus, Name: "H", Required: key.ModCtrl},
			key.Filter{Focus: &v.focus, Name: "N", Required: key.ModCtrl, Optional: key.ModShift},
			key.Filter{Focus: &v.focus, Name: key.NameLeftArrow, Optional: key.ModShift},
			key.Filter{Focus: &v.focus, Name: key.NameRightArrow, Optional: key.ModShift},
			key.Filter{Focus: &v.focus, Name: key.NameLeftArrow, Required: key.ModCtrl},
			key.Filter{Focus: &v.focus, Name: key.NameRightArrow, Required: key.ModCtrl},
			key.Filter{Focus: &v.focus, Name: key.NameLeftArrow, Required: key.ModAlt},
			key.Filter{Focus: &v.focus, Name: key.NameRightArrow, Required: key.ModAlt},
			key.Filter{Focus: &v.focus, Name: "1", Required: key.ModAlt},
			key.Filter{Focus: &v.focus, Name: "2", Required: key.ModAlt},
			key.Filter{Focus: &v.focus, Name: "3", Required: key.ModAlt},
			key.Filter{Focus: &v.focus, Name: "4", Required: key.ModAlt},
			key.Filter{Focus: &v.focus, Name: "5", Required: key.ModAlt},
			key.Filter{Focus: &v.focus, Name: "6", Required: key.ModAlt},
			key.Filter{Focus: &v.focus, Name: "7", Required: key.ModAlt},
			key.Filter{Focus: &v.focus, Name: "8", Required: key.ModAlt},
			key.Filter{Focus: &v.focus, Name: "9", Required: key.ModAlt},
			key.Filter{Focus: &v.focus, Name: key.NameHome},
			key.Filter{Focus: &v.focus, Name: key.NameEnd},
			key.Filter{Focus: &v.focus, Name: key.NamePageUp},
			key.Filter{Focus: &v.focus, Name: key.NamePageDown},
			key.Filter{Focus: &v.focus, Name: key.NameReturn},
			key.Filter{Focus: &v.focus, Name: key.NameReturn, Required: key.ModCtrl},
			key.Filter{Focus: &v.focus, Name: key.NameEnter, Required: key.ModCtrl},
			key.Filter{Focus: &v.focus, Name: key.NameEnter},
			key.Filter{Focus: &v.focus, Name: key.NameDeleteBackward},
			key.Filter{Focus: &v.focus, Name: key.NameDeleteForward},
		)
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
			if edit.Text == "/" {
				v.startFilter(gtx)
				return
			}
			if strings.HasPrefix(edit.Text, "@") && v.interaction.idle() {
				v.startPalette(gtx)
				if s := v.paletteInteraction(); s != nil {
					v.commandSearch.SetText(edit.Text)
					end := utf8.RuneCountInString(edit.Text)
					v.commandSearch.SetCaret(end, end)
					s.filter(edit.Text)
				}
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
	if v.interactionKey(gtx, ke) {
		return
	}
	p := &v.panels[v.active]
	next := p.selected
	page := max(1, p.list.Position.Count-1)
	switch ke.Name {
	case contextMenuKey:
		if ke.Modifiers == 0 {
			v.startContextMenu(gtx, v.active, p.selected, false)
		}
		return
	case "/":
		if ke.Modifiers == key.ModAlt {
			v.startContextMenu(gtx, v.active, p.selected, false)
		} else if ke.Modifiers == 0 {
			v.startFilter(gtx)
		}
		return
	case "`", "P":
		if ke.Name == "`" && ke.Modifiers == 0 || ke.Name == "P" && ke.Modifiers == key.ModCtrl|key.ModShift {
			v.startPalette(gtx)
			if s := v.paletteInteraction(); s != nil {
				s.openingBacktick = ke.Name == "`"
			}
		}
		return
	case "C", "X", "V":
		if ke.Modifiers == key.ModCtrl {
			if ke.Name == "V" {
				v.pasteFiles()
			} else {
				v.clipboardSelected(ke.Name == "X")
			}
		}
		return
	case key.NameF2:
		v.startName(gtx, nameRename)
		return
	case key.NameF3:
		if ke.Modifiers == key.ModCtrl {
			v.toggleMetadataSort()
		}
		return
	case key.NameF5, key.NameF6:
		if ke.Modifiers == 0 {
			v.startPanelTransfer(ke.Name == key.NameF6)
		}
		return
	case key.NameDeleteForward:
		if ke.Modifiers == 0 {
			v.recycleSelected()
		}
		return
	case key.NameTab:
		direction := 1
		if ke.Modifiers&key.ModShift != 0 {
			direction = -1
		}
		v.switchPanelTo(v.panelNeighbor(direction, true, -1))
		return
	case "M":
		direction := 1
		if ke.Modifiers&key.ModShift != 0 {
			direction = -1
		}
		p.metadata = p.metadata.cycle(direction)
		if p.sortByMetadata && !p.loading && v.interaction.idle() {
			p.resort()
		}
		return
	case "W":
		if ke.Modifiers == key.ModCtrl {
			v.closePanel()
		}
		if ke.Modifiers == key.ModCtrl|key.ModShift {
			v.reopenPanel()
		}
		return
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if ke.Modifiers == key.ModAlt {
			index := int(ke.Name[0] - '1')
			if index < len(v.panels) {
				v.switchPanelTo(index)
			}
		}
		return
	case "D":
		if ke.Modifiers == key.ModCtrl|key.ModShift {
			v.startDrag(gtx, v.active, p.selected, true)
		} else if ke.Modifiers == key.ModCtrl {
			v.duplicateSelected()
		} else if ke.Modifiers == key.ModAlt {
			v.startLocation(gtx)
		}
		return
	case "R":
		v.refreshPanel()
		return
	case "H":
		if ke.Modifiers == key.ModCtrl {
			v.cycleVisibility()
		}
		return
	case "N":
		kind := nameCreateFile
		if ke.Modifiers&key.ModShift != 0 {
			kind = nameCreateFolder
		}
		v.startName(gtx, kind)
		return
	case key.NameEscape:
		v.escape(gtx)
		return
	case "A":
		p.prefix = ""
		p.markAll()
		return
	case key.NameSpace:
		if p.prefix == "" {
			p.toggleMark(p.selected)
			v.suppressSpaceText = true
		}
		return
	case insertKey:
		if ke.Modifiers != 0 || p.loading {
			return
		}
		p.toggleMark(p.selected)
		next++
	case key.NameLeftArrow, key.NameRightArrow:
		direction := 1
		if ke.Name == key.NameLeftArrow {
			direction = -1
		}
		if ke.Modifiers == key.ModShift {
			v.swapPanel(direction)
		} else if ke.Modifiers == key.ModAlt {
			v.addPanel(direction)
		} else if ke.Modifiers&key.ModCtrl != 0 {
			v.openAdjacent(direction)
		} else {
			v.switchPanel(direction)
		}
		return
	case key.NameReturn, key.NameEnter:
		p.prefix = ""
		v.activateSelected()
		return
	case key.NameDeleteBackward:
		p.prefix = ""
		v.parentFolder()
		return
	case key.NameUpArrow:
		next--
	case key.NameDownArrow:
		next++
	case key.NameHome:
		next = 0
	case key.NameEnd:
		next = len(p.items) - 1
	case key.NamePageUp:
		next -= page
	case key.NamePageDown:
		next += page
	default:
		return
	}
	v.selectBrowsingRow(ke, next)
}

func (v *view) selectBrowsingRow(ke key.Event, next int) {
	p := &v.panels[v.active]
	p.prefix = ""
	if len(p.items) == 0 {
		return
	}
	next = max(0, min(next, len(p.items)-1))
	if next != p.selected && ke.Name != insertKey {
		v.playNavigationSound()
	}
	if ke.Modifiers&key.ModShift != 0 {
		p.markTo(next)
	} else {
		p.markRange = nil
	}
	p.selected = next
	p.reveal, p.err = true, nil
}
