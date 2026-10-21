package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"gioui.org/io/key"
)

func writePanelTAR(t *testing.T, filename string, compressed bool) {
	t.Helper()
	var raw bytes.Buffer
	w := tar.NewWriter(&raw)
	if err := w.WriteHeader(&tar.Header{Name: "docs/report.txt", Size: 7, Mode: 0644}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("payload")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data := raw.Bytes()
	if compressed {
		var buffer bytes.Buffer
		gz := gzip.NewWriter(&buffer)
		if _, err := gz.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		data = buffer.Bytes()
	}
	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestTARUnpackUsesSharedCommandsAndKeepsExternalOpening(t *testing.T) {
	for _, extension := range []string{".tar", ".tar.gz", ".tgz", ".TAR.GZ"} {
		t.Run(extension, func(t *testing.T) {
			source, target := resolvedTempDir(t), resolvedTempDir(t)
			filename := filepath.Join(source, "bundle"+extension)
			writePanelTAR(t, filename, extension != ".tar")
			v := newBrowserView([]string{source, target}, nil)
			t.Cleanup(v.finishOperations)
			waitLoaded(t, v, 0)
			waitLoaded(t, v, 1)
			selectName(t, &v.panels[0], filepath.Base(filename))
			v.panels[0].toggleMark(v.panels[0].selected)
			forbidTransferClipboard(t, v)
			opened := make(chan string, 1)
			v.openFile = func(path string) error { opened <- path; return nil }
			send, _ := panelInputFrames(v)
			send(pressed(key.NameReturn))
			select {
			case path := <-opened:
				if path != filename || v.panels[0].archiveFolder != "" {
					t.Fatal("Enter stopped using the external association")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("Enter did not open the archive externally")
			}
			if extension == ".tgz" {
				send(pressed("`"), key.EditEvent{Text: "Entry Unpack archive"})
				send(pressed(key.NameReturn))
			} else {
				send(modified(key.NameF6, key.ModAlt))
			}
			if a := v.inlineInteraction(); a == nil || !slices.Equal(a.options, []string{"Extract here", "Extract to folder"}) {
				t.Fatal("TAR did not reuse the extraction choices")
			}
			folder := ""
			if extension != ".tar" {
				folder = "bundle"
				send(pressed(key.NameDownArrow), pressed(key.NameReturn))
				send()
				if n := v.nameInteraction(); n == nil || v.nameEditor.Text() != folder {
					t.Fatal("default folder did not remove the complete archive suffix")
				}
				send(pressed(key.NameReturn))
				waitCreated(t, v)
			} else {
				send(pressed(key.NameReturn))
				waitLoaded(t, v, 1)
			}
			send()
			data, err := os.ReadFile(filepath.Join(target, folder, "docs", "report.txt"))
			if err != nil || string(data) != "payload" || v.active != 1 || !v.interaction.idle() || v.mutationsBusy() || v.transferPending {
				t.Fatal("TAR extraction failed or retained its session", err)
			}
			if !slices.Equal(v.panels[0].selectedNames(), []string{filepath.Base(filename)}) {
				t.Fatal("Unpack changed source marks")
			}
		})
	}
}
