#!/usr/bin/env sh
#
# Strip the sandstorm-irc-idler executable and build an spk

strip "$(dirname $0)/.sandstorm/gopath/bin/sandstorm-irc-idler"
vagrant-spk pack irc-idler-$(date -Isecond).spk
