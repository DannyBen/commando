//go:build !windows

package browser

import "sync"

// Linux supports development of file operations; native file clipboard
// interoperability is provided on the target platform, Windows.
var developmentClipboard struct {
	sync.Mutex
	data FileClipboard
}

func ReadFileClipboard() (FileClipboard, error) {
	developmentClipboard.Lock()
	defer developmentClipboard.Unlock()
	data := developmentClipboard.data
	data.Paths = append([]string(nil), data.Paths...)
	if data.Archive != nil {
		data.Archive = &ArchiveClipboard{Location: data.Archive.Location, Names: append([]string(nil), data.Archive.Names...), Stamp: data.Archive.Stamp}
	}
	if len(data.Paths) == 0 && data.Archive == nil {
		return data, ErrNoClipboardFiles
	}
	return data, nil
}

func WriteFileClipboard(paths []string, move bool) (FileClipboard, error) {
	if _, err := encodeFileDrop(paths); err != nil {
		return FileClipboard{}, err
	}
	developmentClipboard.Lock()
	defer developmentClipboard.Unlock()
	developmentClipboard.data = FileClipboard{Paths: append([]string(nil), paths...), Move: move, Sequence: developmentClipboard.data.Sequence + 1}
	return developmentClipboard.data, nil
}

func FileClipboardSequence() uint32 {
	developmentClipboard.Lock()
	defer developmentClipboard.Unlock()
	return developmentClipboard.data.Sequence
}

func CompleteFileClipboard(snapshot FileClipboard, moved []string) (FileClipboard, error) {
	developmentClipboard.Lock()
	defer developmentClipboard.Unlock()
	if !snapshot.Move || len(moved) == 0 || developmentClipboard.data.Sequence != snapshot.Sequence {
		return developmentClipboard.data, nil
	}
	if snapshot.Archive != nil {
		remaining, err := snapshot.Archive.Remaining(moved)
		if err != nil {
			return snapshot, err
		}
		developmentClipboard.data = FileClipboard{Archive: remaining, Move: remaining != nil, Sequence: snapshot.Sequence + 1}
	} else {
		developmentClipboard.data = FileClipboard{Paths: remainingClipboardPaths(snapshot, moved), Move: true, Sequence: snapshot.Sequence + 1}
	}
	return developmentClipboard.data, nil
}

func WriteArchiveClipboard(reference *ArchiveClipboard) (FileClipboard, error) {
	data, err := encodeArchiveClipboard(reference)
	if err != nil {
		return FileClipboard{}, err
	}
	copied, err := decodeArchiveClipboard(data)
	if err != nil {
		return FileClipboard{}, err
	}
	developmentClipboard.Lock()
	defer developmentClipboard.Unlock()
	developmentClipboard.data = FileClipboard{Archive: copied, Move: reference.Move, Sequence: developmentClipboard.data.Sequence + 1}
	return FileClipboard{Archive: reference, Move: reference.Move, Sequence: developmentClipboard.data.Sequence}, nil
}
