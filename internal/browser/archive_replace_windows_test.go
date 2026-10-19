package browser

import (
	"bytes"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsArchiveReplacementPreservesNamedStreams(t *testing.T) {
	filename := archiveFixture(t)
	stream := []byte("[ZoneTransfer]\r\nZoneId=3\r\n")
	if err := os.WriteFile(filename+":Zone.Identifier", stream, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := DeleteArchive(Location{Path: filename, Folder: "."}, []string{"a2.txt"}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filename + ":Zone.Identifier")
	if err != nil || !bytes.Equal(stream, after) {
		t.Fatal("archive rewrite lost original named stream", err)
	}
	_, contents, _ := zipMembers(t, filename)
	if _, exists := contents["a2.txt"]; exists {
		t.Fatal("replacement did not install edited ZIP")
	}
	assertNoZIPTemporary(t, filename)
}

func TestWindowsArchiveReplacementFailureKeepsOriginal(t *testing.T) {
	filename := archiveFixture(t)
	before, _ := os.ReadFile(filename)
	name, err := windows.UTF16PtrFromString(filename)
	if err != nil {
		t.Fatal(err)
	}
	// Other readers may allow ordinary reads/writes while preventing replacement.
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if result, err := DeleteArchive(Location{Path: filename, Folder: "."}, []string{"a2.txt"}); err == nil || len(result.Names) != 0 {
		t.Fatal("locked archive unexpectedly replaced", result, err)
	}
	after, err := os.ReadFile(filename)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed Windows replacement damaged original", err)
	}
	assertNoZIPTemporary(t, filename)
}
