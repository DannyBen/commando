package app

type renameSpan struct {
	text  string
	added bool
}

// Keep a Unicode character diff in the session, rather than recomputing it on
// every frame. Trim shared ends before finding the longest common subsequence.
func renameSpans(before, after string) []renameSpan {
	a, b := []rune(before), []rune(after)
	var spans []renameSpan
	appendText := func(text string, added bool) {
		if text == "" {
			return
		}
		if len(spans) > 0 && spans[len(spans)-1].added == added {
			spans[len(spans)-1].text += text
		} else {
			spans = append(spans, renameSpan{text, added})
		}
	}
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	appendText(string(b[:prefix]), false)
	a, b = a[prefix:], b[prefix:]
	suffix := 0
	for suffix < len(a) && suffix < len(b) && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	tail := string(b[len(b)-suffix:])
	a, b = a[:len(a)-suffix], b[:len(b)-suffix]
	// Bound work for expressions producing unusually long names. Ordinary
	// Windows filenames fit in this table, including a full-name replacement.
	if len(a)+1 > 65536/(len(b)+1) {
		appendText(string(b), true)
	} else {
		width := len(b) + 1
		common := make([]int, (len(a)+1)*width)
		for i := len(a) - 1; i >= 0; i-- {
			for j := len(b) - 1; j >= 0; j-- {
				if a[i] == b[j] {
					common[i*width+j] = 1 + common[(i+1)*width+j+1]
				} else {
					common[i*width+j] = max(common[(i+1)*width+j], common[i*width+j+1])
				}
			}
		}
		i, j := 0, 0
		for i < len(a) && j < len(b) {
			if a[i] == b[j] {
				appendText(string(b[j]), false)
				i++
				j++
			} else if common[(i+1)*width+j] >= common[i*width+j+1] {
				i++
			} else {
				appendText(string(b[j]), true)
				j++
			}
		}
		appendText(string(b[j:]), true)
	}
	appendText(tail, false)
	return spans
}
