#!/bin/bash
set -euo pipefail

export GOPATH="/opt/app/.sandstorm/gopath"

# To pull in the dependencies:
go get -d zenhack.net/go/irc-idler/cmd/sandstorm-irc-idler

cd $GOPATH/src/zenhack.net/go/irc-idler/cmd/sandstorm-irc-idler
go install -v

exit 0
