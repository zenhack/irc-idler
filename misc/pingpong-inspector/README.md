This directory contains a tool `pingpong-inspector` that makes
experimenting with a raw IRC connection a little easier, by
managing PINGs and PONGs itself. It otherwise just shuttles
IRC messages between it's stdio and it's peer. It can act as
either a client or a server.

# Usage

To act as a client:

    ./pingpong-inspector -raddr irc.example.net:6667

To act as a server:

    ./pingpong-inspector -laddr :6667

In the server case, you'll see a message "connection received" when the
client connects.

In either case, you can then experiment by typing in raw IRC messages,
just as you would with netcat or telnet, but you will never need to
respond to PINGs, which can make things a bit easier.

Note that the server will only accept one connection at a time.
