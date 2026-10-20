package renameexpr_test

import (
	"testing"

	"github.com/dannyben/commando/internal/renameexpr"
)

// Each row is: before, expression, after. Counters use a single-entry batch here;
// multi-entry ordering and padding examples live in evaluate_test.go.
func TestExamples(t *testing.T) {
	examples := []struct{ before, expression, after string }{
		{"hello.jpg", `<name>`, "hello.jpg"},
		{"hello.jpg", `<name from:1 to:3>`, "hel"},
		{"hello.jpg", `<base from:1 to:3>.<ext>`, "hel.jpg"},
		{"hello.jpg", `Holiday-<name>`, "Holiday-hello.jpg"},
		{"hello.jpg", `<base>-final.<ext>`, "hello-final.jpg"},
		{"hello.jpg", `<name><name>`, "hello.jpghello.jpg"},
		{"hello.jpg", `<ext>-<base>-<name>`, "jpg-hello-hello.jpg"},
		{"hello.jpg", `literal`, "literal"},
		{"hello.jpg", ``, ""},
		{"hello.jpg", `<n>`, "hello.jpg"},
		{"hello.jpg", `<b>.<e>`, "hello.jpg"},
		{"hello.jpg", `<c>-<n>`, "1-hello.jpg"},
		{"hello.jpg", `<counter start:10 step:5 pad:0> <name>`, "10 hello.jpg"},
		{"hello.jpg", `<counter start:0 step:0>`, "0"},
		{"hello.jpg", `<c pad:0 min:1>. <name>`, "01. hello.jpg"},
		{"hello - Copy.jpg", `<c pad:0 min:1>. <name remove:" - Copy">`, "01. hello.jpg"},
		{"hello.jpg", `<counter min:2 pad:0>`, "001"},
		{"hello.jpg", `<counter pad:0 min:0>`, "1"},
		{"hello.jpg", `<counter pad:0 min:1 | remove:0>`, "1"},
		{"hello.jpg", `<counter pad:0 | replace:1 with:One>`, "One"},
		{"hello.jpg", `<counter start:9223372036854775807>`, "9223372036854775807"},
		{"Image.Holiday.jpg", `<base replace:Image with:Photo>.<ext>`, "Photo.Holiday.jpg"},
		{"Image.Holiday.jpg", `<base remove:Image.>.<ext>`, "Holiday.jpg"},
		{"PhotoPhoto.jpg", `<base replace:Photo with:PhotoPhoto>.<ext>`, "PhotoPhotoPhotoPhoto.jpg"},
		{"PhotoPHOTO.jpg", `<base replace:Photo with:Image>.<ext>`, "ImagePHOTO.jpg"},
		{"PhotoPhoto.jpg", `<base remove:Photo>.<ext>`, ".jpg"},
		{"hello - Copy.jpg", `<base remove:" - Copy">.<ext>`, "hello.jpg"},
		{"Photo Photo.jpg", `<base replace:"Photo " with:Image>.<ext>`, "ImagePhoto.jpg"},
		{"Family Photo.jpg", `<base replace:"Family Photo" with:Trip>.<ext>`, "Trip.jpg"},
		{"Photo.jpg", `<base replace:Photo with:"">.<ext>`, ".jpg"},
		{"hello.jpg", `<base with:hi replace:hello>.<ext>`, "hi.jpg"},
		{"Photo.jpg", `<base replace:Photo with:photo>-<base>.<ext>`, "photo-Photo.jpg"},
		{"Image.Holiday.jpg", `<base case:lower>.<ext case:upper>`, "image.holiday.JPG"},
		{"ÄÉΩ.jpg", `<base case:lower>.<ext>`, "äéω.jpg"},
		{"äéω.jpg", `<base case:upper>.<ext>`, "ÄÉΩ.jpg"},
		{"Oldest.jpg", `<base from:1 to:3 | replace:Old with:NewName>.<ext>`, "NewName.jpg"},
		{"Oldest.jpg", `<base replace:Old with:NewName | from:1 to:3>.<ext>`, "New.jpg"},
		{"OLD.jpg", `<base case:lower | replace:old with:new>.<ext>`, "new.jpg"},
		{"OLD.jpg", `<base replace:old with:new | case:lower>.<ext>`, "old.jpg"},
		{"Old - Copy.jpg", `<base remove:" - Copy" | replace:Old with:New | case:lower>.<ext>`, "new.jpg"},
		{"hello.jpg", `<base from:2 to:4>.<ext>`, "ell.jpg"},
		{"hello.jpg", `<base to:3>.<ext>`, "hel.jpg"},
		{"hello.jpg", `<base from:4>.<ext>`, "lo.jpg"},
		{"hello.jpg", `<base to:3 from:2>.<ext>`, "el.jpg"},
		{"hello.jpg", `<base from:3 to:3>.<ext>`, "l.jpg"},
		{"hello.jpg", `<base from:4 to:999>.<ext>`, "lo.jpg"},
		{"hello.jpg", `<base from:99>.<ext>`, ".jpg"},
		{"hello.jpg", `<base from:1 to:3>-<base from:4>.<ext>`, "hel-lo.jpg"},
		{"你好世界.jpg", `<base from:2 to:3>.<ext>`, "好世.jpg"},
		{"😀🚀🎉.jpg", `<base from:2 to:2>.<ext>`, "🚀.jpg"},
		{"e\u0301x.jpg", `<base from:1 to:2>.<ext>`, "e\u0301.jpg"},
		{"archive.tar.gz", `<base>-new.<ext>`, "archive.tar-new.gz"},
		{".env", `<base>-new.<ext>`, ".env-new"},
		{".env.local", `<base>-new.<ext>`, ".env-new.local"},
		{"README", `<base>-new.<ext>`, "README-new"},
		{"README", `<ext>`, ""},
		{"hello.jpg", `<base>[final].<ext>`, "hello[final].jpg"},
		{"hello[final].jpg", `<name>`, "hello[final].jpg"},
		{"hello.jpg", `<base> 100%.<ext>`, "hello 100%.jpg"},
		{"hello.jpg", `<base replace:hello with:"a:b">`, "a:b"},
		{"hello.jpg", `<base replace:hello with:"<name>|x">`, "<name>|x"},
		{"hello.jpg", `<base replace:hello with:"a\"b">`, "a\"b"},
		{"hello.jpg", `<base replace:hello with:"a\\b">`, `a\b`},
		{"hello.jpg", `<name replace:hello with:" x ">`, " x .jpg"},
		{"hello.jpg", `< name >`, "hello.jpg"},
		{"hello.jpg", `<base replace:hello with:HELLO|case:lower>.<ext>`, "hello.jpg"},
		{"hello.jpg", `<base>...`, "hello"},
		{"hello.jpg", `...`, ""},
		{"hello.jpg", `<base> `, "hello "},
	}
	for _, example := range examples {
		t.Run(example.before+" / "+example.expression, func(t *testing.T) {
			expr, err := renameexpr.Parse(example.expression)
			if err != nil {
				t.Fatal(err)
			}
			got, err := expr.Evaluate([]renameexpr.Entry{{Name: example.before}})
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0] != example.after {
				t.Fatalf("before %q, expression %q: got %q, want %q", example.before, example.expression, got, example.after)
			}
		})
	}
}
