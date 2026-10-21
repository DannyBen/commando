package browser

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tarUnpackFixture(t *testing.T, extension string, headers ...*tar.Header) string {
	t.Helper()
	var raw bytes.Buffer
	writer := tar.NewWriter(&raw)
	for _, header := range headers {
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA {
			if _, err := writer.Write(bytes.Repeat([]byte("x"), int(header.Size))); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data := raw.Bytes()
	if strings.HasSuffix(strings.ToLower(extension), "gz") {
		var compressed bytes.Buffer
		w := gzip.NewWriter(&compressed)
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		data = compressed.Bytes()
	}
	filename := filepath.Join(t.TempDir(), "bundle"+extension)
	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatal(err)
	}
	return filename
}

func TestUnpackTARVariantsPreserveContentsMetadataAndSource(t *testing.T) {
	modified := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)
	for _, extension := range []string{".tar", ".tar.gz", ".tgz", ".TAR.GZ"} {
		t.Run(extension, func(t *testing.T) {
			filename := tarUnpackFixture(t, extension,
				&tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: 0755},
				&tar.Header{Name: "./docs/résumé.txt", Mode: 0644, Size: 7, ModTime: modified},
				&tar.Header{Name: "loose.txt", Mode: 0644, Size: 3},
				&tar.Header{Name: "empty/", Mode: 0755, Typeflag: tar.TypeDir},
				&tar.Header{Name: "zero.txt", Mode: 0644})
			before, _ := os.ReadFile(filename)
			for _, folder := range []string{"", "extracted"} {
				target := resolvedTempDir(t)
				created, err := UnpackArchive(filename, target, folder)
				if err != nil || created.Name != folder {
					t.Fatal(created, err)
				}
				root := filepath.Join(target, folder)
				for name, size := range map[string]int{"docs/résumé.txt": 7, "loose.txt": 3, "zero.txt": 0} {
					data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
					if err != nil || !bytes.Equal(data, bytes.Repeat([]byte("x"), size)) {
						t.Fatal(name, string(data), err)
					}
				}
				info, err := os.Stat(filepath.Join(root, "docs", "résumé.txt"))
				if err != nil || !info.ModTime().Equal(modified) {
					t.Fatal("file timestamp changed", info, err)
				}
				if info, err := os.Stat(filepath.Join(root, "empty")); err != nil || !info.IsDir() {
					t.Fatal("empty directory missing", err)
				}
			}
			after, _ := os.ReadFile(filename)
			if !bytes.Equal(before, after) || ArchiveFormat(filename) != nil {
				t.Fatal("Unpack changed source bytes or enabled internal browsing")
			}
		})
	}
}

func TestUnpackTARPreflightsNamesTypesAndExistingRoots(t *testing.T) {
	for _, names := range [][]string{
		{"good", "../escape"}, {"good", "/absolute"}, {"A/one", "a/two"}, {"file", "file/child"}, {"same", "./same"}, {"good", `bad\name`},
	} {
		headers := make([]*tar.Header, 0, len(names))
		for _, name := range names {
			headers = append(headers, &tar.Header{Name: name, Mode: 0644, Size: 1})
		}
		filename := tarUnpackFixture(t, ".tar.gz", headers...)
		target := t.TempDir()
		if _, err := UnpackArchive(filename, target, "new"); err == nil {
			t.Fatal("invalid paths accepted", names)
		}
		if entries, _ := os.ReadDir(target); len(entries) != 0 {
			t.Fatal("preflight wrote output", entries)
		}
	}
	for _, kind := range []byte{tar.TypeSymlink, tar.TypeLink, tar.TypeFifo} {
		filename := tarUnpackFixture(t, ".tgz", &tar.Header{Name: "link", Linkname: "target", Typeflag: kind})
		target := t.TempDir()
		if _, err := UnpackArchive(filename, target, "new"); !errors.Is(err, errors.ErrUnsupported) {
			t.Fatal("special entry accepted", kind, err)
		}
		if entries, _ := os.ReadDir(target); len(entries) != 0 {
			t.Fatal("unsupported entry created output")
		}
	}
	for _, existing := range []string{"DOCS", "loose.txt"} {
		filename := tarUnpackFixture(t, ".tar", &tar.Header{Name: "docs/file", Size: 1}, &tar.Header{Name: "loose.txt", Size: 1})
		target := t.TempDir()
		if err := os.WriteFile(filepath.Join(target, existing), []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := UnpackArchive(filename, target, ""); !errors.Is(err, fs.ErrExist) {
			t.Fatal("existing root accepted", err)
		}
		if entries, _ := os.ReadDir(target); len(entries) != 1 {
			t.Fatal("collision created partial output")
		}
	}
}

func TestUnpackTARRejectsTruncationAndGzipChecksumBeforeWriting(t *testing.T) {
	for _, corrupt := range []string{"gzip checksum", "gzip truncated", "tar truncated", "bad header"} {
		extension := ".tar.gz"
		if corrupt == "tar truncated" || corrupt == "bad header" {
			extension = ".tar"
		}
		filename := tarUnpackFixture(t, extension, &tar.Header{Name: "file", Mode: 0644, Size: 1024})
		data, _ := os.ReadFile(filename)
		switch corrupt {
		case "gzip checksum":
			data[len(data)-8] ^= 1
		case "gzip truncated":
			data = data[:len(data)-4]
		case "tar truncated":
			data = data[:600]
		default:
			data[0] ^= 1
		}
		if err := os.WriteFile(filename, data, 0600); err != nil {
			t.Fatal(err)
		}
		target := t.TempDir()
		if _, err := UnpackArchive(filename, target, "new"); err == nil {
			t.Fatal("corrupt archive accepted", corrupt)
		}
		if entries, _ := os.ReadDir(target); len(entries) != 0 {
			t.Fatal("corrupt archive created output", corrupt)
		}
	}
}
