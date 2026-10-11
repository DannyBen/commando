package browser

import (
	"cmp"
	"path/filepath"
	"strings"
)

// CompareEntries keeps parents and folders first. Files compare by stem, then
// final extension; folders compare by their whole name. Number runs compare by
// value, case is ignored, and exact names break remaining ties consistently.
func CompareEntries(a, b Entry) int {
	if a.Parent != b.Parent {
		if a.Parent {
			return -1
		}
		return 1
	}
	if a.Directory != b.Directory {
		if a.Directory {
			return -1
		}
		return 1
	}
	aName, bName := a.Name, b.Name
	aExt, bExt := "", ""
	if !a.Directory {
		aName, aExt = splitFilename(aName)
		bName, bExt = splitFilename(bName)
	}
	if order := compareNatural(aName, bName); order != 0 {
		return order
	}
	if order := compareNatural(aExt, bExt); order != 0 {
		return order
	}
	return strings.Compare(a.Name, b.Name)
}

func splitFilename(name string) (stem, extension string) {
	extension = filepath.Ext(name)
	if extension == name {
		// A dotfile such as .env has a name, rather than only an extension.
		extension = ""
	}
	return strings.TrimSuffix(name, extension), extension
}

func compareNatural(a, b string) int {
	a, b = strings.ToLower(a), strings.ToLower(b)
	zeroOrder := 0
	for a != "" && b != "" {
		if asciiDigit(a[0]) && asciiDigit(b[0]) {
			ai, bi := 1, 1
			for ai < len(a) && asciiDigit(a[ai]) {
				ai++
			}
			for bi < len(b) && asciiDigit(b[bi]) {
				bi++
			}
			an, bn := strings.TrimLeft(a[:ai], "0"), strings.TrimLeft(b[:bi], "0")
			// Comparing digit counts and text avoids integer overflow.
			if order := cmp.Compare(len(an), len(bn)); order != 0 {
				return order
			}
			if order := strings.Compare(an, bn); order != 0 {
				return order
			}
			if zeroOrder == 0 {
				zeroOrder = cmp.Compare(ai, bi)
			}
			a, b = a[ai:], b[bi:]
			continue
		}
		if order := cmp.Compare(a[0], b[0]); order != 0 {
			return order
		}
		a, b = a[1:], b[1:]
	}
	if order := cmp.Compare(len(a), len(b)); order != 0 {
		return order
	}
	// Equal numeric names put fewer leading zeros first, after all other
	// segments have been compared (v01.2 still precedes v1.10).
	return zeroOrder
}

func asciiDigit(value byte) bool { return value >= '0' && value <= '9' }
