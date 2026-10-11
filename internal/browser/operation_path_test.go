package browser

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolveDOSPath(t *testing.T) {
	for _, tc := range []struct {
		name, path, want string
		mappings         map[string]string
		queries          []string
	}{
		{"physical", `C:\Work\note.txt`, `C:\Work\note.txt`, map[string]string{"C:": `\Device\HarddiskVolume3`}, []string{"C:"}},
		{"subst", `W:\some-file`, `C:\Work\some-file`, map[string]string{"W:": `\??\C:\Work`, "C:": `\Device\HarddiskVolume3`}, []string{"W:", "C:"}},
		{"nested", `w:\notes\one.txt`, `C:\Work\Project\notes\one.txt`, map[string]string{"W:": `\??\V:\Project`, "V:": `\??\C:\Work`, "C:": `\Device\HarddiskVolume3`}, []string{"W:", "V:", "C:"}},
		{"unicode", `W:\日記\🎵.txt`, `C:\My Work\日記\🎵.txt`, map[string]string{"W:": `\??\C:\My Work\`, "C:": `\Device\HarddiskVolume3`}, []string{"W:", "C:"}},
		{"root alias", `W:\notes.txt`, `C:\notes.txt`, map[string]string{"W:": `\??\C:\`, "C:": `\Device\HarddiskVolume3`}, []string{"W:", "C:"}},
		{"drive root", `W:\`, `C:\Work\`, map[string]string{"W:": `\??\C:\Work`, "C:": `\Device\HarddiskVolume3`}, []string{"W:", "C:"}},
		{"extended", `\\?\W:\note.txt`, `\\?\C:\Work\note.txt`, map[string]string{"W:": `\??\C:\Work`, "C:": `\Device\HarddiskVolume3`}, []string{"W:", "C:"}},
		{"unc", `\\server\share\note.txt`, `\\server\share\note.txt`, nil, nil},
		{"extended unc", `\\?\UNC\server\share\note.txt`, `\\?\UNC\server\share\note.txt`, nil, nil},
		{"unc alias", `W:\note.txt`, `\\server\share\note.txt`, map[string]string{"W:": `\??\UNC\server\share`}, []string{"W:"}},
		{"network drive", `N:\note.txt`, `N:\note.txt`, map[string]string{"N:": `\Device\LanmanRedirector\server\share`}, []string{"N:"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var queries []string
			got, err := resolveDOSPath(tc.path, func(drive string) (string, error) {
				queries = append(queries, drive)
				mapping, ok := tc.mappings[drive]
				if !ok {
					return "", os.ErrNotExist
				}
				return mapping, nil
			})
			if err != nil || got != tc.want || !reflect.DeepEqual(queries, tc.queries) {
				t.Fatalf("resolved=%q err=%v queries=%v; want=%q queries=%v", got, err, queries, tc.want, tc.queries)
			}
		})
	}
}

func TestResolveDOSPathRefusesInvalidMappings(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		mappings   map[string]string
		want       error
	}{
		{"self cycle", `W:\note.txt`, map[string]string{"W:": `\??\w:\Work`}, ErrDriveMappingCycle},
		{"nested cycle", `W:\note.txt`, map[string]string{"W:": `\??\V:\Work`, "V:": `\??\W:\Other`}, ErrDriveMappingCycle},
		{"missing drive", `W:\note.txt`, nil, os.ErrNotExist},
		{"missing backing drive", `W:\note.txt`, map[string]string{"W:": `\??\C:\Work`}, os.ErrNotExist},
		{"unknown mapping", `W:\note.txt`, map[string]string{"W:": `\Unknown\Work`}, ErrDriveMapping},
		{"relative mapping", `W:\note.txt`, map[string]string{"W:": `\??\C:Work`}, ErrDriveMapping},
		{"empty mapping", `W:\note.txt`, map[string]string{"W:": ""}, ErrDriveMapping},
		{"drive relative", `W:note.txt`, nil, ErrDriveMapping},
		{"device path", `\\.\C:\note.txt`, nil, ErrDriveMapping},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveDOSPath(tc.path, func(drive string) (string, error) {
				mapping, ok := tc.mappings[drive]
				if !ok {
					return "", os.ErrNotExist
				}
				return mapping, nil
			})
			if got != "" || !errors.Is(err, tc.want) {
				t.Fatalf("resolved=%q err=%v; want %v", got, err, tc.want)
			}
		})
	}
}

func TestResolveDOSPathKeepsLongSuffix(t *testing.T) {
	suffix := strings.Repeat(`long folder\`, 50) + "note.txt"
	got, err := resolveDOSPath(`W:\`+suffix, func(drive string) (string, error) {
		if drive == "W:" {
			return `\??\C:\Work`, nil
		}
		return `\Device\HarddiskVolume3`, nil
	})
	if err != nil || got != `C:\Work\`+suffix {
		t.Fatalf("long path was truncated: %q, %v", got, err)
	}
}

func TestResolveOperationPathReadsOnly(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "日記.txt")
	contents := []byte("leave this file untouched")
	if err := os.WriteFile(path, contents, 0600); err != nil {
		t.Fatal(err)
	}
	for _, item := range []string{root, path} {
		resolved, err := ResolveOperationPath(item)
		if err != nil {
			t.Fatal(err)
		}
		if err := verifyOperationPath(item, resolved); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(contents) {
		t.Fatalf("probe changed or removed file: contents=%q err=%v", got, err)
	}
	if got, err := ResolveOperationPath(filepath.Join(root, "missing")); got != "" || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file resolved: %q, %v", got, err)
	}
}

func TestVerifyOperationPathRefusesDifferentEntries(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	for _, path := range []string{a, b} {
		if err := os.WriteFile(path, []byte("same contents"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := verifyOperationPath(a, b); !errors.Is(err, ErrPathIdentity) {
		t.Fatalf("different entries were accepted: %v", err)
	}
}

func TestResolveOperationPathKeepsFinalLink(t *testing.T) {
	root := t.TempDir()
	target, link := filepath.Join(root, "target"), filepath.Join(root, "link")
	if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for _, broken := range []bool{false, true} {
		if broken {
			if err := os.Remove(target); err != nil {
				t.Fatal(err)
			}
		}
		resolved, err := ResolveOperationPath(link)
		if err != nil || filepath.Base(resolved) != "link" {
			t.Fatalf("link was followed or rejected: path=%q err=%v broken=%v", resolved, err, broken)
		}
		if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("probe changed the link: %v", err)
		}
	}
}
