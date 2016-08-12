#!/bin/bash
set -euo pipefail

export GOPATH="/opt/app/.sandstorm/gopath"

deps='
	golang.org/x/net/context
	zenhack.net/go/sandstorm/grain
'

for d in $deps; do
	[ -e $GOPATH/src/$d ] || go get $d
done

cd /opt/app/cmd/irc-idler-sandstorm
go build
exit 0
