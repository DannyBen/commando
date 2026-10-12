//go:build !windows

package browser

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestTransfersHaveNoNonWindowsFallback(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	writeTransferFiles(t, source, map[string]string{"file": "data"})
	for _, move := range []bool{false, true} {
		result, err := Transfer(TransferRequest{Paths: []string{filepath.Join(source, "file")}, Directory: target, Move: move})
		if !errors.Is(err, ErrTransferUnavailable) || result.Attempted || len(result.Items) != 0 {
			t.Fatalf("unexpected fallback: %+v %v", result, err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(source, "file")); err != nil || string(data) != "data" {
		t.Fatal("unsupported transfer changed source")
	}
	if entries, err := os.ReadDir(target); err != nil || len(entries) != 0 {
		t.Fatal("unsupported transfer wrote destination")
	}
}
