package app

import (
	"context"
	"errors"
	"io/fs"
	"strconv"
	"time"

	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

type sizeSession struct {
	panel   int
	paths   []string
	ctx     context.Context
	cancel  context.CancelFunc
	folders map[string]bool
}

func (s *sizeSession) interactionPanel() int { return s.panel }
func (*sizeSession) holdsOperations() bool   { return true }
func (*sizeSession) blocksPanels() bool      { return true }

func calculateSizeReady(v *view) bool {
	p := &v.panels[v.active]
	return v.loader != nil && p.nativeFolder() && !p.refreshing &&
		v.canOfferInteraction(v.active, actionMutation) && len(p.selectedPaths()) > 0
}

func (v *view) startCalculateSize(gtx layout.Context) {
	if !v.interaction.idle() || !calculateSizeReady(v) {
		return
	}
	p := &v.panels[v.active]
	ctx, cancel := context.WithCancel(context.Background())
	s := &sizeSession{panel: v.active, paths: p.selectedPaths(), ctx: ctx, cancel: cancel}
	s.folders = make(map[string]bool)
	for row, item := range p.items {
		if item.file.Directory && p.markable(row) && (len(p.marks) > 0 && p.marks[row] || len(p.marks) == 0 && row == p.selected) {
			s.folders[p.entryPath(row)] = true
		}
	}
	if !v.startInteraction(s) {
		cancel()
		return
	}
	if !v.loader.submit(loadRequest{panel: s.panel, path: p.path, size: s, action: actionBrowse}) {
		v.interaction.finish(s)
		cancel()
		return
	}
	p.prefix = ""
	v.operationProgress(s.panel, "Calculating size…")
	v.focusInteraction(gtx)
}

func (v *view) acceptCalculateSize(result loadResult) {
	s := result.request.size
	defer s.cancel()
	if !v.interaction.finish(s) {
		return
	}
	p := &v.panels[s.panel]
	p.feedback.Complete()
	v.interaction.focusPending = true
	if errors.Is(result.err, context.Canceled) || s.ctx.Err() != nil {
		v.showFeedback(s.panel, feedback.Info, "Calculation cancelled")
		p.feedback.Acknowledge()
		v.announceInteraction(eventCancelled)
	} else if result.err != nil {
		message := "Cannot calculate size"
		switch {
		case errors.Is(result.err, fs.ErrPermission):
			message = "Access denied"
		case errors.Is(result.err, fs.ErrNotExist):
			message = "Entry not found"
		}
		v.showFeedback(s.panel, feedback.Error, message)
	} else {
		if result.folderSizes != nil {
			v.applyFolderSizes(result.folderSizes)
		}
		v.showFeedback(s.panel, feedback.Success, sizeResultLabel(result.size))
	}
}

func sizeResultLabel(result browser.SizeResult) string {
	return fileCountLabel(result.Files) + " · " + formatSize(result.Bytes)
}

func fileCountLabel(files int64) string {
	count := strconv.FormatInt(files, 10)
	for i := len(count) - 3; i > 0; i -= 3 {
		count = count[:i] + "," + count[i:]
	}
	word := " files"
	if files == 1 {
		word = " file"
	}
	return count + word
}

// The worker measures each selected target separately so folder records never
// contain a marked selection's aggregate. Publish and save only a complete batch.
func (l *directoryLoader) calculateCachedSize(s *sizeSession) (browser.SizeResult, folderSizeCache, error) {
	if l.folderCacheDir == "" {
		total, err := l.calculateSize(s.ctx, s.paths)
		return total, nil, err
	}
	var total browser.SizeResult
	records := make(folderSizeCache)
	for _, path := range s.paths {
		result, err := l.calculateSize(s.ctx, []string{path})
		if err != nil {
			return browser.SizeResult{}, nil, err
		}
		total.Bytes += result.Bytes
		total.Files += result.Files
		if s.folders[path] {
			records[folderSizeKey(path)] = folderSize{Path: path, Bytes: result.Bytes, Files: result.Files, CalculatedAt: time.Now()}
		}
	}
	if err := s.ctx.Err(); err != nil {
		return browser.SizeResult{}, nil, err
	}
	if len(records) == 0 {
		return total, nil, nil
	}
	for key, record := range records {
		l.folderSizes[key] = record
	}
	l.saveFolderSizeCache()
	return total, l.folderSizes.clone(), nil
}
