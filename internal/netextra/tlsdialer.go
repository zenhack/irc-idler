package netextra

import (
	"crypto/tls"
	"net"
)

// A TLSDialer speaks TLS over the `Base` Dialer, verifying the hostname
// it is passed.
type TLSDialer struct {
	Base Dialer
}

// Dial invokes d.Base.Dial, and then establishes a TLS session over the
// resulting connection. If any error occurs during the handshake, the
// connection will be closed and the error will be returned to the caller.
func (d *TLSDialer) Dial(network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	cfg := &tls.Config{
		ServerName: host,
	}
	if err != nil {
		return nil, err
	}
	conn, err := d.Base.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	tlsConn := tls.Client(conn, cfg)
	err = tlsConn.Handshake()
	if err != nil {
		conn.Close()
		return nil, err
	}
	return tlsConn, nil
}
