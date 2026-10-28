package app

import (
	"context"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"gioui.org/layout"
	"gioui.org/text"
	"github.com/dannyben/commando/internal/feedback"
)

type deepFilter struct {
	root, query string
	matches     map[string]bool
	started     time.Time
	searching   bool
	cancel      context.CancelFunc
	ctx         context.Context
	dirs        []string
	progress    feedback.Channel
}

type deepSearchRequest struct {
	ctx    context.Context
	filter *deepFilter
	dirs   []string
}

func deepKey(path string) string { return strings.ToLower(filepath.Clean(path)) }

func deepContains(root, path string) bool {
	rootKey, pathKey := deepKey(root), deepKey(path)
	return pathKey == rootKey || strings.HasPrefix(pathKey, strings.TrimRight(rootKey, string(filepath.Separator))+string(filepath.Separator))
}

func (d *deepFilter) includes(path string) bool { return d.matches[deepKey(path)] }

func (d *deepFilter) add(path string) {
	for {
		d.matches[deepKey(path)] = true
		if samePath(path, d.root) {
			return
		}
		parent := filepath.Dir(path)
		if parent == path {
			return
		}
		path = parent
	}
}

func (v *view) startDeepSearch(panelIndex int, query string) {
	p := &v.panels[panelIndex]
	previous := p.deep
	ctx, cancel := context.WithCancel(context.Background())
	d := &deepFilter{root: p.path, query: query, matches: make(map[string]bool), started: time.Now(), searching: true, cancel: cancel, ctx: ctx}
	if v.loader == nil || !v.loader.submit(loadRequest{panel: panelIndex, deepSearch: &deepSearchRequest{ctx: ctx, filter: d, dirs: []string{p.path}}, action: actionBrowse}) {
		cancel()
		v.showFeedback(panelIndex, feedback.Error, "Cannot search")
		return
	}
	if previous != nil {
		previous.cancel()
	}
	p.ensureEntries()
	p.deep, p.filter = d, ""
	p.clearMarks()
	p.refilter()
	d.progress.QuietProgressAt("Searching…", d.started.Add(v.appearance.FeedbackProgressDelay))
}

func (v *view) clearDeepSearch(index int) {
	p := &v.panels[index]
	if p.deep == nil {
		return
	}
	p.deep.cancel()
	p.deep.progress.Complete()
	p.deep = nil
	p.feedback.Complete()
	p.refilter()
}

func (v *view) acceptDeepSearch(result loadResult) {
	d := result.request.deepSearch.filter
	for i := range v.panels {
		p := &v.panels[i]
		if p.deep != d {
			continue
		}
		if result.err != nil {
			v.clearDeepSearch(i)
			v.showFeedback(i, feedback.Error, readError(result.err))
			return
		}
		for _, path := range result.deepMatches {
			d.add(path)
		}
		if result.deepDone {
			d.searching = false
			d.progress.Complete()
		} else {
			d.dirs = result.deepDirs
			v.resumeDeepSearch(i)
		}
		p.refilter()
		return
	}
}

func (v *view) resumeDeepSearches() {
	for i := range v.panels {
		v.resumeDeepSearch(i)
	}
}

func (v *view) resumeDeepSearch(index int) {
	d := v.panels[index].deep
	if d == nil || !d.searching || len(d.dirs) == 0 || v.loader == nil {
		return
	}
	request := loadRequest{panel: index, deepSearch: &deepSearchRequest{ctx: d.ctx, filter: d, dirs: d.dirs}, action: actionBrowse}
	if v.loader.submit(request) {
		d.dirs = nil
	}
}

func (l *directoryLoader) runDeepSearch(request loadRequest) {
	d := request.deepSearch.filter
	dirs := request.deepSearch.dirs
	var batch []string
	var scanErr error
	for count := 0; count < 16 && len(dirs) > 0 && request.deepSearch.ctx.Err() == nil; count++ {
		dir := dirs[0]
		dirs = dirs[1:]
		entries, err := os.ReadDir(dir)
		if err != nil {
			if samePath(dir, d.root) {
				scanErr = err
			}
			continue // Inaccessible branches do not hide accessible matches.
		}
		for _, entry := range entries {
			if request.deepSearch.ctx.Err() != nil {
				break
			}
			path := filepath.Join(dir, entry.Name())
			if strings.Contains(strings.ToLower(entry.Name()), strings.ToLower(d.query)) {
				batch = append(batch, path)
			}
			if entry.IsDir() {
				dirs = append(dirs, path)
			}
		}
	}
	result := loadResult{request: request, deepMatches: batch, deepDirs: dirs, deepDone: len(dirs) == 0 || request.deepSearch.ctx.Err() != nil, err: scanErr}
	select {
	case l.results <- result:
		if l.invalidate != nil {
			l.invalidate()
		}
	case <-l.done:
	}
}

func (v *view) deepRowContents(gtx layout.Context, item entry, nameColor, metadataColor color.NRGBA, mode metadataColumn, roles fileTextRoles, query string) layout.Dimensions {
	name := mode.displayName(item)
	if !strings.Contains(strings.ToLower(name), strings.ToLower(query)) {
		name = item.name
	}
	start := strings.Index(strings.ToLower(name), strings.ToLower(query))
	var indices []int
	if start >= 0 {
		first := utf8.RuneCountInString(name[:start])
		for i := range utf8.RuneCountInString(query) {
			indices = append(indices, first+i)
		}
	}
	return v.textAs(roles.filename, func(gtx layout.Context) layout.Dimensions {
		return v.contentInset().Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				metadata := mode.valueAt(item, v.dateDisplay(), gtx.Now)
				if mode == metadataExtension && name == item.name {
					metadata = ""
				}
				var trailing layout.Widget
				if metadata != "" {
					trailing = func(gtx layout.Context) layout.Dimensions {
						return v.labelFor(roles.metadata, gtx, metadata, v.appearance.TextSize, metadataColor, text.End)
					}
				}
				return v.trailingLayout(gtx, func(gtx layout.Context) layout.Dimensions {
					return v.highlightedLabelBold(gtx, name, indices, v.appearance.TextSize, nameColor, item.file.Directory)
				}, trailing)
			})
		})
	})(gtx)
}
