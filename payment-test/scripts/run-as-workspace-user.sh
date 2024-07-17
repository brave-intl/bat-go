#!/bin/sh

# Helper to sync the container user with the id of the owner of workspace and
# then run the command as that user.

set -eu

workspace_uid_gid="$(stat -c %u:%g /workspace)"
uid="${workspace_uid_gid%:*}"
gid="${workspace_uid_gid#*:}"

h="/workspace/payment-test/container-home"

if ! test -h /home/user; then
    # The user home is not a symlink so the user in the container not yet
    # adjusted.
    groupmod -g "$gid" user
    usermod -u "$uid" user

    # Updating files inside /workspace may race with other containers. So to
    # copy skeleton first copy it to a temporeary location and then move
    # atomically.
    if ! test -d "$h"; then
        echo "Creating $h" >&2
        chown -R user:user /home/user
        tmp="$(mktemp -u -p "${h%/*}")"
        cp -a /home/user "$tmp" || { rm -rf "$tmp"; exit 1; }
        mv "$tmp" "$h" || { rm -rf "$tmp"; exit 1; }
    fi
    rm -rf /home/user
    ln -s "$h" /home/user
fi

# We want to keep the current environmnet for the subprocess so we do not use
# --reset-env with setprov. Rather we just fixup few variables using env.
exec setpriv  --init-groups --regid "$gid" --reuid "$uid" --no-new-privs \
    env HOME="$h" SHELL=/usr/bin/bash USER=user LOGNAME=user \
        PATH=/usr/local/bin:/usr/bin "$@"
