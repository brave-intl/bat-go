#!/bin/sh

# Helper to run a command with its arguments until the timestamp of its
# executable changes.

set -eu

target="$1"

test -x "$1" || {
    echo "The argument $1 is not an executable" >&2
    exit 1
}

monitor() {
    local modification_time t
    modification_time="$(stat -c "%Y" "$target")"
    while :; do
        sleep 1
        t="$(stat -c "%Y" "$target")"
        if test "$t" -ne "$modification_time"; then
            echo "Newer $target is detected, restarting" >&2
            kill "$$"
            sleep 0.5
            kill -9 "$$"
        fi
    done
}

monitor &

bar="======================================================================"
t="$(date '+%Y-%m-%d %H:%M:%S')"
printf '%s\n[%s] Running\n[%s] %s\n%s\n' "$bar" "$t" "$t" "$*" "$bar" >&2

exec "$@"
