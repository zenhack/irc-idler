package mask

import (
	"regexp"
	"testing"
)

// A set of specific (pattern, input, result) triples to check.
var specificCases = []struct {
	Pattern     string
	Input       string
	ShouldMatch bool
}{
	{"a", "a", true},
	{"a", "b", false},
	{"a?", "ab", true},
	{"a?", "a", false},
	{"?a", "a", false},
	{"?a", "ab", false},
	{"?a", "ba", true},
	{"a?c", "abc", true},
	{"a?c", "aac", true},
	{"a?c", "ac", false},
	{"a?c", "abdc", false},
	{"a*c", "abc", true},
	{"a*c", "ac", true},
	{"a*c", "abdc", true},
	{"a*c", "a", false},
	{"a*c", "c", false},
	{"a\\?a", "a?a", true},
	{"a\\?a", "aaa", false},
	{"a\\*a", "a*a", true},
	{"a\\*a", "aa", false},
	// On escaping backslashes: the spec doesn't say that backslashes can escape other
	// backslashes, but... (1) I don't trust it, and (2) It seems an odd choice. I'm
	// willing to be this is the sort of thing we can't count on implementations in
	// the wild to be consistent with. TODO: actually study some implementations and
	// see how they handle it.
	{"a\\\\*a", "a\\a", true},
	{"a\\\\*a", "aba", false},
	{"a\\\\*a", "a\\ba", true},
}

// Check each of the elements in the specificCases slice.
func TestSpecificCases(t *testing.T) {
	for i, v := range specificCases {
		doesMatch, err := regexp.MatchString(ToRegexp(v.Pattern), v.Input)
		if err != nil {
			t.Errorf("regexp.MatchString: %v", err)
		} else if doesMatch && !v.ShouldMatch {
			t.Errorf("Case %2d (Fail): Input %q should not have matched pattern %q, but did.",
				i, v.Input, v.Pattern)
		} else if !doesMatch && v.ShouldMatch {
			t.Errorf("Case %2d (Fail): Input %q should have matched pattern %q, but did not.",
				i, v.Input, v.Pattern)
		}
	}
}
