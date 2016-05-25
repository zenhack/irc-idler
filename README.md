IRC idler is a (very, very WIP) program which idles in IRC
for you. [sandstorm][1] will be the preferred way of running it,
though it will work in traditional environments as well.

# Why

Lot's of folks prefer to be persistently online on IRC. A common
solution to this is to be logged in via a console IRC client on a server
somewhere, running in tmux or GNU screen. This works, but is less than
ideal.

# What

IRC idler connects to the IRC server for you, and then acts as an IRC
server itself -- you connect to IRC idler, and it proxies the
connection. When you disconnect, it stays connected, and flags you as
away until you reconnect.

## Design Ideas

IRC isn't a web-app so building a sandstorm app that offers it is
slightly more complicated. We'd like to still leverage sandstorm for
authentication and authorization. One idea for how to do this is to
listen on a websocket instead of a raw TCP port, and have users use
[websocket-proxy][2] to connect. This scheme also translates decently
to the non-sandstorm scheme.

On sandstorm, the plan is to have each IRC connection run in its own
grain. The websocket trick means we don't need to allocate a separate
port to each network.

# Building

The sandstorm version is in <cmd/irc-idler-sandstorm>, the non-sandstorm
version is in <cmd/irc-idler>. Either executable can be built via standard go
build.

Notes on the sandstorm build:

* The sandstorm capnproto wrappers require some manual work to build,
  though this is temporary, see the [README][3] in that repository for
  details.
* This repo doesn't have the usual vagrant-spk boilerplate; to work on
  the sandstorm version, you'll need a box with sandstorm set up in
  development mode, on which you can run `spk dev` directly.

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
