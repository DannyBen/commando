package app

import (
	"time"

	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

type entry struct {
	name, size string
	file       browser.Entry
}

type panel struct {
	path      string // Last physical directory, retained while displaying drives.
	drives    bool
	items     []entry
	rows      map[int]*widget.Clickable
	list      widget.List
	selected  int
	marks     map[int]bool
	markRange *selectionRange
	metadata  metadataColumn
	reveal    bool
	loading   bool
	recycling bool
	err       error
	prefix    string
	prefixAt  time.Time
	feedback  feedback.Channel
}

type view struct {
	appearance          Theme
	theme               *material.Theme
	panels              []panel
	active              int
	focus               bool
	started             bool
	loader              *directoryLoader
	openFile            func(string) error
	editing             bool
	location            widget.Editor
	naming              *nameSession
	nameEditor          widget.Editor
	nativeKeys          *nativeKeyInput
	suppressSpaceText   bool
	feedbackPointer     bool
	cutClipboard        browser.FileClipboard
	cutLocations        []string
	cutPanelPaths       []string
	transferPending     bool
	clipboardSequence   func() uint32
	actions             *inlineActionSession
	pendingFileConflict *loadResult
	pendingDuplicate    *loadResult
}

func newPreviewView() *view {
	v := newView(samplePanels())
	for _, row := range []int{1, 3, 4} {
		v.panels[0].toggleMark(row)
	}
	for _, row := range []int{2, 5} {
		v.panels[1].toggleMark(row)
	}
	return v
}

func newView(panels []panel) *view {
	appearance := defaultTheme
	v := &view{appearance: appearance, theme: appearance.materialTheme(), panels: panels,
		location:          widget.Editor{SingleLine: true, Submit: true},
		nameEditor:        widget.Editor{SingleLine: true, Submit: true},
		clipboardSequence: browser.FileClipboardSequence,
	}
	for i := range v.panels {
		p := &v.panels[i]
		p.rows = make(map[int]*widget.Clickable)
		p.list.Axis = layout.Vertical
		p.metadata = metadataSize
	}
	return v
}
