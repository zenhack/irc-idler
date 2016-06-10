Our IRC implementation is based mostly on [RFC 2812][rfc], but the
behavior of software out in the wild sometimes differs, and the RFC is a
valiant, but not always successful attempt at documenting that behavior.
This document indexes the various workarounds we've had to implement in
order to deal with other software that doesn't follow the spec
perfectly.

# USER/NICK ordering.

Section 3.1 describes the process of registering a new connection.
Clients SHOULD send a NICK message followed by a USER message, but some
clients (notably pidgin) swap these. We therefore accept these in either
order.


[rfc]: https://www.rfc-editor.org/rfc/rfc2812.txt
