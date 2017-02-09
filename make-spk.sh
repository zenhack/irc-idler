#!/usr/bin/env sh
set -ex
cd "$(dirname $0)"

cd cmd/sandstorm-irc-idler
go build -i -v
strip sandstorm-irc-idler
cd -
spk pack irc-idler-$(git describe).spk
