package fuzzy

import (
	"math"
	"slices"
	"strings"
	"testing"
)

// These ranking examples are shared with Site Command Palette.
func TestRanking(t *testing.T) {
	for _, tc := range []struct {
		query string
		order []string
	}{
		{"facebook", []string{"https://facebook.com/a-long-page-name", "https://a.co/somewhere/facebook"}},
		{"gitdan", []string{"https://gitdan.com/", "https://github.com/danny", "https://something.com/git/dan", "https://github.com/something/danny", "https://github.com/something/david/now"}},
		{"facebook", []string{"Facebook > A much longer page name", "Other > Somewhere > Facebook"}},
		{"ab", []string{"a-b" + strings.Repeat("x", 21), "axxxxxxxx-b" + strings.Repeat("x", 13)}},
		{"abcd", []string{"abcdxxx", "ab---cd", "a-b-c-d"}},
		{"abcd", []string{"xxxx/ab/cd", "ab/xxxxxxxx/cd"}},
		{"abcd", []string{"ab/cd/long-trailing-text", "prefix/ab/cd"}},
		{"gitdan", []string{"Github > Danny", "Github > Danny > More pages"}},
	} {
		t.Run(tc.query+tc.order[0], func(t *testing.T) {
			actual := slices.Clone(tc.order)
			slices.Reverse(actual)
			slices.SortStableFunc(actual, func(a, b string) int {
				left, lok := Match(tc.query, a)
				right, rok := Match(tc.query, b)
				if !lok || !rok {
					t.Fatalf("expected matches: %q, %q", a, b)
				}
				if left.Score > right.Score {
					return -1
				}
				return 1
			})
			if !slices.Equal(actual, tc.order) {
				t.Fatalf("got %v, want %v", actual, tc.order)
			}
		})
	}
}

func TestHighlightAlignment(t *testing.T) {
	for _, tc := range []struct {
		query, candidate string
		indices          []int
		matches          bool
	}{
		{"abc", "a/b___ab/c", []int{6, 7, 9}, true},
		{"FACEBOOK", "/facebook/x/facebook", []int{1, 2, 3, 4, 5, 6, 7, 8}, true},
		{"bash", "Bashly > Home", []int{0, 1, 2, 3}, true},
		{"bash", "Github > Bashly", []int{9, 10, 11, 12}, true},
		{"åö", "Ångström Öppna", []int{0, 9}, true},
		{"ba", "ab", nil, false},
		{"abc", "ab", nil, false},
		{"", "anything", nil, true},
	} {
		result, ok := Match(tc.query, tc.candidate)
		if ok != tc.matches || !slices.Equal(result.Indices, tc.indices) {
			t.Errorf("Match(%q, %q) = %+v, %v; want %v, %v", tc.query, tc.candidate, result, ok, tc.indices, tc.matches)
		}
	}
}

// Enumerate independent alignments to guard against a greedy port. When there
// is a contiguous occurrence, only contiguous alignments may supply highlights.
func exhaustive(query, candidate string) (Result, bool) {
	if query == "" {
		return Result{}, true
	}
	needle, haystack := []rune(query), []rune(candidate)
	var best Result
	var walk func([]int, int)
	walk = func(indices []int, from int) {
		if len(indices) < len(needle) {
			for i := from; i < len(haystack); i++ {
				if haystack[i] == needle[len(indices)] {
					walk(append(slices.Clone(indices), i), i+1)
				}
			}
			return
		}
		contiguous := indices[len(indices)-1]-indices[0]+1 == len(indices)
		if strings.Contains(candidate, query) && !contiguous {
			return
		}
		score := -float64(indices[0])*0.5 - float64(len(haystack))*0.01
		for j, i := range indices {
			score++
			if i == 0 || strings.ContainsRune(" /_-", haystack[i-1]) {
				score += 4
			}
			if j > 0 {
				if gap := i - indices[j-1] - 1; gap == 0 {
					score += 5
				} else {
					score -= float64(gap)
				}
			}
		}
		if contiguous && indices[0] == 0 {
			score += 8
		}
		if best.Indices == nil || score > best.Score {
			best = Result{Score: score, Indices: slices.Clone(indices)}
		}
	}
	walk(nil, 0)
	return best, best.Indices != nil
}

func TestMatchesExhaustiveAlignments(t *testing.T) {
	var candidates []string
	var build func(string, int)
	build = func(prefix string, remaining int) {
		candidates = append(candidates, prefix)
		if remaining > 0 {
			for _, r := range "ab/" {
				build(prefix+string(r), remaining-1)
			}
		}
	}
	build("", 6)
	for _, query := range []string{"", "a", "b", "aa", "ab", "ba", "bb", "aaa", "aab", "aba", "abb", "baa", "bab", "bba", "bbb"} {
		for _, candidate := range candidates {
			got, gok := Match(query, candidate)
			want, wok := exhaustive(query, candidate)
			if gok != wok || !slices.Equal(got.Indices, want.Indices) || math.Abs(got.Score-want.Score) > 1e-9 {
				t.Fatalf("Match(%q, %q) = %+v, %v; exhaustive %+v, %v", query, candidate, got, gok, want, wok)
			}
		}
	}
}
