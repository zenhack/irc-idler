#!/bin/bash
set -euo pipefail

export GOPATH="/opt/app/.sandstorm/gopath"

deps='
	golang.org/x/net/context
	zenhack.net/go/sandstorm/grain
'

for d in $deps; do
	go get $d
done

cd /opt/app/cmd/irc-idler-sandstorm
go build

[ -x /opt/app/cmd/irc-idler-sandstorm/irc-idler-sandstorm ] || {
	echo 'irc-idler-sandstorm executable not found!' >&2
	echo 'You must build it via `go build`; vagrant-spk' >&2
        echo 'will not do it for you' >&2
	exit 1
}
exit 0
