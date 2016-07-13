#!/bin/bash
set -euo pipefail

[ -x /opt/app/cmd/irc-idler-sandstorm/irc-idler-sandstorm ] || {
	echo 'irc-idler-sandstorm executable not found!' >&2
	echo 'You must build it via `go build`; vagrant-spk' >&2
        echo 'will not do it for you' >&2
	exit 1
}
exit 0
