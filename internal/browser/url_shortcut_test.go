package browser

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestURLShortcut(t *testing.T) {
	for _, tc := range []struct{ text, name string }{
		{"https://example.com/a/page?q=one%20two&x=1#section", "example.com.url"},
		{" \thttp://www.example.com:8080/path\r\n", "www.example.com.url"},
		{"HTTPS://Example.COM/path", "Example.COM.url"},
		{"http://localhost:3000/", "localhost.url"},
		{"http://127.0.0.1:8080/", "127.0.0.1.url"},
		{"https://[::1]:8080/path", "--1.url"},
		{"https://con.example/path", "link.url"},
		{"", ""}, {"ordinary text", ""}, {"example.com/path", ""},
		{"ftp://example.com/file", ""}, {"file:///C:/file", ""},
		{"https:///path", ""}, {"https://", ""}, {"https://example.com:bad", ""},
		{"https://example.com/a b", ""},
		{"https://example.com/\nhttps://other.example/", ""},
		{"https://example.com/\r\nIconFile=bad", ""},
		{"https://example.com/\x00hidden", ""},
	} {
		t.Run(tc.text, func(t *testing.T) {
			name, data := URLShortcut(tc.text)
			if name != tc.name {
				t.Fatalf("name = %q, want %q", name, tc.name)
			}
			if name == "" {
				if len(data) != 0 {
					t.Fatal("unrelated text produced shortcut contents")
				}
				return
			}
			if err := ValidateName(name); err != nil {
				t.Fatal("invalid proposed name", err)
			}
			if string(data) != "[InternetShortcut]\r\nURL="+strings.TrimSpace(tc.text)+"\r\n" {
				t.Fatalf("complete URL changed: %q", data)
			}
		})
	}
}

func TestURLShortcutPreservesUnicode(t *testing.T) {
	const target = "https://example.com/שלום?q=🎵#פרק"
	name, data := URLShortcut(target)
	if name != "example.com.url" || len(data)%2 != 0 || binary.LittleEndian.Uint16(data) != 0xfeff {
		t.Fatal("Unicode shortcut lacks UTF-16LE BOM")
	}
	var words []uint16
	for i := 2; i < len(data); i += 2 {
		words = append(words, binary.LittleEndian.Uint16(data[i:]))
	}
	if got := string(utf16.Decode(words)); got != "[InternetShortcut]\r\nURL="+target+"\r\n" {
		t.Fatal("Unicode URL changed", got)
	}
}
