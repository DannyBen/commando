package browser

import (
	"errors"
	"io/fs"
	"testing"
)

func TestShellClipboardPreparationIsCopyOnlyAndValidatesDestination(t *testing.T) {
	sequence := uint32(7)
	base := TransferRequest{ShellClipboard: &sequence, Directory: resolvedTempDir(t), Owner: 42}
	prepared, err := prepareTransfer(base)
	if err != nil || prepared.ShellClipboard != &sequence || prepared.Owner != 42 || len(prepared.Paths) != 0 {
		t.Fatalf("Shell source preparation: %+v %v", prepared, err)
	}
	for _, kind := range []string{"paths", "names", "move", "duplicate", "missing destination"} {
		t.Run(kind, func(t *testing.T) {
			request := base
			switch kind {
			case "paths":
				request.Paths = []string{"source"}
			case "names":
				request.Names = []string{"name"}
			case "move":
				request.Move = true
			case "duplicate":
				request.Duplicate = true
			case "missing destination":
				request.Directory += "/missing"
			}
			_, err := prepareTransfer(request)
			if kind == "missing destination" {
				if !errors.Is(err, fs.ErrNotExist) {
					t.Fatal(err)
				}
			} else if !errors.Is(err, fs.ErrInvalid) {
				t.Fatal("accepted ambiguous or mutating Shell request", err)
			}
		})
	}
}
