package browser

import (
	"path/filepath"
	"runtime"
	"strings"
)

type transferJournal struct {
	observations map[string]transferObservation
	roots        map[string]string
	openFolders  []string
}

func newTransferJournal(request TransferRequest) transferJournal {
	journal := transferJournal{observations: make(map[string]transferObservation), roots: make(map[string]string), openFolders: request.OpenFolders}
	for _, path := range request.Paths {
		journal.roots[transferPathKey(path)] = path
	}
	return journal
}

func (journal *transferJournal) snapshot() []transferObservation {
	observations := make([]transferObservation, 0, len(journal.observations))
	for _, observation := range journal.observations {
		observations = append(observations, observation)
	}
	return observations
}

func (journal *transferJournal) needs(source string) bool {
	if _, selected := journal.roots[transferPathKey(source)]; selected {
		return true
	}
	for _, folder := range journal.openFolders {
		if folder != "" && pathWithin(source, folder) {
			return true
		}
	}
	for path := filepath.Dir(source); ; path = filepath.Dir(path) {
		key := transferPathKey(path)
		if _, selected := journal.roots[key]; selected {
			_, recorded := journal.observations[key]
			return !recorded
		}
		if filepath.Dir(path) == path {
			return false
		}
	}
}

// Keep one outcome per selected root, plus directories containing open panels.
// Millions of child files do not require millions of UI bookkeeping records.
func (s *transferJournal) observe(observation transferObservation) {
	for path := observation.source; ; path = filepath.Dir(path) {
		if root, ok := s.roots[transferPathKey(path)]; ok {
			key := transferPathKey(root)
			if sameTransferPath(root, observation.source) {
				s.observations[key] = observation
			} else if _, exists := s.observations[key]; !exists {
				relative, _ := filepath.Rel(root, observation.source)
				target := observation.destination
				for relative != "." {
					target = filepath.Dir(target)
					relative = filepath.Dir(relative)
				}
				s.observations[key] = transferObservation{source: root, destination: target, directory: true}
			}
			break
		}
		if filepath.Dir(path) == path {
			break
		}
	}
	if observation.directory {
		for _, folder := range s.openFolders {
			if folder != "" && pathWithin(observation.source, folder) {
				s.observations[transferPathKey(observation.source)] = observation
				break
			}
		}
	}
}

func transferPathKey(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToUpper(path)
	}
	return path
}
