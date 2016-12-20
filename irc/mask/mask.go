// Package mask handles the irc mask syntax specified in RFC 2812, section 2.5.
package mask

import (
	"bytes"
	"regexp"
)

// Convert the mask to a regexp pattern matching the same set of strings, as
// recognized by go's regexp package.
func ToRegexp(mask string) string {
	rePat := &bytes.Buffer{}
	nextChunk := &bytes.Buffer{}
	input := []byte(mask)

	flush := func() {
		if nextChunk.Len() != 0 {
			rePat.WriteString(regexp.QuoteMeta(nextChunk.String()))
			nextChunk.Reset()
		}
	}
	inEscape := false
	for _, c := range input {
		if inEscape {
			nextChunk.WriteByte(c)
			inEscape = false
			continue
		}
		switch c {
		case '?':
			flush()
			rePat.WriteByte('.')
		case '*':
			flush()
			rePat.WriteString(".*")
		case '\\':
			inEscape = true
		default:
			nextChunk.WriteByte(c)
		}
	}
	flush()
	return rePat.String()
}
