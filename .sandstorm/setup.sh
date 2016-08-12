#!/bin/bash

# When you change this file, you must take manual action. Read this doc:
# - https://docs.sandstorm.io/en/latest/vagrant-spk/customizing/#setupsh

set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y golang git

export GOPATH="/opt/app/.sandstorm/gopath"
_srcdir="$GOPATH/src/zenhack.net/go/irc-idler"
[ -L "$_srcdir" ] || {
	mkdir -p $(dirname $_srcdir) || true
	ln -s /opt/app  $_srcdir
}
exit 0
