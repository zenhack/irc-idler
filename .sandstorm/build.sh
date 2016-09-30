#!/bin/bash
set -euo pipefail

export GOPATH="/opt/app/.sandstorm/gopath"

_srcdir="$GOPATH/src/zenhack.net/go/irc-idler"
[ -L "$_srcdir" ] || {
       mkdir -p $(dirname $_srcdir) || true
       cd $(dirname _srcdir)
       # Make a symlink back to /opt/app. We do this as
       # a relative path so we can play with it on the
       # host machine more easily.
       ln -s ../../../../..  $_srcdir
}

# To pull in the dependencies:
go get -d zenhack.net/go/irc-idler/cmd/sandstorm-irc-idler

cd $GOPATH/src/zenhack.net/go/irc-idler/cmd/sandstorm-irc-idler
go install -v

exit 0
