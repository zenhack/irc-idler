This directory contains a tool `pingpong-inspector` that makes
experimenting with a raw IRC connection a little easier, by
managing PINGs and PONGs itself. It otherwise just shuttles
IRC messages between it's stdio and the server.

Usage:

    ./pingpong-inspector -raddr irc.example.net:6667

Then, type raw IRC messages. You can experiment just as you would with
`telnet`, but you will never need to respond to PINGs, which can make
things a bit easier.
