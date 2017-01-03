package netextra

import (
	"crypto/tls"
	"net"
)

// Dialer that speaks TLS over the `Base` Dialer, verifying the hostname
// it is passed.
type TLSDialer struct {
	Base Dialer
}

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
