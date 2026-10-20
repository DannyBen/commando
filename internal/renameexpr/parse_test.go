package renameexpr_test

import (
	"strings"
	"testing"

	"github.com/dannyben/commando/internal/renameexpr"
)

func TestInvalidExpressions(t *testing.T) {
	for _, expression := range []string{
		"\xff", "<", "<>", "< >", "<unknown>", "<Name>", "<name", "<name<base>>",
		`<name replace:old>`, `<name with:new>`, `<name remove:"">`, `<name replace:"" with:new>`,
		`<name replace:old with:new with:other>`, `<name remove:x remove:y>`,
		`<name case:title>`, `<name case:LOWER>`, `<name case:>`,
		`<name case :lower>`, `<name case: lower>`, `<name case=lower>`,
		`<name replace:"old"with:"new">`, `<name replace:old with:two:words>`,
		`<name remove:two"words>`, `<name remove:"two"words>`,
		`<name remove:"unclosed>`, `<name remove:"bad\q">`, `<name remove:"bad\`,
		`<name remove:>`, `<name remove:<base>>`, `<name remove:x extra:y>`,
		`<name remove:x case:lower>`, `<name from:1 remove:x>`,
		`<name replace:x with:y remove:z>`, `<name case:lower from:1>`,
		`<name |>`, `<name | >`, `<name || case:lower>`, `<name case:lower | >`,
		`<name | case:lower | | remove:x>`,
		`<base from:0>`, `<base to:0>`, `<base from:-1>`, `<base to:-1>`,
		`<base from:4 to:2>`, `<base from:1.5>`, `<base from:+1>`,
		`<base from:9223372036854775808>`, `<base to:abc>`,
		`<counter start:-1>`, `<counter step:-1>`, `<counter start:1.5>`,
		`<counter start:+1>`, `<counter step:1e2>`, `<counter start:"">`,
		`<counter step:9223372036854775808>`, `<counter pad:3>`,
		`<counter start:1 start:2>`, `<counter | start:2>`, `<counter | pad:0>`,
		`<c min:1>`, `<c pad:0 min:-1>`, `<c pad:0 min:1.5>`, `<c pad:0 min:255>`,
		`<c pad:0 min:9223372036854775808>`, `<c pad:0 min:1 min:2>`, `<c pad:0 | min:1>`, `<name min:1>`,
		`<name start:1>`, `<base step:1>`, `<ext pad:0>`, `<name unknown:value>`,
		`<name> <base from:0>`,
	} {
		t.Run(expression, func(t *testing.T) {
			expr, err := renameexpr.Parse(expression)
			if err == nil || expr != nil {
				t.Fatalf("Parse(%q) = %v, %v; want nil expression and error", expression, expr, err)
			}
		})
	}
}

func TestParseExplainsErrors(t *testing.T) {
	for _, example := range []struct{ expression, reason string }{
		{`<who>`, "unknown token"},
		{`<name`, "unclosed token"},
		{`<name remove:"x>`, "unclosed quoted value"},
		{`<name remove:x case:lower>`, "pipe"},
		{`<name replace:x>`, "requires with"},
		{`<base from:2 to:1>`, "precedes start"},
		{`<counter step:-1>`, "nonnegative integer"},
		{`<name remove:x remove:y>`, "duplicate argument"},
	} {
		_, err := renameexpr.Parse(example.expression)
		if err == nil || !strings.Contains(err.Error(), example.reason) {
			t.Errorf("Parse(%q): got %v, want reason %q", example.expression, err, example.reason)
		}
	}
}

func FuzzExpression(f *testing.F) {
	for _, seed := range []string{
		`<name>`, `<counter pad:0 start:0 step:5> <base from:2 | case:upper>.<ext>`,
		`<name replace:"a>b" with:"<name>|\"">`, `<name remove:"bad\`, "\xff", "",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, expression string) {
		expr, err := renameexpr.Parse(expression)
		if err != nil {
			return
		}
		entries := []renameexpr.Entry{{Name: "你好.jpg"}, {Name: "folder.v2", IsDir: true}}
		first, err := expr.Evaluate(entries)
		if err != nil {
			if first != nil {
				t.Fatal("error returned partial results")
			}
			return
		}
		second, err := expr.Evaluate(entries)
		if err != nil || len(first) != len(entries) || len(second) != len(entries) {
			t.Fatalf("inconsistent evaluation: %q, %q, %v", first, second, err)
		}
		for i := range first {
			if first[i] != second[i] {
				t.Fatal("evaluation changed the expression")
			}
		}
	})
}
