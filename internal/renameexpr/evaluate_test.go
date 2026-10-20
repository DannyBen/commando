package renameexpr_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/dannyben/commando/internal/renameexpr"
)

func TestBatchExamples(t *testing.T) {
	for _, example := range []struct {
		name, expression string
		before           []renameexpr.Entry
		after            []string
	}{
		{"panel order", `<counter> <name>`, []renameexpr.Entry{{Name: "z.jpg"}, {Name: "a.jpg"}}, []string{"1 z.jpg", "2 a.jpg"}},
		{"zero step", `<counter pad:0 start:10 step:0>`, []renameexpr.Entry{{Name: "a"}, {Name: "b"}}, []string{"10", "10"}},
		{"independent counters", `<c start:9>-<counter start:0 step:5 pad:0>`, []renameexpr.Entry{{Name: "a"}, {Name: "b"}, {Name: "c"}}, []string{"9-00", "10-05", "11-10"}},
		{"unchanged entries keep their index", `<counter>`, []renameexpr.Entry{{Name: "1"}, {Name: "other"}}, []string{"1", "2"}},
		{"folders have no extension", `<base>-new.<ext>`, []renameexpr.Entry{{Name: "folder.v2", IsDir: true}, {Name: "file.v2"}}, []string{"folder.v2-new", "file-new.v2"}},
		{"folder full name", `<name>`, []renameexpr.Entry{{Name: "folder.v2", IsDir: true}}, []string{"folder.v2"}},
		{"empty text in pipeline", `<base remove:hello | from:1 to:10 | case:upper>`, []renameexpr.Entry{{Name: "hello.jpg"}}, []string{""}},
		{"invalid names are caller's responsibility", `CON`, []renameexpr.Entry{{Name: "a"}}, []string{"CON"}},
		{"duplicates are caller's responsibility", `same`, []renameexpr.Entry{{Name: "a"}, {Name: "b"}}, []string{"same", "same"}},
		{"empty batch", `<counter pad:0> <name>`, nil, []string{}},
	} {
		t.Run(example.name, func(t *testing.T) {
			expr, err := renameexpr.Parse(example.expression)
			if err != nil {
				t.Fatal(err)
			}
			before := append([]renameexpr.Entry(nil), example.before...)
			for range 2 {
				got, err := expr.Evaluate(example.before)
				if err != nil || !reflect.DeepEqual(got, example.after) {
					t.Fatalf("got %q, %v; want %q", got, err, example.after)
				}
				if !reflect.DeepEqual(example.before, before) {
					t.Fatal("evaluation mutated input entries")
				}
			}
		})
	}
}

func TestCounterPaddingExamples(t *testing.T) {
	for _, example := range []struct {
		expression  string
		count       int
		first, last string
	}{
		{`<counter pad:0>`, 1, "1", "1"},
		{`<counter pad:0>`, 9, "1", "9"},
		{`<c pad:0 min:1>`, 9, "01", "09"},
		{`<c pad:0 min:1>`, 60, "001", "060"},
		{`<c pad:0 min:1>`, 99, "001", "099"},
		{`<c pad:0 min:1>`, 120, "0001", "0120"},
		{`<c pad:0 min:2>`, 3, "001", "003"},
		{`<c pad:0 min:0>`, 120, "001", "120"},
		{`<c pad:0 min:1 start:10 step:5>`, 120, "0010", "0605"},
		{`<c pad:0 min:1 start:0 step:0>`, 60, "00", "00"},
		{`<counter pad:0>`, 10, "01", "10"},
		{`<counter pad:0>`, 120, "001", "120"},
		{`<counter pad:0 start:10 step:5>`, 120, "010", "605"},
		{`<counter pad:0 start:0 step:5>`, 3, "00", "10"},
		{`<counter pad:0 start:0 step:0>`, 120, "0", "0"},
		{`<counter>`, 120, "1", "120"},
	} {
		t.Run(fmt.Sprintf("%s / %d", example.expression, example.count), func(t *testing.T) {
			expr, err := renameexpr.Parse(example.expression)
			if err != nil {
				t.Fatal(err)
			}
			got, err := expr.Evaluate(make([]renameexpr.Entry, example.count))
			if err != nil {
				t.Fatal(err)
			}
			if got[0] != example.first || got[len(got)-1] != example.last {
				t.Fatalf("got endpoints %q, %q; want %q, %q", got[0], got[len(got)-1], example.first, example.last)
			}
		})
	}
}

func TestCounterOverflow(t *testing.T) {
	for _, example := range []struct {
		expression string
		count      int
		overflow   bool
	}{
		{`<counter start:9223372036854775807>`, 0, false},
		{`<counter start:9223372036854775807>`, 1, false},
		{`<counter start:9223372036854775807>`, 2, true},
		{`<counter start:9223372036854775807 step:0>`, 2, false},
		{`<counter start:0 step:9223372036854775807>`, 2, false},
		{`<counter start:0 step:9223372036854775807>`, 3, true},
		{`<counter start:0 step:4611686018427387904>`, 3, true},
		{`literal<counter start:9223372036854775807 | remove:9>`, 2, true},
	} {
		t.Run(fmt.Sprintf("%s / %d", example.expression, example.count), func(t *testing.T) {
			expr, err := renameexpr.Parse(example.expression)
			if err != nil {
				t.Fatal(err)
			}
			got, err := expr.Evaluate(make([]renameexpr.Entry, example.count))
			if (err != nil) != example.overflow || (err != nil && got != nil) {
				t.Fatalf("got %q, %v; want overflow=%v, no partial results on error", got, err, example.overflow)
			}
		})
	}
}
