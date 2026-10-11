package browser

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrDriveMapping      = errors.New("unsupported drive mapping")
	ErrDriveMappingCycle = errors.New("drive mapping cycle")
	ErrPathIdentity      = errors.New("resolved path identifies a different item")
)

// ResolveOperationPath expands Windows SUBST drive aliases and verifies that
// the resulting path names the same entry. It only reads mappings and metadata;
// it never moves or deletes anything. Final symlinks/junctions remain entries,
// rather than being replaced by their targets. This does not test recyclability.
func ResolveOperationPath(path string) (string, error) {
	resolved, err := resolveOperationPath(path)
	if err != nil {
		return "", err
	}
	if err := verifyOperationPath(path, resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

// DOS drive mappings are resolved independently of host filepath semantics so
// nested aliases, Unicode and invalid mappings can also be checked on Linux.
// query must return only the current (first) QueryDosDevice mapping.
func resolveDOSPath(path string, query func(string) (string, error)) (string, error) {
	extended := strings.HasPrefix(path, `\\?\`)
	if extended {
		path = path[4:]
		if strings.HasPrefix(strings.ToUpper(path), `UNC\`) {
			path = `\\` + path[4:]
		}
	}
	seen := make(map[string]bool)
	for {
		if strings.HasPrefix(path, `\\`) && !strings.HasPrefix(path, `\\.\`) && !strings.HasPrefix(path, `\\?\`) {
			break // A UNC path has no DOS drive alias to expand.
		}
		if !rootedDOSPath(path) {
			return "", ErrDriveMapping
		}
		drive := strings.ToUpper(path[:2])
		if seen[drive] {
			return "", ErrDriveMappingCycle
		}
		seen[drive] = true
		mapping, err := query(drive)
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", drive, err)
		}
		if strings.HasPrefix(strings.ToUpper(mapping), `\DEVICE\`) {
			break // An ordinary physical or mapped network drive.
		}
		if !strings.HasPrefix(mapping, `\??\`) {
			return "", ErrDriveMapping
		}
		target := mapping[4:]
		if strings.HasPrefix(strings.ToUpper(target), `UNC\`) {
			target = `\\` + target[4:]
		} else if !rootedDOSPath(target) {
			return "", ErrDriveMapping
		}
		path = strings.TrimRight(target, `\`) + `\` + path[3:]
	}
	if extended {
		if strings.HasPrefix(path, `\\`) {
			return `\\?\UNC\` + path[2:], nil
		}
		return `\\?\` + path, nil
	}
	return path, nil
}

func rootedDOSPath(path string) bool {
	return len(path) >= 3 && path[1] == ':' && path[2] == '\\' &&
		(path[0] >= 'A' && path[0] <= 'Z' || path[0] >= 'a' && path[0] <= 'z')
}
