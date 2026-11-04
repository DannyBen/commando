package app

import (
	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/feedback"
)

// A file target is a second cursor. Ordinary selections and batch marks stay put.
type fileTargeting struct {
	sourcePanel int
	sourcePath  string
	label       string
	rows        map[int]int
	choose      func(int, int)
}

func (v *view) chooseFileTarget(label string, choose func(int, int)) {
	source := v.active
	f := &fileTargeting{sourcePanel: source, sourcePath: v.panels[source].nativeFilePath(v.panels[source].selected), label: label,
		rows: map[int]int{}, choose: choose}
	target := source + 1
	if target >= len(v.panels) {
		target = max(0, source-1)
	}
	v.offerFileTarget(f, target)
}

func (v *view) validFileTarget(f *fileTargeting, panel, row int) bool {
	path := v.panels[panel].nativeFilePath(row)
	return path != "" && !samePath(path, f.sourcePath)
}

func (v *view) findFileTarget(f *fileTargeting, panel, row, step int) int {
	for ; row >= 0 && row < len(v.panels[panel].items); row += step {
		if v.validFileTarget(f, panel, row) {
			return row
		}
	}
	return -1
}

func (v *view) offerFileTarget(f *fileTargeting, panel int) {
	p := &v.panels[panel]
	row, known := f.rows[panel]
	if !known {
		row = p.selected
	}
	if !v.validFileTarget(f, panel, row) {
		start := max(0, row)
		row = v.findFileTarget(f, panel, start, 1)
		if row < 0 {
			row = v.findFileTarget(f, panel, min(start-1, len(p.items)-1), -1)
		}
	}
	v.active = panel
	v.startInlineActions(panel, max(0, row+1), "", "", []string{f.label}, func(int) {
		f.choose(panel, f.rows[panel])
	}, func() { v.active = f.sourcePanel })
	a := v.inlineInteraction()
	a.fileTarget, a.subjectRow, f.rows[panel] = f, row, row
	a.canChoose = func() bool {
		if !v.validFileTarget(f, panel, a.subjectRow) {
			v.showFeedback(panel, feedback.Info, "Choose another file")
			return false
		}
		return true
	}
	a.navigate = func(direction int, cycle bool) {
		target := v.panelNeighbor(direction, cycle, -1)
		if target != panel {
			v.stopInlineActions(true)
			v.offerFileTarget(f, target)
		}
	}
}

func (v *view) moveFileTarget(name key.Name) bool {
	a := v.inlineInteraction()
	row, step := a.subjectRow, 1
	p := &v.panels[a.panel]
	switch name {
	case key.NameUpArrow:
		row, step = row-1, -1
	case key.NameDownArrow:
		row++
	case key.NameHome:
		row = 0
	case key.NameEnd:
		row, step = len(p.items)-1, -1
	case key.NamePageUp:
		row, step = max(0, row-max(1, p.list.Position.Count-2)), -1
	case key.NamePageDown:
		row = min(len(p.items)-1, row+max(1, p.list.Position.Count-2))
	default:
		return false
	}
	row = v.findFileTarget(a.fileTarget, a.panel, row, step)
	if row < 0 && name == key.NamePageUp {
		row = v.findFileTarget(a.fileTarget, a.panel, 0, 1)
	} else if row < 0 && name == key.NamePageDown {
		row = v.findFileTarget(a.fileTarget, a.panel, len(p.items)-1, -1)
	}
	if row >= 0 && row != a.subjectRow {
		v.retargetFile(a.panel, row)
	}
	return true
}

// File clicks choose a target; only the action row or Enter executes it.
func (v *view) retargetFile(panel, row int) {
	defer v.beginSoundAction()()
	previousHandoff := v.soundAction.handoff
	v.soundAction.handoff = true
	defer func() { v.soundAction.handoff = previousHandoff }()
	a := v.inlineInteraction()
	f := a.fileTarget
	if !v.validFileTarget(f, panel, row) || panel == a.panel && row == a.subjectRow {
		return
	}
	f.rows[panel] = row
	if panel != a.panel {
		v.stopInlineActions(true)
		v.offerFileTarget(f, panel)
	} else {
		a.subjectRow, a.row = row, row+1
		v.panels[panel].rows = make(map[int]*fileRow)
	}
	v.announceInteraction(eventSelectionChanged)
}
