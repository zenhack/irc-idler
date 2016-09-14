#!/bin/bash

# When you change this file, you must take manual action. Read this doc:
# - https://docs.sandstorm.io/en/latest/vagrant-spk/customizing/#setupsh


# The version of golang in the main repo is *ancient* (1.3.x); let's get
# ourselves a newer version:

echo 'deb http://httpredir.debian.org/debian/ jessie-backports main' >> \
	/etc/apt/sources.list.d/backports.list
apt-get update
apt-get install -y git
apt-get -t jessie-backports install -y golang

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

set -euo pipefail
exit 0
