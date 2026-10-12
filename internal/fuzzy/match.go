// Package fuzzy scores ordered subsequence matches and returns rune indices for
// highlighting. Adapted from Site Command Palette's core.js fuzzyMatch.
package fuzzy

import (
	"slices"
	"strings"
	"unicode"
)

type Result struct {
	Score   float64
	Indices []int
}

// Match prefers contiguous occurrences within a candidate, then searches all
// subsequence alignments. Continuing a run outweighs jumping to a word start.
func Match(query, candidate string) (Result, bool) {
	if query == "" {
		return Result{}, true
	}
	needle, haystack := []rune(strings.ToLower(query)), []rune(strings.ToLower(candidate))
	var exact Result
	for start := 0; start+len(needle) <= len(haystack); start++ {
		if !slices.Equal(needle, haystack[start:start+len(needle)]) {
			continue
		}
		indices := make([]int, len(needle))
		for i := range indices {
			indices[i] = start + i
		}
		score := float64(len(needle)+(len(needle)-1)*5) - float64(start)*0.5
		if wordStart(haystack, start) {
			score += 4
		}
		exact = better(exact, Result{Score: score, Indices: indices})
	}
	if exact.Indices != nil {
		if exact.Indices[0] == 0 {
			exact.Score += 8
		}
		exact.Score -= float64(len(haystack)) * 0.01
		return exact, true
	}

	previous := make([]Result, len(haystack))
	for ni, letter := range needle {
		current := make([]Result, len(haystack))
		var separated Result
		for i, r := range haystack {
			if i >= 2 && previous[i-2].Indices != nil {
				state := previous[i-2]
				state.Score += float64(i - 2)
				separated = better(separated, state)
			}
			if r != letter {
				continue
			}
			score := 1.0
			if wordStart(haystack, i) {
				score += 4
			}
			if ni == 0 {
				current[i] = Result{Score: score - float64(i)*0.5, Indices: []int{i}}
				continue
			}
			if i > 0 && previous[i-1].Indices != nil {
				current[i] = extend(previous[i-1], i, score+5)
			}
			if separated.Indices != nil {
				current[i] = better(current[i], extend(separated, i, score-float64(i-1)))
			}
		}
		previous = current
	}
	var best Result
	for _, state := range previous {
		best = better(best, state)
	}
	if best.Indices == nil {
		return Result{}, false
	}
	best.Score -= float64(len(haystack)) * 0.01
	return best, true
}

func wordStart(text []rune, index int) bool {
	return index == 0 || unicode.IsSpace(text[index-1]) || strings.ContainsRune("/_-", text[index-1])
}

func extend(state Result, index int, score float64) Result {
	return Result{Score: state.Score + score, Indices: append(slices.Clone(state.Indices), index)}
}

func better(left, right Result) Result {
	if left.Indices == nil {
		return right
	}
	if right.Indices == nil {
		return left
	}
	if right.Score > left.Score || right.Score == left.Score && slices.Compare(right.Indices, left.Indices) < 0 {
		return right
	}
	return left
}
