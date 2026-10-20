// Package renameexpr evaluates multi-rename expressions without filesystem or UI access.
package renameexpr

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Expression is a parsed, immutable expression. Use Parse to construct one.
type Expression struct {
	parts []part
}

type part struct {
	literal string
	source  string
	counter counter
	ops     []operation
}

type counter struct {
	start, step int64
	pad         bool
	min         int // Extra zeroes beyond the largest generated value's digit count.
}

type operation struct {
	kind, search, replacement string
	from, to                  int
}

// Parse validates syntax and arguments, including operations on an empty batch.
// It does not validate generated filenames or check destination collisions.
func Parse(expression string) (*Expression, error) {
	if !utf8.ValidString(expression) {
		return nil, fmt.Errorf("expression is not valid UTF-8")
	}
	p := parser{text: expression}
	result := &Expression{}
	for p.pos < len(p.text) {
		start := p.pos
		for p.pos < len(p.text) && p.text[p.pos] != '<' {
			p.pos++
		}
		if p.pos > start {
			result.parts = append(result.parts, part{literal: p.text[start:p.pos]})
		}
		if p.pos == len(p.text) {
			break
		}
		p.pos++
		token, err := p.token()
		if err != nil {
			return nil, fmt.Errorf("at byte %d: %w", p.pos+1, err)
		}
		result.parts = append(result.parts, token)
	}
	return result, nil
}

type parser struct {
	text string
	pos  int
}

func (p *parser) space() bool {
	start := p.pos
	for p.pos < len(p.text) {
		r, size := utf8.DecodeRuneInString(p.text[p.pos:])
		if !unicode.IsSpace(r) {
			break
		}
		p.pos += size
	}
	return p.pos != start
}

func (p *parser) bare() string {
	start := p.pos
	for p.pos < len(p.text) {
		r, size := utf8.DecodeRuneInString(p.text[p.pos:])
		if unicode.IsSpace(r) || strings.ContainsRune(`<>|:"`, r) {
			break
		}
		p.pos += size
	}
	return p.text[start:p.pos]
}

func (p *parser) value() (string, error) {
	if p.pos == len(p.text) || p.text[p.pos] != '"' {
		value := p.bare()
		if value == "" {
			return "", fmt.Errorf("missing argument value")
		}
		return value, nil
	}
	p.pos++
	var value strings.Builder
	for p.pos < len(p.text) {
		ch := p.text[p.pos]
		p.pos++
		switch ch {
		case '"':
			return value.String(), nil
		case '\\':
			if p.pos == len(p.text) || p.text[p.pos] != '\\' && p.text[p.pos] != '"' {
				return "", fmt.Errorf("invalid quoted escape")
			}
			ch = p.text[p.pos]
			p.pos++
		}
		value.WriteByte(ch)
	}
	return "", fmt.Errorf("unclosed quoted value")
}

func (p *parser) token() (part, error) {
	p.space()
	token := part{source: p.bare(), counter: counter{start: 1, step: 1}}
	switch token.source {
	case "n", "name":
		token.source = "name"
	case "b", "base":
		token.source = "base"
	case "e", "ext":
		token.source = "ext"
	case "c", "counter":
		token.source = "counter"
	default:
		return part{}, fmt.Errorf("unknown token %q", token.source)
	}
	first := true
	for {
		args := map[string]string{}
		for {
			spaced := p.space()
			if p.pos == len(p.text) {
				return part{}, fmt.Errorf("unclosed token")
			}
			if strings.ContainsRune("|>", rune(p.text[p.pos])) {
				break
			}
			if !spaced && (first || len(args) > 0) {
				return part{}, fmt.Errorf("arguments must be separated by spaces")
			}
			key := p.bare()
			if key == "" || p.pos == len(p.text) || p.text[p.pos] != ':' {
				return part{}, fmt.Errorf("expected keyword:value")
			}
			if _, exists := args[key]; exists {
				return part{}, fmt.Errorf("duplicate argument %q", key)
			}
			p.pos++
			value, err := p.value()
			if err != nil {
				return part{}, err
			}
			args[key] = value
		}
		if !first && len(args) == 0 {
			return part{}, fmt.Errorf("empty pipeline stage")
		}
		if token.source == "counter" && first {
			if err := configureCounter(&token.counter, args); err != nil {
				return part{}, err
			}
		}
		op, err := parseOperation(args)
		if err != nil {
			return part{}, err
		}
		if op.kind != "" {
			token.ops = append(token.ops, op)
		}
		end := p.text[p.pos]
		p.pos++
		if end == '>' {
			return token, nil
		}
		first = false
	}
}

func configureCounter(c *counter, args map[string]string) error {
	_, hasMinimum := args["min"]
	for _, key := range []string{"start", "step", "pad", "min"} {
		value, ok := args[key]
		if !ok {
			continue
		}
		if key == "pad" {
			if value != "0" {
				return fmt.Errorf("pad must be 0")
			}
			c.pad = true
		} else {
			n, err := nonnegative(value)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			if key == "min" {
				// A Windows filename has room for at most 254 leading zeroes
				// and one digit. Reject oversized allocations while parsing.
				if n > 254 {
					return fmt.Errorf("min must be between 0 and 254")
				}
				c.min = int(n)
			} else if key == "start" {
				c.start = n
			} else {
				c.step = n
			}
		}
		delete(args, key)
	}
	if hasMinimum && !c.pad {
		return fmt.Errorf("min requires pad:0")
	}
	return nil
}

func nonnegative(value string) (int64, error) {
	if value == "" || strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return 0, fmt.Errorf("expected a nonnegative integer")
	}
	return strconv.ParseInt(value, 10, 64)
}

func parseOperation(args map[string]string) (operation, error) {
	op := operation{from: 1}
	for _, key := range []string{"replace", "remove", "case", "from", "to"} {
		value, ok := args[key]
		if !ok {
			continue
		}
		kind := key
		if key == "from" || key == "to" {
			kind = "slice"
		}
		if op.kind != "" && op.kind != kind {
			return operation{}, fmt.Errorf("separate operations with a pipe")
		}
		op.kind = kind
		switch key {
		case "replace", "remove":
			if value == "" {
				return operation{}, fmt.Errorf("search text cannot be empty")
			}
			op.search = value
			if key == "replace" {
				var exists bool
				op.replacement, exists = args["with"]
				if !exists {
					return operation{}, fmt.Errorf("replace requires with")
				}
				delete(args, "with")
			}
		case "case":
			if value != "lower" && value != "upper" {
				return operation{}, fmt.Errorf("case must be lower or upper")
			}
			op.kind = value
		case "from", "to":
			n, err := nonnegative(value)
			if err != nil || n == 0 || int64(int(n)) != n {
				return operation{}, fmt.Errorf("%s must be a positive integer", key)
			}
			if key == "from" {
				op.from = int(n)
			} else {
				op.to = int(n)
			}
		}
		delete(args, key)
	}
	if len(args) > 0 {
		return operation{}, fmt.Errorf("unknown or misplaced argument")
	}
	if op.to != 0 && op.to < op.from {
		return operation{}, fmt.Errorf("slice end precedes start")
	}
	return op, nil
}
