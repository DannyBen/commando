package browser

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCalculateSizeSkipsWindowsJunctions(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "data"), []byte("not counted"), 0644); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{"loop": root, "outside": outside} {
		if output, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(root, name), target).CombinedOutput(); err != nil {
			t.Fatalf("create junction: %v: %s", err, output)
		}
	}
	got, err := CalculateSize(context.Background(), []string{root})
	if err != nil || got != (SizeResult{}) {
		t.Fatalf("followed junction: %+v, %v", got, err)
	}
}
