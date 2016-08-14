package irc

import (
	"bytes"
	"encoding/base64"
	"math/rand"
	"reflect"
)

// implementation of testing/quick.Generator for *Message.
//
// The returned messages aren't entirely valid; they follow rules
// about presence/absence of prefixes and commands, message length,
// and number of arguments, but the values of prefixes, arguments, and
// commands are just random strings. This may be refined in the future.
func (msg *Message) Generate(r *rand.Rand, size int) reflect.Value {
	return reflect.ValueOf(genMessage(r))
}

func genMessage(r *rand.Rand) *Message {
	spaceLeft := MaxMessageLen - 2 // for CLRF
	prefixLen := int(r.Float64() * 16)
	spaceLeft -= prefixLen
	if prefixLen != 0 {
		spaceLeft -= 2 // leading colon plus trailing space.
	}

	commandLen := int(r.Float64()*16) + 1
	spaceLeft -= commandLen

	numParams := int(r.Float64() * 16)

	// A space between each pair of args (and before the first one, to
	// separate it from the command), and a leading ':' for the last arg.
	spaceLeft -= numParams + 1

	params := []string{}

	for i := numParams; i > 0; i-- {
		paramLen := int(r.Float64()*float64(spaceLeft/i)) + 1
		if paramLen == 0 {
			continue
		}
		params = append(params, genBase64(paramLen, r))
		spaceLeft -= paramLen
	}

	return &Message{
		Prefix:  genBase64(prefixLen, r),
		Command: genBase64(commandLen, r),
		Params:  params,
	}
}

// generate a random base64 string of the given length.
func genBase64(length int, r *rand.Rand) string {
	buf := &bytes.Buffer{}
	b64 := base64.NewEncoder(base64.StdEncoding, buf)

	// Using base64 reduces the information content of a byte; you
	// have 64 (2^5) possible values insead of 256 (2^8) possible
	// values. So we reduce the length of the random binary buffer
	// accordingly:
	randBytes := make([]byte, int(float64(length)*(5/8)))

	r.Read(randBytes)
	b64.Write(randBytes)
	b64.Close()

	return buf.String()
}
