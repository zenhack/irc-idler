package netextra

import (
	"io"
	"io/ioutil"
	"net"
	"testing"
	"time"
)

type pipeDialer struct {
	otherSide func(conn net.Conn)
}

func (d pipeDialer) Dial(network, addr string) (net.Conn, error) {
	c1, c2 := net.Pipe()
	go d.otherSide(c1)
	return c2, nil
}

// Verify that TLSDialer closes the underlying connection if a handshake fails.
func TestHandshakeErrorClosesConn(t *testing.T) {
	done := make(chan struct{})
	tlsDialer := &TLSDialer{&pipeDialer{func(conn net.Conn) {
		go io.Copy(ioutil.Discard, conn)
		buf := make([]byte, 4096)
		err := error(nil)
		for err == nil {
			_, err = conn.Write(buf)
		}
		done <- struct{}{}
	}}}
	_, err := tlsDialer.Dial("tcp", "example.net:443")
	if err == nil {
		t.Fatal("Dial should have failed, but returned no error.")
	} else {
		t.Logf("Got an error from the dialer (as desired): %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Connection was not closed.")
	}
}
