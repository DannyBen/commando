package app

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"github.com/dannyben/commando/internal/settings"
)

func (v *view) historySearchDelay() time.Duration {
	return time.Duration(v.settingsValues.Get(settings.CatalogueDefault, settingHistoryMatchDelay).Number) * time.Millisecond
}

func historyQuery(text string) (string, bool) {
	if !strings.HasPrefix(text, ">") {
		return "", false
	}
	return strings.TrimSpace(text[1:]), true
}

// Search the saved paths in memory. Only a settled query may read its best
// match's parent; typing itself never probes the filesystem.
func (v *view) updateHistorySearch(gtx layout.Context, s *locationSession) bool {
	query, searching := historyQuery(v.location.Text())
	if !searching {
		s.searchQuery, s.searchTarget, s.searchResolved, s.searchFailed, s.searchDue = "", "", "", "", time.Time{}
		s.searchMatches = nil
		s.searchSelected = 0
		return false
	}
	s.completion.candidates = nil
	s.completion.matches = nil
	if query != s.searchQuery {
		s.searchQuery, s.searchTarget, s.searchResolved, s.searchFailed = query, "", "", ""
		s.searchMatches = nil
		s.searchSelected = 0
		s.searchDue = gtx.Now.Add(v.historySearchDelay())
	}
	if query == "" {
		return true
	}
	if gtx.Now.Before(s.searchDue) {
		gtx.Execute(op.InvalidateCmd{At: s.searchDue})
		return true
	}
	if s.searchResolved != query {
		s.searchMatches = v.visited.matches(query)
		s.searchSelected = 0
		s.searchTarget = ""
		if len(s.searchMatches) > 0 {
			s.searchTarget = s.searchMatches[0]
		}
		s.searchResolved = query
	}
	target := s.searchTarget
	if target == "" || samePath(target, s.searchFailed) || v.loader == nil {
		return true
	}
	p := &v.panels[s.panel]
	parent, name := filepath.Dir(target), filepath.Base(target)
	if !p.loading && p.nativeFolder() && samePath(p.path, parent) {
		for i, item := range p.items {
			if item.file.Directory && samePath(item.file.Name, name) {
				p.selected, p.reveal = i, true
				return true
			}
		}
	}
	if p.loading {
		return true
	}
	request := loadRequest{panel: s.panel, path: parent, selectName: name, suggestedPath: target,
		historySearch: s, action: actionBrowse}
	if v.loader.submit(request) {
		p.loading, p.navigating = true, true
	}
	return true
}

func (v *view) cycleHistorySearch(gtx layout.Context, s *locationSession) {
	if len(s.searchMatches) < 2 {
		return
	}
	s.searchSelected = (s.searchSelected + 1) % len(s.searchMatches)
	s.searchTarget = s.searchMatches[s.searchSelected]
	s.searchFailed = ""
	v.updateHistorySearch(gtx, s)
}

func (v *view) submitHistorySearch(gtx layout.Context, s *locationSession) {
	query, ok := historyQuery(v.location.Text())
	if !ok || query == "" {
		return
	}
	target := s.searchTarget
	if s.searchResolved != query {
		target = v.visited.best(query)
	}
	if target == "" {
		return
	}
	v.interaction.finish(s)
	v.focusInteraction(gtx)
	p := &v.panels[s.panel]
	if p.loading {
		s.searchAccept = target
		return
	}
	v.queueLoad(loadRequest{panel: s.panel, path: target, explicitPath: true, historyVisit: true})
}

// A late parent read cannot replace a newer search result. Enter may arrive
// while that read is pending; then open the accepted folder after it drains.
func (v *view) acceptHistorySearch(result loadResult) bool {
	s := result.request.historySearch
	if s == nil {
		return false
	}
	p := &v.panels[s.panel]
	p.loading, p.navigating = false, false
	if s.searchAccept != "" {
		v.queueLoad(loadRequest{panel: s.panel, path: s.searchAccept, explicitPath: true, historyVisit: true})
		return true
	}
	if v.interaction.current != s || !samePath(s.searchTarget, result.request.suggestedPath) {
		return true
	}
	if result.err != nil {
		if os.IsNotExist(result.err) {
			v.visited.forget(result.request.suggestedPath)
			s.searchResolved = ""
		} else {
			s.searchFailed = result.request.suggestedPath
		}
		return true
	}
	found := false
	for _, entry := range result.directory.Entries {
		if entry.Directory && !entry.Parent && samePath(entry.Name, filepath.Base(result.request.suggestedPath)) {
			found = true
			break
		}
	}
	if !found {
		v.visited.forget(result.request.suggestedPath)
		s.searchResolved = ""
		return true
	}
	// The normal directory result path below installs the parent and selects the
	// matched child. This is a real panel change; Escape keeps it.
	return false
}
