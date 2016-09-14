// Testing utilities for implementations of Store
package testing

import (
	"fmt"
	"io"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"
	"zenhack.net/go/irc-idler/irc"
	"zenhack.net/go/irc-idler/storage"

	// for casting reflect.Values to their actual values:
	"unsafe"
)

// A randomized black-box test for store implementations. It does the
// following:
//
// * Insert random values into the logs
// * Verify that reading back those values succeeds
// * Clear the logs and verify that they are actually empty.
//
// If any of the checks are unsuccessful, RandTest calls t.Fatal
//
// The function newStore should return a new (empty) store to test.
func RandTest(t *testing.T, newStore func() storage.Store) {
	genValues := func(values []reflect.Value, r *rand.Rand) {
		for i := range values {
			values[i] = genStore(r)
		}
	}
	cfg := &quick.Config{Values: genValues}
	if err := quick.Check(checkFunc(newStore), cfg); err != nil {
		t.Fatal(err)
	}
}

func checkFunc(newStore func() storage.Store) func(m map[string][]*irc.Message) bool {
	return func(m map[string][]*irc.Message) bool {
		store := newStore()
		fillStore(m, store)
		if !checkFilled(m, store) {
			return false
		}
		return checkClear(m, store)
	}
}

func fillStore(m map[string][]*irc.Message, store storage.Store) {
	for k, v := range m {
		log, _ := store.GetChannel(k)
		for _, msg := range v {
			log.LogMessage(msg)
		}
	}
}

func checkClear(m map[string][]*irc.Message, store storage.Store) bool {
	for k, _ := range m {
		log, _ := store.GetChannel(k)
		log.Clear()
		cursor, _ := log.Replay()
		msg, err := cursor.Get()
		if err != io.EOF {
			fmt.Printf("Clear() did not clear the log for channel %q; "+
				"Get() expected EOF but got (%q, %q)", k, msg, err)
			cursor.Close()
			return false
		}
		cursor.Close()
	}
	return true
}

func checkFilled(m map[string][]*irc.Message, store storage.Store) bool {
	for k, v := range m {
		log, err := store.GetChannel(k)
		if err != nil {
			panic(fmt.Sprintf("Getting channel %q: %q", k, err))
		}
		cursor, err := log.Replay()
		if err != nil {
			panic(fmt.Sprintf("Getting log for channel %q: %q", k, err))
		}
		for i, msg := range v {
			loggedMsg, err := cursor.Get()
			if err != nil {
				fmt.Printf("Unexpected error getting log entry %d for channel %q: %q\n",
					i, k, err)
				cursor.Close()
				return false
			}
			oldStr := loggedMsg.String()
			newStr := msg.String()
			if oldStr != newStr {
				fmt.Printf(
					"Mismatch at position %d in log for channel %q: "+
						"expected %q but got %q.\n", i, k, newStr, oldStr)
				cursor.Close()
				return false
			}
			cursor.Next()
		}
		msg, err := cursor.Get()
		if err == nil {
			fmt.Printf("Unexpected message at position %d in log for channel "+
				"%q: expected EOF but got %q.\n", len(v), k, msg.String())
			cursor.Close()
			return false
		}
		cursor.Close()
	}
	return true
}

func genStore(r *rand.Rand) reflect.Value {
	numChannels := int(r.Uint32() % 50)
	ret := make(map[string][]*irc.Message)

	for i := 0; i < numChannels; i++ {
		value, _ := quick.Value(reflect.TypeOf(""), r)
		buf := []byte(value.String())
		if len(buf) > 16 { // relatively arbitrary cap
			buf = buf[:16]
		}
		ret[fmt.Sprintf("%x", buf)] = genChannel(r)
	}
	return reflect.ValueOf(ret)
}

func genChannel(r *rand.Rand) []*irc.Message {
	numMessages := int(r.Float64() * 70)
	ret := make([]*irc.Message, numMessages)
	for i := range ret {
		value := ret[i].Generate(r, 0)
		ret[i] = (*irc.Message)(unsafe.Pointer(value.Pointer()))
	}
	return ret
}
