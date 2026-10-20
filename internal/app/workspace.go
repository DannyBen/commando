package app

import (
	"image"
	"image/color"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/diagnostics"
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
	showParentEntry     bool
	path                string // Last physical directory, retained while displaying drives.
	archiveFolder       string // Empty for filesystem locations; "." is an archive root.
	drives              bool
	items               []entry
	entries             []browser.Entry // Complete listing; items is the currently visible projection.
	rows                map[int]*fileRow
	emptySpace          *fileRow
	locationBar         *clickTarget
	locationScroll      textMarquee
	list                widget.List
	selected            int
	selectionAnchor     image.Point
	marks               map[int]bool
	markRange           *selectionRange
	metadata            metadataColumn
	lastMetadata        metadataColumn // Last visible column, retained while metadata is None.
	sortByMetadata      bool
	sizeRatio           float64 // Relative to a normal panel; follows the pane model.
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
	pendingFeedback     chan feedback.Message
}

type view struct {
	settingsValues      settings.Values
	prepareSettingsSave func(settings.Values) func() (string, error) // Captures UI state before worker I/O; nil in previews.
	directSettingsSave  *settingsSaveRequest
	defaultDataFolder   string
	activeDataFolder    string // Fixed for this process; settings changes apply after restart.
	fontFamilies        []string
	fontFamiliesLoading bool

	textBackground    color.NRGBA
	textRole          contrastRole
	textContrast      textContrastCache
	appearance        Theme
	theme             *material.Theme
	panels            []panel
	favorites         []browser.Entry
	closedPanels      []closedPanel
	active            int
	focus             bool
	frameSize         image.Point
	helpHidden        bool
	helpHeld          map[key.Name]bool
	helpScrollID      string
	helpScroll        [2]textMarquee
	started           bool
	loader            *directoryLoader
	monitor           *directoryMonitor
	openFile          func(string) error
	fileIcons         map[fileIconKey]fileIconEntry
	fileIconPending   bool
	fileIconWanted    []fileIconKey
	fileIconClock     uint64
	fileIconAdjusted  map[fileIconAdjustment]fileIconEntry
	invalidate        func()
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
	toggleFullScreen  func()
	ownerWindow       uintptr
	pendingNameDraft  *loadResult
	shellExecutor     func(shellRequest) shellCompletion
	shellResults      chan shellCompletion
	traceOverride     *diagnostics.Mask
	traceError        string
	sounds            interface {
		Play(sound.Name, float64)
		Stop()
	}
	soundSettings soundSettings
}

func newPreviewView() *view {
	v := newView(samplePanels())
	for i := range v.panels {
		v.panels[i].showParentEntry = true
	}
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
		p.sizeRatio = panelSizeRatio(p.sizeRatio)
	}
	return v
}
