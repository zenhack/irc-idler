package netextra

import "net"

var Direct = direct{}

// This is the same as the Dialer interface from "golang.org/x/net/proxy".
// the Dial method has the same semantics as net.Dial from the standard
// library.
//
// We duplicate this here, as well as the "Direct" dialer, rather than
// pull in an extra dependency from which we use so little.
type Dialer interface {
	Dial(network, addr string) (c net.Conn, err error)
}

type direct struct{}

func (d direct) Dial(network, addr string) (c net.Conn, err error) {
	return net.Dial(network, addr)
}
