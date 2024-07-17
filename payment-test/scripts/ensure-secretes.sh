#!/bin/sh

set -eu

self="$(realpath "$0")"
secrets="${self%/*/*}/secrets"

test -d "$secrets" || {
    echo "Creating $secrets directory" >&2
    mkdir -m 0700 "$secrets"
}

f="$secrets/payments-test.json"
test -s "$f" || {
    echo "The file with configuration keys $f does not exist or empty, please obtain it"
    exit 1
}

ensureEd25519Key() {
    local name pem pub x
    name="$1"

    # We need to generate ED25519 key in PEM format and its public key in
    # OpenSSH format. Unfortunately ssh-keygen released only in 2024 supports
    # that. So use other tools.

    pem="$secrets/$name.pem"
    test -s "$pem" || {
        echo "ED25519 private key file $pem does not exist, generating it" >&2
        x="$(command -v openssl 2>/dev/null || :)"
        test "$x" || {
            echo "openssl tool does not exist, please install it. On Debian-based system use:" >&2
            echo "    apt install openssl" >&2
            exit 1
        }
        rm -f "$pem.tmp"
        openssl genpkey -algorithm ed25519 > "$pem.tmp"
        mv "$pem.tmp" "$pem"
    }
    pub="${pem%.pem}.pub"
    test -s "$pub" || {
        echo "ED25519 public key file $pub does not exist, producing it from" >&2
        x="$(command -v sshpk-conv 2>/dev/null || :)"
        test "$x" || {
            echo "sshpk-conv utility does not exist, please install it. On Debian-based system use:" >&2
            echo "    apt install node-sshpk" >&2
            exit 1
        }
        sshpk-conv -T pem -t ssh -f "$pem" -o "$pub" -c "$name"
    }
}

ensureEd25519Key test-operator
ensureEd25519Key test-operator2
