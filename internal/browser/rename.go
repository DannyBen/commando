package browser

import (
	"errors"
	"path/filepath"
	"strings"
	"unicode"
)

var ErrInvalidName = errors.New("invalid filename")

// ValidateName accepts a single Windows filename, never a navigation path.
func ValidateName(name string) error {
	if name == "" || name == "." || name == ".." || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return ErrInvalidName
	}
	for _, r := range name {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return ErrInvalidName
		}
	}
	base, _, _ := strings.Cut(strings.ToUpper(name), ".")
	base = strings.TrimRight(base, " ")
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" {
		return ErrInvalidName
	}
	if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
		if suffix := strings.TrimPrefix(strings.TrimPrefix(base, "COM"), "LPT"); len([]rune(suffix)) == 1 && strings.ContainsAny(suffix, "123456789¹²³") {
			return ErrInvalidName
		}
	}
	return nil
}

// Rename changes one entry's name within its directory without replacing a
// destination that already exists. The platform primitive enforces this atomically.
func Rename(directory, oldName, newName string) error {
	if err := ValidateName(newName); err != nil {
		return err
	}
	if oldName == "" || oldName == "." || oldName == ".." || strings.ContainsAny(oldName, `/\`) {
		return ErrInvalidName
	}
	if oldName == newName {
		return nil
	}
	return renameExclusive(filepath.Join(directory, oldName), filepath.Join(directory, newName))
}
