[![Travis CI Build Status][ci-img]][ci]

IRC Idler is a program which idles in IRC for you. [Sandstorm][1] will
be the preferred way of running it, though it will work in traditional
environments as well.

This is very much a work in progress. I'm currently dogfooding the
sandstorm version, but it's not exactly polished.

# Why

Lots of folks prefer to be persistently online on IRC. A common
solution to this is to be logged in via a console IRC client on a server
somewhere, running in tmux or GNU screen. This works, but is less than
ideal.

# What

IRC Idler connects to the IRC server for you, and then acts as an IRC
server itself -- you connect to IRC Idler, and it proxies the
connection. When you disconnect, it stays connected, and flags you as
away until you reconnect, at which point it replays an messages you
missed while you were gone.

## Sandstorm Design Notes

IRC isn't a web-app so building a sandstorm app that offers it is
slightly more complicated. We still want leverage sandstorm for
authentication and authorization. We do this by listening on a websocket
instead of a raw TCP port, and have users use [websocket-proxy][2] to
connect. This scheme also translates decently to the non-sandstorm case.

On sandstorm, each IRC connection runs in its own grain. The websocket
trick means we don't need to allocate a separate port to each network.

# Building

The sandstorm version is in `cmd/sandstorm-irc-idler`, the non-sandstorm
version is in `cmd/irc-idler`. Either executable can be built via
standard go build.

Note on the sandstorm build: The vagrant-spk boilerplate doesn't
actually compile anything; you have to build the executable on the host
machine. If you're developing on a platform other than linux/amd64, you
can build via:

    GOOS=linux GOARCH=amd64 go build

The reasons for this are twofold:

1. Cross compiling Go is really easy.
2. The version of go available in the standard vagrant-spk vm is very
   old (1.3.x), and I'd rather not be limited to what was available
   then.

The script `./run-spk-dev.sh` will recompile the sandstorm app and then
run `vagrant-spk dev`.

# Using (sandstorm)

To use the sandstorm version, you must be an administrator for your
sandstorm installation. This is because IRC Idler requires raw network
access, which only an administrator can grant.

Each irc network you want to connect to must run in its own grain. To
set up a new network:

* Create a new IRC Idler grain
* Fill out the settings for the IRC server on IRC Idler's web
  interface. For example, to connect to freenode, you would supply:
  * Host: irc.freenode.net
  * Port: 6667 for unencrypted, 6697 for TLS
  * Check the TLS box or not, depending on whether you want to use it
    (recommended).
* Click on the "Request Network Access" button, and grant network access
  in the dialog that sandstorm presents
* You will be presented with a websocket URL you can use to connect. You
  can get a traditional IRC client to connect to this by using
  [websocket-proxy][2]:

      websocket-proxy -listen :6000 -url ${websocket_url}

...and then pointing your IRC client at localhost port 6000.

# Using (non-sandstorm)

As an example, to connect to Freenode via TLS:

    ./irc-idler -tls -raddr irc.freenode.net:6697 -laddr :6667

Then, point your irc client at port 6667 on the host running irc-idler.

Note well: irc-idler does not support accepting client connections via
TLS, and it preforms no authentication. As a consequence, you should run
it on a trusted network. One solution is to have it only listening on
localhost on the server that's running it (and have port 6667 firewalled
off for good measure), and use ssh port forwarding to connect from your
laptop/desktop.

This will hopefully be more streamlined in the future; one possibility
is to make the websocket solution used by the sandstorm version
available for the non-sandstorm version as well.

# License

    Copyright (C) 2016  Ian Denhardt <ian@zenhack.net>

    This program is free software: you can redistribute it and/or modify
    it under the terms of the GNU General Public License as published by
    the Free Software Foundation, either version 3 of the License, or
    (at your option) any later version.

    This program is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
    GNU General Public License for more details.

    You should have received a copy of the GNU General Public License
    along with this program.  If not, see <http://www.gnu.org/licenses/>.

(See COPYING for a copy of the license).

[1]: https://sandstorm.io
[2]: https://github.com/zenhack/websocket-proxy
[3]: https://github.com/zenhack/go.sandstorm
[ci-img]: https://api.travis-ci.org/zenhack/irc-idler.svg?branch=master
[ci]: https://travis-ci.org/zenhack/irc-idler
