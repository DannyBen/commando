package app

import (
	"errors"
	"image/color"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/widget"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

type nameOperation uint8

const (
	nameRename nameOperation = iota
	nameCreateFolder
	nameCreateFile
	nameDuplicate
	namePack
	nameUnpack
	nameFavorite
)

// One inline file-list editor handles existing names and temporary draft rows.
type nameSession struct {
	kind              nameOperation
	panel, row        int
	path, oldName     string
	shortcutExtension string
	archiveFolder     string
	pending, finished bool
	focused           bool
	err               error
	duplicate         *browser.DuplicatePlan
	transferTarget    *panelTransferTarget
	data              []byte // Captured contents for a new file.
	packPaths         []string
	unpackArchive     string
	folder            bool
	favorite          *favoriteSession
	previousPosition  layout.Position
	previousReveal    bool
}

// Captured contents use the same draft and exclusive creation as an empty file.
type fileDraftRequest struct {
	name          string
	data          []byte
	packPaths     []string
	unpackArchive string
}

func (v *view) beginFileDraft(gtx layout.Context, panel int, name string, data []byte) {
	p := &v.panels[panel]
	r := &nameSession{kind: nameCreateFile, panel: panel, path: p.path,
		row: max(0, min(p.selected+1, len(p.items))), data: data,
		previousPosition: p.list.Position, previousReveal: p.reveal}
	v.beginName(gtx, r, name, false)
}

func (n *nameSession) draft() bool { return n.kind != nameRename && !n.finished }

func (v *view) startName(gtx layout.Context, kind nameOperation) {
	p := &v.panels[v.active]
	if kind == nameRename && p.nativeFolder() && len(p.selectedNames()) > 1 {
		v.startMultiRename(gtx)
		return
	}
	supported := p.nativeFolder() || kind == nameRename && p.favoriteSelected()
	if p.archiveFolder != "" {
		capabilities := p.location().Capabilities()
		switch kind {
		case nameRename:
			supported = capabilities.Rename
		case nameCreateFile, nameCreateFolder:
			supported = capabilities.Add
		}
	}
	if p.loading || !supported || !v.interaction.idle() || !v.actionAllowed(actionMutation) {
		return
	}
	r := &nameSession{kind: kind, panel: v.active, path: v.entryDirectory(v.active), archiveFolder: p.archiveFolder, row: p.selected,
		previousPosition: p.list.Position, previousReveal: p.reveal}
	if kind == nameRename {
		if !p.markable(p.selected) || p.items[p.selected].file.Name == "" {
			return
		}
		item := p.items[p.selected].file
		r.oldName = item.Name
		if item.Favorite {
			r.shortcutExtension = filepath.Ext(item.Name)
		}
		r.folder = item.Directory
		p.markRange = nil
	} else {
		if p.drives || p.path == "" {
			return
		}
		// Keep the draft next to the current row until successful creation.
		r.row = max(0, min(p.selected+1, len(p.items)))
	}
	v.beginName(gtx, r, strings.TrimSuffix(r.oldName, r.shortcutExtension), r.folder || r.shortcutExtension != "")
}

func (v *view) beginName(gtx layout.Context, r *nameSession, name string, folder bool) {
	if !v.startInteraction(r) {
		return
	}
	p := &v.panels[r.panel]
	if r.draft() {
		p.rows = make(map[int]*fileRow)
	}
	p.prefix, p.reveal = "", true
	v.nameEditor.SetText(name)
	v.nameEditor.ReadOnly = false
	selectionEnd := len(name)
	if !folder {
		if ext := filepath.Ext(name); ext != name {
			selectionEnd -= len(ext)
		}
	}
	v.nameEditor.SetCaret(0, utf8.RuneCountInString(name[:selectionEnd]))
	v.focusInteraction(gtx)
}

func (v *view) stopName(gtx layout.Context) {
	r := v.nameInteraction()
	if r == nil || r.pending {
		return
	}
	if r.draft() {
		p := &v.panels[r.panel]
		p.list.Position, p.reveal = r.previousPosition, r.previousReveal
		p.rows = make(map[int]*fileRow)
	}
	v.finishInteraction(r)
	if r.favorite != nil && !r.finished {
		v.panels[r.panel] = r.favorite.previous
	}
	if r.kind == nameDuplicate {
		v.transferPending = false
		if !r.finished {
			v.showFeedback(r.panel, feedback.Info, "Copy cancelled")
		}
	}
	v.nameEditor.ReadOnly = false
	v.focusInteraction(gtx)
}

func (v *view) updateName(gtx layout.Context) {
	r := v.nameInteraction()
	if r.finished {
		v.stopName(gtx)
		return
	}
	if gtx.Focused(&v.nameEditor) {
		r.focused = true
	} else if r.focused && !r.pending {
		// As with rename, never leave an invisible draft after scrolling away.
		v.stopName(gtx)
		return
	} else if !r.focused {
		v.focusInteraction(gtx)
	}
	for {
		e, ok := gtx.Event(
			key.Filter{Focus: &v.nameEditor, Name: key.NameEscape},
			key.Filter{Focus: &v.nameEditor, Name: key.NameF2},
		)
		if !ok {
			break
		}
		if ke, ok := e.(key.Event); ok && ke.State == key.Press && !r.pending {
			v.dismissFeedback()
			if ke.Name == key.NameF2 {
				v.nameEditor.SetCaret(0, utf8.RuneCountInString(v.nameEditor.Text()))
			} else {
				v.escape(gtx)
				return
			}
		}
	}
	for {
		e, ok := v.nameEditor.Update(gtx)
		if !ok {
			return
		}
		switch e := e.(type) {
		case widget.ChangeEvent:
			v.dismissFeedback()
			r.err = nil
		case widget.SubmitEvent:
			v.dismissFeedback()
			if r.pending {
				continue
			}
			if r.kind == nameRename && e.Text+r.shortcutExtension == r.oldName {
				v.stopName(gtx)
				return
			}
			name := e.Text
			err := browser.ValidateName(name)
			if r.kind == nameCreateFile || r.kind == nameCreateFolder {
				name, err = browser.NormalizeCreatePath(name)
			}
			if err != nil {
				r.err = err
				v.showFeedback(r.panel, feedback.Error, r.errorText(err))
				continue
			}
			if v.loader == nil {
				r.err = errors.ErrUnsupported
				v.showFeedback(r.panel, feedback.Error, r.errorText(r.err))
				continue
			}
			request := loadRequest{panel: r.panel, path: r.path, archiveFolder: r.archiveFolder, locations: v.fileOperationLocations()}
			if r.kind == nameFavorite {
				request := loadRequest{panel: r.panel, path: name, homeDirectory: r.path, favoriteWrite: r}
				if v.loader.submit(request) {
					r.err, r.pending, v.nameEditor.ReadOnly = nil, true, true
					v.operationProgress(r.panel, "Creating favorite")
				}
				continue
			}
			progress := "Renaming"
			if r.kind == nameUnpack {
				request.unpack = &unpackRequest{archive: r.unpackArchive, name: name}
				progress = "Extracting archive"
			} else if r.kind == nameRename {
				request.rename = &renameRequest{oldName: r.oldName, newName: name + r.shortcutExtension}
				if r.archiveFolder != "" {
					request.rename.archiveLocations = v.archiveOperationLocations()
				}
			} else if r.kind == nameDuplicate {
				request.duplicate = &duplicateRequest{target: r.transferTarget, plan: r.duplicate, name: e.Text, locations: v.fileOperationLocations(), owner: v.ownerWindow}
				if r.archiveFolder != "" {
					request.duplicate.archiveLocations = v.archiveOperationLocations()
				}
				progress = "Copying"
			} else {
				request.create = &createRequest{name: name, folder: r.kind == nameCreateFolder, data: r.data}
				request.create.packPaths = r.packPaths
				if r.archiveFolder != "" {
					request.create.archiveLocations = v.archiveOperationLocations()
				}
				progress = "Creating " + r.noun()
				if r.kind == namePack {
					progress = "Packing ZIP"
				}
			}
			r.err, r.pending, v.nameEditor.ReadOnly = nil, true, true
			v.operationProgress(r.panel, progress)
			if !v.loader.submit(request) {
				r.pending, v.nameEditor.ReadOnly = false, false
				v.panels[r.panel].loading = false
				v.showFeedback(r.panel, feedback.Info, "Folder is busy")
			}
		}
	}
}

func (v *view) acceptName(result loadResult) {
	r := v.nameInteraction()
	if r == nil {
		return
	}
	r.pending, r.err, v.nameEditor.ReadOnly = false, result.err, false
	if result.request.create != nil && result.request.archiveFolder == "" && filepath.Dir(result.request.create.name) != "." {
		// A failed attempt retains its stationary draft. Reconcile its partial
		// parent directories after the editor closes, without disturbing input.
		if result.err != nil && result.created.Name == "" {
			if r.panel < len(result.panelUpdates) {
				result.panelUpdates[r.panel].needed = false
			}
			v.panels[r.panel].refreshNeeded = true
		}
		v.applyTransferPanels(result, result.request.locations)
		if result.created.Name != "" && result.directory.Path != "" {
			p := &v.panels[r.panel]
			p.reconcileDirectoryListing(result.directory)
			for row, item := range p.items {
				if item.file.Name == result.created.Name {
					p.selected, p.reveal = row, true
					break
				}
			}
		}
	}
	if result.request.duplicate != nil && result.transferred.Attempted {
		refreshFailed := false
		if result.request.archiveFolder != "" {
			refreshFailed = v.applyArchivePanels(result, result.request.duplicate.archiveLocations)
		} else {
			refreshFailed = v.applyTransferPanels(result, result.request.duplicate.locations)
		}
		r.finished = true
		if result.created.Name != "" {
			v.applyDuplicate(result)
		}
		v.revealPanelTransfer(result.request.duplicate.target, result)
		v.showTransferResult(result, refreshFailed)
		return
	}
	if result.err != nil && result.created.Name == "" {
		v.showFeedback(r.panel, feedback.Error, r.errorText(result.err))
		v.panels[r.panel].reveal = true
		r.focused = false
		return
	}
	if result.request.create != nil && result.request.archiveFolder != "" {
		v.acceptArchiveCreate(result, r)
		return
	}
	success := "Renamed"
	if request := result.request.rename; request != nil {
		if result.request.archiveFolder != "" {
			failed := v.applyArchivePanels(result, request.archiveLocations)
			r.finished = true
			if failed {
				v.showFeedback(r.panel, feedback.Warning, "Renamed; refresh failed")
			} else {
				v.showFeedback(r.panel, feedback.Success, success)
			}
			return
		}
		v.applyRename(result, request.oldName, request.newName)
	} else if result.request.duplicate != nil {
		v.applyDuplicate(result)
		success = "Copied"
	} else {
		v.applyCreatedEntry(result)
		success = "File created"
		if result.created.Directory {
			success = "Folder created"
		}
		if r.kind == namePack {
			success = "ZIP created"
		}
	}
	r.finished = true
	if result.err != nil {
		message := "Copied; cleanup failed"
		if result.request.create != nil {
			message = "Created; refresh failed"
		}
		v.showFeedback(r.panel, feedback.Warning, message)
	} else {
		v.showFeedback(r.panel, feedback.Success, success)
	}
}

func (n *nameSession) noun() string {
	if n.kind == nameFavorite {
		return "favorite"
	}
	if n.kind == nameCreateFolder || n.kind == nameUnpack || n.kind == nameDuplicate && n.folder {
		return "folder"
	}
	return "file"
}

func (n *nameSession) errorText(err error) string {
	if n.kind == nameUnpack {
		return unpackError(err)
	}
	if n.kind == namePack {
		if message := fileTransferError(err); message != "Cannot transfer" {
			return message
		}
		return "Cannot create ZIP"
	}
	if n.kind == nameRename {
		return renameError(err)
	}
	if n.kind == nameDuplicate {
		return fileTransferError(err)
	}
	return createError(err, n.noun())
}

// Convert display rows to stored rows. A draft is visual only: real entries,
// marks, selection and persisted state keep their ordinary indices.
func (v *view) draftRow(panel int) int {
	if n := v.nameInteraction(); n != nil && n.panel == panel && n.draft() {
		return n.row
	}
	return -1
}

func (v *view) storedRow(panel, row int) int {
	if start, count := v.transientRows(panel); start >= 0 {
		if row >= start && row < start+count {
			return -1
		}
		if row >= start+count {
			return row - count
		}
	}
	return row
}

func (v *view) nameContents(gtx layout.Context, metadata string, metaColor color.NRGBA) layout.Dimensions {
	hint := ""
	if v.nameInteraction().draft() {
		hint = "File name"
		if v.nameInteraction().noun() == "folder" {
			hint = "Folder name"
		}
		if v.nameInteraction().kind == nameFavorite {
			hint = "Favorite name"
		}
	}
	return v.contentInset().Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			var trailing layout.Widget
			if metadata != "" && v.nameInteraction().kind != nameRename {
				trailing = func(gtx layout.Context) layout.Dimensions {
					return v.labelFor(contrastSecondary, gtx, metadata, v.appearance.TextSize, metaColor, text.End)
				}
			}
			return v.trailingLayout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					paint.FillShape(gtx.Ops, v.appearance.Colors.Surface, clip.Rect{Max: gtx.Constraints.Min}.Op())
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}, func(gtx layout.Context) layout.Dimensions {
					return v.textEditor(&v.nameEditor, hint).Layout(gtx)
				})
			}, trailing)
		})
	})
}
