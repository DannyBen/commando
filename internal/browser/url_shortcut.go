package browser

import (
	"encoding/binary"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf16"
)

// URLShortcut proposes a hostname-only name and captures the complete URL.
// Unrelated text and multiple URLs do not become files. No network access is used.
func URLShortcut(text string) (string, []byte) {
	text = strings.TrimSpace(text)
	if strings.ContainsFunc(text, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return "", nil
	}
	u, err := url.Parse(text)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", nil
	}
	// IPv6 hosts contain colons, which cannot appear in a Windows filename.
	name := strings.ReplaceAll(u.Hostname(), ":", "-") + ".url"
	if ValidateName(name) != nil {
		name = "link.url"
	}
	contents := "[InternetShortcut]\r\nURL=" + text + "\r\n"
	if strings.ContainsFunc(contents, func(r rune) bool { return r > 127 }) {
		// Windows INI readers recognize UTF-16LE with a BOM. Keep Unicode URLs
		// intact rather than interpreting UTF-8 bytes through the system code page.
		words := utf16.Encode([]rune(contents))
		data := make([]byte, 2+len(words)*2)
		binary.LittleEndian.PutUint16(data, 0xfeff)
		for i, word := range words {
			binary.LittleEndian.PutUint16(data[2+i*2:], word)
		}
		return name, data
	}
	return name, []byte(contents)
}
