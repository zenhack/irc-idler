#!/usr/bin/env sh
#
# Recompile sandstorm-irc-idler and build an spk

set -ex
pushd $(dirname $0)/cmd/sandstorm-irc-idler
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build
strip sandstorm-irc-idler
popd
cd $(dirname $0)
vagrant-spk pack irc-idler-$(date -Isecond).spk
