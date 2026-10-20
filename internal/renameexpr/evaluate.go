package renameexpr

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Entry is one selected file or folder, in the order used for counters.
type Entry struct {
	Name  string
	IsDir bool
}

// Evaluate generates complete destination names in input order. Trailing dots
// are removed. Names may still be empty, invalid, or conflicting: the caller
// must validate destinations before any filesystem mutation. Errors return no
// partial results. The expression and entries are never modified.
func (e *Expression) Evaluate(entries []Entry) ([]string, error) {
	widths := make([]int, len(e.parts))
	for i, part := range e.parts {
		if part.source != "counter" || len(entries) == 0 {
			continue
		}
		lastIndex := int64(len(entries) - 1)
		if part.counter.step != 0 && lastIndex > (math.MaxInt64-part.counter.start)/part.counter.step {
			return nil, fmt.Errorf("counter exceeds integer range")
		}
		if part.counter.pad {
			last := part.counter.start + lastIndex*part.counter.step
			widths[i] = len(strconv.FormatInt(last, 10)) + part.counter.min
		}
	}
	result := make([]string, len(entries))
	for index, entry := range entries {
		base, ext := splitName(entry)
		var name strings.Builder
		for i, part := range e.parts {
			text := part.literal
			switch part.source {
			case "name":
				text = entry.Name
			case "base":
				text = base
			case "ext":
				text = ext
			case "counter":
				text = strconv.FormatInt(part.counter.start+int64(index)*part.counter.step, 10)
				if padding := widths[i] - len(text); padding > 0 {
					text = strings.Repeat("0", padding) + text
				}
			}
			for _, op := range part.ops {
				text = op.apply(text)
			}
			name.WriteString(text)
		}
		result[index] = strings.TrimRight(name.String(), ".")
	}
	return result, nil
}

func splitName(entry Entry) (base, ext string) {
	if !entry.IsDir {
		if dot := strings.LastIndexByte(entry.Name, '.'); dot > 0 {
			return entry.Name[:dot], entry.Name[dot+1:]
		}
	}
	return entry.Name, ""
}

func (op operation) apply(text string) string {
	switch op.kind {
	case "replace", "remove":
		return strings.ReplaceAll(text, op.search, op.replacement)
	case "lower":
		return strings.ToLower(text)
	case "upper":
		return strings.ToUpper(text)
	case "slice":
		chars := []rune(text)
		if op.from > len(chars) {
			return ""
		}
		end := len(chars)
		if op.to != 0 && op.to < end {
			end = op.to
		}
		return string(chars[op.from-1 : end])
	}
	return text
}
