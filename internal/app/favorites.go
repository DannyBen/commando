package app

import (
	"path/filepath"
	"slices"

	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

// A Home lookup owns its captured source panel until naming begins. Cancelling
// either step restores that complete model; stale reads cannot reopen naming.
type favoriteSession struct {
	panel    int
	target   string
	previous panel
}

func (s *favoriteSession) interactionPanel() int        { return s.panel }
func (*favoriteSession) openingEvent() interactionEvent { return eventSoundOwned }
func (*favoriteSession) holdsOperations() bool          { return true }
func (*favoriteSession) blocksPanels() bool             { return true }

func (v *view) favoritesDirectory() string {
	if v.activeDataFolder == "" {
		return ""
	}
	return filepath.Join(v.activeDataFolder, "favorites")
}

func favoriteReady(v *view) bool {
	p := &v.panels[v.active]
	return v.loader != nil && v.favoritesDirectory() != "" && p.nativeFolder() &&
		!p.loading && !p.refreshing && v.canOfferInteraction(v.active, actionMutation)
}

func (v *view) startFavorite(gtx layout.Context) {
	if !favoriteReady(v) || !v.interaction.idle() {
		return
	}
	p := &v.panels[v.active]
	s := &favoriteSession{panel: v.active, target: p.path, previous: *p}
	if !v.startInteraction(s) {
		return
	}
	if !v.loader.submit(loadRequest{panel: s.panel, homeDirectory: v.favoritesDirectory(), favoriteRead: s, action: actionBrowse}) {
		v.finishInteraction(s)
		return
	}
	p.loading = true
	v.dismissFeedback()
	v.focusInteraction(gtx)
}

func (v *view) acceptFavoriteRead(result loadResult) {
	s := result.request.favoriteRead
	if !v.finishInteraction(s) {
		return
	}
	if result.err != nil {
		v.panels[s.panel] = s.previous
		v.showFeedback(s.panel, feedback.Error, "Cannot read favorites")
		v.interaction.focusPending = true
		return
	}
	result.request.favoriteRead = nil
	v.acceptLoad(result)
	p := &v.panels[s.panel]
	for row, item := range p.items {
		if item.file.Favorite && item.file.Target != "" && samePath(item.file.Target, s.target) {
			p.selected, p.reveal = row, true
			v.showFeedback(s.panel, feedback.Info, "Already a favorite")
			v.interaction.focusPending = true
			return
		}
	}
	name := filepath.Base(s.target)
	if browser.ValidateName(name) != nil {
		name = filepath.VolumeName(s.target)
		if browser.ValidateName(name) != nil {
			name = "Favorite"
		}
	}
	n := &nameSession{kind: nameFavorite, panel: s.panel, path: v.favoritesDirectory(), row: len(v.favorites), favorite: s}
	v.beginName(layout.Context{}, n, name, true)
	v.interaction.focusPending = true
}

func (l *directoryLoader) readHome(directory string) (browser.Directory, error) {
	home, err := l.readDirectory("")
	if err != nil {
		return home, err
	}
	favorites, err := l.readFavorites(directory)
	if err != nil {
		return home, err
	}
	home.Entries = append(favorites, home.Entries...)
	return home, nil
}

func (v *view) rememberFavorites(home browser.Directory) {
	v.favorites = nil
	for _, file := range home.Entries {
		if file.Favorite {
			v.favorites = append(v.favorites, file)
		}
	}
}

func (v *view) acceptFavoriteWrite(result loadResult) {
	n := result.request.favoriteWrite
	if v.nameInteraction() != n {
		return
	}
	n.pending, v.nameEditor.ReadOnly = false, false
	if result.err != nil && result.created.Name == "" {
		n.err, n.focused = result.err, false
		v.showFeedback(n.panel, feedback.Error, n.errorText(result.err))
		return
	}
	n.finished = true
	if v.monitor != nil {
		// The store may have been absent when Home first tried to watch it.
		v.monitor.retry(v.favoritesDirectory())
	}
	if result.err != nil {
		result.directory = browser.Directory{Drives: true, Entries: append(slices.Clone(v.panels[n.panel].entries), result.created)}
	}
	v.rememberFavorites(result.directory)
	for pi := range v.panels {
		p := &v.panels[pi]
		if !p.drives {
			continue
		}
		p.reconcileDirectoryListing(result.directory)
		if pi == n.panel {
			for row, item := range p.items {
				if item.file.Favorite && item.file.Name == result.created.Name {
					p.selected, p.reveal = row, true
				}
			}
		}
	}
	if result.err != nil {
		v.showFeedback(n.panel, feedback.Warning, "Added; refresh failed")
	} else {
		v.showFeedback(n.panel, feedback.Success, "Favorite added")
	}
}

// Home keeps its previous browsing path. Entry actions use the shortcut store.
func (v *view) entryDirectory(index int) string {
	if v.panels[index].drives {
		return v.favoritesDirectory()
	}
	return v.panels[index].path
}

func (p *panel) favoriteSelected() bool {
	return p.drives && p.markable(p.selected) && p.items[p.selected].file.Favorite
}

func (p *panel) entryPath(row int) string {
	file := p.items[row].file
	if p.drives {
		return file.Path
	}
	return filepath.Join(p.path, file.Name)
}

func (v *view) refreshHomePanels() {
	for i := range v.panels {
		if v.panels[i].drives {
			v.panels[i].refreshNeeded = true
		}
	}
}
