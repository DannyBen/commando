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

// Favorite aliases are offered only for bare names, never inside a path or
// environment expression. Quotes delimit a name rather than changing its meaning.
func favoriteCompletionQuery(query string) bool {
	query = strings.Trim(query, `"`)
	return query != "" && !strings.ContainsAny(query, `/\:`) && expandLocationEnvironment(query) == query
}

func favoriteCompletionTarget(favorites []browser.Entry, candidate string) string {
	if !favoriteCompletionQuery(candidate) {
		return ""
	}
	name := strings.Trim(candidate, `"`)
	for _, file := range favorites {
		if strings.EqualFold(strings.TrimSuffix(file.Name, filepath.Ext(file.Name)), name) {
			return file.Target
		}
	}
	return ""
}

func (s *locationSession) selectedFavoriteTarget() string {
	if s.completion.suffix() == "" {
		return ""
	}
	candidate := s.completion.matches[s.completion.selected]
	// A local file may have the very same name as an alias. Both remain in the
	// cycle; favorites precede filesystem entries, so later copies are literal.
	for _, earlier := range s.completion.matches[:s.completion.selected] {
		if strings.EqualFold(earlier, candidate) {
			return ""
		}
	}
	return favoriteCompletionTarget(s.favorites, candidate)
}

func (v *view) updateLocationCompletion() {
	s, ok := v.interaction.current.(*locationSession)
	if !ok {
		return
	}
	// Each editing session gets a fresh shortcut snapshot, even before Home has
	// been visited. This read never probes favorite targets or creates the store.
	if !s.favoritesLoaded && !s.favoritesPending && v.loader != nil && v.favoritesDirectory() != "" {
		s.favoritesPending = v.loader.submit(loadRequest{completion: s, favoritesDirectory: v.favoritesDirectory(), action: actionBrowse})
	}
	s.completion.trailer = ""
	if strings.HasPrefix(v.location.Text(), `"`) {
		s.completion.trailer = `"`
	}
	s.completion.update(&v.location)
	if s.completion.query == "" {
		return
	}
	plan, valid := completionLocation(v.panels[s.panel].locationPath(), s.completion.query)
	if !valid {
		s.completionPath, s.completionLoaded = "", false
		s.completionEntries = nil
	} else {
		if s.completionPath != plan.directory {
			s.completionPath, s.completionLoaded = plan.directory, false
			s.completionEntries = nil
		}
		if !s.completionLoaded && s.pendingPath == "" && v.loader != nil {
			if v.loader.submit(loadRequest{path: plan.directory, completion: s, action: actionBrowse}) {
				s.pendingPath = plan.directory
			}
		}
	}
	s.completion.candidates = s.completion.candidates[:0]
	if favoriteCompletionQuery(s.completion.query) {
		for _, file := range s.favorites {
			if file.Target == "" {
				continue
			}
			name := strings.TrimSuffix(file.Name, filepath.Ext(file.Name))
			if s.completion.trailer != "" {
				name = `"` + name + `"`
			}
			s.completion.candidates = append(s.completion.candidates, name)
		}
	}
	if valid {
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
	}
	s.completion.update(&v.location)
}

// Accepting an alias makes its destination explicit and uses the editor's normal
// undo history. A local entry with the same name cannot retarget an accepted alias.
func (v *view) acceptLocationSuggestion() bool {
	s := v.interaction.current.(*locationSession)
	s.completion.update(&v.location)
	if s.completion.suffix() != "" {
		if target := s.selectedFavoriteTarget(); target != "" {
			if s.completion.trailer != "" {
				target = `"` + target + `"`
			}
			v.location.SetCaret(0, len([]rune(v.location.Text())))
			v.location.Insert(target)
			if s.completion.trailer != "" {
				end := len([]rune(target)) - 1
				v.location.SetCaret(end, end)
			}
			return true
		}
	}
	return s.completion.accept(&v.location)
}

func (v *view) acceptLocationCompletion(result loadResult) {
	s := result.request.completion
	if v.interaction.current != s {
		return
	}
	if result.request.favoritesDirectory != "" {
		s.favoritesPending, s.favoritesLoaded = false, true
		s.favorites = nil
		if result.err == nil {
			s.favorites = slices.Clone(result.favorites)
		}
		v.updateLocationCompletion()
		return
	}
	if s.pendingPath != result.request.path {
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
