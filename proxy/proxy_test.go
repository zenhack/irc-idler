package proxy

import (
	"testing"
	"time"
)

func TestConnectDisconnect(t *testing.T) {
	state := StartTestProxy()
	err := Expect(state, time.Second)
	if err != nil {
		t.Fatal(err)
	}
}
