#!/bin/sh

set -eu

self="$(realpath "$0")"
scripts="${self%/*}"
repo="${scripts%/*/*}"

"$scripts/ensure-secretes.sh"

cd "$repo/tools/payments"

exec ipython3 --profile-dir=ipython-profile
