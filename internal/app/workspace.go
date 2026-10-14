package app

import (
	"image"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

type entry struct {
	name, size string
	file       browser.Entry
}

type panel struct {
	showProtectedSystem bool
	path                string // Last physical directory, retained while displaying drives.
	drives              bool
	items               []entry
	entries             []browser.Entry // Complete listing; items is the currently visible projection.
	rows                map[int]*fileRow
	list                widget.List
	selected            int
	selectionAnchor     image.Point
	marks               map[int]bool
	markRange           *selectionRange
	metadata            metadataColumn
	sortByMetadata      bool
	visibility          visibilityMode
	reveal              bool
	loading             bool
	navigating          bool // Retained rows must not look active while their replacement loads.
	refreshing          bool // Automatic read; ordinary browsing remains available.
	refreshNeeded       bool // Coalesced notification deferred until the listing is safe.
	err                 error
	filter              string
	prefix              string
	prefixAt            time.Time
	feedback            feedback.Channel
}

type view struct {
	settingsValues    settings.Values
	saveSettings      func(settings.Values) error // nil in previews; storage uses the shared data folder.
	appearance        Theme
	theme             *material.Theme
	panels            []panel
	closedPanels      []closedPanel
	active            int
	focus             bool
	frameSize         image.Point
	started           bool
	loader            *directoryLoader
	monitor           *directoryMonitor
	openFile          func(string) error
	interaction       interactionController
	location          widget.Editor
	nameEditor        widget.Editor
	commandSearch     widget.Editor
	filterEditor      widget.Editor
	nativeKeys        *nativeKeyInput
	suppressSpaceText bool
	feedbackPointer   bool
	cutClipboard      browser.FileClipboard
	cutLocations      []string
	cutPanelPaths     []string
	transferPending   bool
	clipboardSequence func() uint32
	ownerWindow       uintptr
	pendingNameDraft  *loadResult
	shellExecutor     func(shellRequest) shellCompletion
	shellResults      chan shellCompletion
	shellTrace        func(shellCompletion)
	sounds            interface {
		Play(sound.Name)
		Stop()
	}
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
		commandSearch:     widget.Editor{SingleLine: true},
		filterEditor:      widget.Editor{SingleLine: true},
		clipboardSequence: browser.FileClipboardSequence,
		shellResults:      make(chan shellCompletion, 1),
	}
	for i := range v.panels {
		p := &v.panels[i]
		p.rows = make(map[int]*fileRow)
		p.list.Axis = layout.Vertical
		p.metadata = metadataSize
	}
	return v
}
