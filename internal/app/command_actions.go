package app

import "gioui.org/layout"

func (v *view) executeBuiltinCommand(gtx layout.Context, id string) {
	p := &v.panels[v.active]
	switch id {
	case "entry.calculate_size":
		v.startCalculateSize(gtx)
	case "location.favorite":
		v.startFavorite(gtx)
	case "file.edit":
		v.editSelectedFiles()
	case "programs.terminal_here", "programs.terminal_here_admin":
		v.openTerminal(id == "programs.terminal_here_admin")
	case "entry.open":
		v.activateSelected()
	case "entry.rename":
		v.startName(gtx, nameRename)
	case "entry.copy":
		v.clipboardSelected(false)
	case "entry.copy_paths":
		v.copySelectedPaths(gtx)
	case "entry.cut":
		v.clipboardSelected(true)
	case "entry.paste":
		v.pasteFiles()
	case "entry.paste_shortcut":
		v.pasteShortcuts()
	case "entry.duplicate":
		v.duplicateSelected()
	case "entry.pack_zip":
		v.startPack()
	case "entry.unpack_zip":
		v.startUnpack()
	case "entry.copy_to_panel", "entry.move_to_panel":
		v.startPanelTransfer(id == "entry.move_to_panel")
	case "entry.copy_left", "entry.move_left":
		v.transferToNeighbor(-1, id == "entry.move_left")
	case "entry.copy_right", "entry.move_right":
		v.transferToNeighbor(1, id == "entry.move_right")
	case "entry.recycle":
		v.removeSelected(gtx, false)
	case "entry.delete_permanently":
		v.removeSelected(gtx, true)
	case "entry.new_file":
		v.startName(gtx, nameCreateFile)
	case "entry.new_folder":
		v.startName(gtx, nameCreateFolder)
	case "entry.context_menu":
		v.startContextMenu(gtx, v.active, p.selected, false)
	case "entry.properties":
		v.startItemShellAction(gtx, v.active, p.selected, false, shellProperties)
	case "navigation.parent_folder":
		v.parentFolder()
	case "navigation.drives":
		v.showDrives()
	case "navigation.edit_location":
		v.startLocation(gtx)
	case "navigation.next_panel":
		v.switchPanelTo(v.panelNeighbor(1, true, -1))
	case "navigation.previous_panel":
		v.switchPanelTo(v.panelNeighbor(-1, true, -1))
	case "navigation.panel_left":
		v.switchPanel(-1)
	case "navigation.panel_right":
		v.switchPanel(1)
	case "navigation.open_in_left_panel":
		v.openAdjacent(-1)
	case "navigation.open_in_right_panel":
		v.openAdjacent(1)
	case "navigation.first_entry":
		v.selectBrowsingRow(0, false, true)
	case "navigation.last_entry":
		v.selectBrowsingRow(len(p.items)-1, false, true)
	case "navigation.page_up":
		v.selectBrowsingRow(p.selected-max(1, p.list.Position.Count-1), false, true)
	case "navigation.page_down":
		v.selectBrowsingRow(p.selected+max(1, p.list.Position.Count-1), false, true)
	case "view.refresh":
		v.refreshPanel()
	case "view.filter_files":
		v.startFilter(gtx)
	case "view.hidden_files":
		v.cycleVisibility()
	case "view.next_metadata_column", "view.previous_metadata_column":
		direction := 1
		if id == "view.previous_metadata_column" {
			direction = -1
		}
		p.setMetadataColumn(v.nextMetadataColumn(p.metadata, direction))
		if p.sortByMetadata && !p.loading && v.interaction.idle() {
			p.resort()
		}
	case "view.sort_by_metadata":
		v.toggleMetadataSort()
	case "view.toggle_metadata_column":
		v.toggleMetadataColumn()
	case "panel.follow_me":
		v.followMe = !v.followMe
		if v.followMe {
			v.announceInteraction(eventFollowEnabled)
		} else {
			v.announceInteraction(eventFollowDisabled)
		}
	case "panel.move_left":
		v.swapPanel(-1)
	case "panel.increase_size":
		v.resizePanel(1)
	case "panel.decrease_size":
		v.resizePanel(-1)
	case "panel.reset_size":
		v.resizePanel(0)
	case "panel.move_right":
		v.swapPanel(1)
	case "panel.add_left":
		v.addPanel(-1)
	case "panel.add_right":
		v.addPanel(1)
	case "panel.close":
		v.closePanel()
	case "panel.reopen_closed":
		v.reopenPanel()
	case "selection.toggle_mark":
		p.toggleMark(p.selected)
	case "selection.toggle_mark_and_advance":
		p.toggleMark(p.selected)
		v.selectBrowsingRow(p.selected+1, false, false)
	case "selection.mark_all":
		p.prefix = ""
		p.markAll()
	case "selection.clear_filter_or_marks":
		v.escape(gtx)
	case "application.command_palette":
		v.startPalette(gtx)
	}
}
