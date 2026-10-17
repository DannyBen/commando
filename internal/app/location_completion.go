package app

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/dannyben/commando/internal/browser"
)

// Resolve the final component separately, preserving the user's parent spelling,
// relative paths, environment variables and quotes in accepted candidates.
type locationCompletionPath struct {
	directory, prefix, trailer string
}

func completionLocation(base, query string) (locationCompletionPath, bool) {
	var plan locationCompletionPath
	if query == "" {
		return plan, false
	}
	if strings.HasPrefix(query, `"`) {
		plan.prefix, plan.trailer = `"`, `"`
		query = strings.TrimPrefix(query, `"`)
		query = strings.TrimSuffix(query, `"`)
	}
	expanded := expandLocationEnvironment(query)
	if strings.HasPrefix(expanded, `\\`) || strings.HasPrefix(expanded, "//") {
		return plan, false
	}
	split := strings.LastIndexFunc(query, func(r rune) bool { return r < 128 && os.IsPathSeparator(uint8(r)) }) + 1
	parent := query[:split]
	plan.prefix += parent
	// A complete environment variable can itself contain the parent directory.
	// Expand only the lookup path; keep the original variable in the suggestion.
	if split == 0 && strings.HasSuffix(query, "%") && expanded != query {
		parent = query
		plan.prefix += query + string(filepath.Separator)
	}
	if runtime.GOOS == "windows" && split == 0 && len(query) >= 2 && query[1] == ':' {
		parent = query[:2]
		plan.prefix += parent
		if len(query) == 2 {
			parent += `\`
			plan.prefix += `\`
		} else {
			parent += "."
		}
	}
	path := expandLocationEnvironment(parent)
	if path == "" {
		path = base
	} else if !filepath.IsAbs(path) && filepath.VolumeName(path) == "" {
		if os.IsPathSeparator(path[0]) {
			path = filepath.VolumeName(base) + path
		} else {
			path = filepath.Join(base, path)
		}
	}
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//") {
		return plan, false
	}
	plan.directory = filepath.Clean(path)
	return plan, plan.directory != "." && plan.directory != ""
}

func (v *view) updateLocationCompletion() {
	s, ok := v.interaction.current.(*locationSession)
	if !ok {
		return
	}
	s.completion.trailer = ""
	if strings.HasPrefix(v.location.Text(), `"`) {
		s.completion.trailer = `"`
	}
	s.completion.update(&v.location)
	if s.completion.query == "" {
		return
	}
	plan, ok := completionLocation(v.panels[s.panel].locationPath(), s.completion.query)
	if !ok {
		s.completionPath, s.completionLoaded = "", false
		s.completion.candidates = nil
		s.completion.update(&v.location)
		return
	}
	if s.completionPath != plan.directory {
		s.completionPath, s.completionLoaded = plan.directory, false
		s.completionEntries, s.completion.candidates = nil, nil
	}
	if !s.completionLoaded && s.pendingPath == "" && v.loader != nil {
		if v.loader.submit(loadRequest{path: plan.directory, completion: s, action: actionBrowse}) {
			s.pendingPath = plan.directory
		}
	}
	s.completion.candidates = s.completion.candidates[:0]
	for _, file := range s.completionEntries {
		if file.Parent || file.Hidden && file.System {
			continue
		}
		name := plan.prefix + file.Name
		if file.Directory {
			name += string(filepath.Separator)
		}
		s.completion.candidates = append(s.completion.candidates, name+plan.trailer)
	}
	s.completion.update(&v.location)
}

func (v *view) acceptLocationCompletion(result loadResult) {
	s := result.request.completion
	if v.interaction.current != s || s.pendingPath != result.request.path {
		return
	}
	s.pendingPath = ""
	if s.completionPath == result.request.path {
		s.completionLoaded = true
		s.completionEntries = nil
		if result.err == nil {
			s.completionEntries = slices.Clone(result.directory.Entries)
		}
		slices.SortFunc(s.completionEntries, browser.CompareEntries)
	}
	// Re-evaluate the live text; a completed read never restores an older query,
	// changes the caret, or navigates the panel. Lookup failures stay quiet.
	v.updateLocationCompletion()
}
