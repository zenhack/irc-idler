Our IRC implementation is based mostly on [RFC 2812][rfc], but the
behavior of software out in the wild sometimes differs, and the RFC is a
valiant, but not always successful attempt at documenting that behavior.
This document indexes the various workarounds we've had to implement in
order to deal with other software that doesn't follow the spec
perfectly, as well general inconsistencies and things that are poorly
described and how we deal with them.

# USER/NICK ordering.

Section 3.1 describes the process of registering a new connection.
Clients SHOULD send a NICK message followed by a USER message, but some
clients (notably pidgin) swap these. We therefore accept these in either
order.

# RPL_WELCOME client identifier

Section 3.1 specifies that the server's RPL_WELCOME message includes the
full client identifier. The term "client identifier" is not explicitly
defined in this section, and does not appear anywhere else in the
document. However, section 5.1 shows an example RPL_WELCOME message:

   001    RPL_WELCOME
      "Welcome to the Internet Relay Network
       <nick>!<user>@<host>"

Which leads one to believe that a client identifier is
`<nick>!<user>@<host>`. This looks roughly like one of the variants of
the "prefix" production in section 2.3.1, so that's the syntax we
accept. Both OFTC and freenode return just the `<nick>` portion, but
since this is allowed according to that

[rfc]: https://www.rfc-editor.org/rfc/rfc2812.txt

# RPL_NAM{,E}REPLY

The spec refers to the numeric reply code 353 by two different names
RPL_NAMREPLY and RPL_NAMEREPLY. We use the latter.
