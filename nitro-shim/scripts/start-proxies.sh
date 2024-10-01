#!/bin/bash

set -eux

service="${1}"
CID="${2}"
PARENT_CID="3" # the CID of the EC2 instance

echo "cid is ${CID}"
# at this point the enclave is up.  depending on what service we're running,
# it's now time to set up proxy tools
if [ "${service}" = "/payments" ]; then
    # setup inbound traffic proxy
    export IN_ADDRS=":8080"
    export OUT_ADDRS="${CID}:8080"
    echo "${IN_ADDRS} to ${OUT_ADDRS}"
    # next startup the proxy
    /enclave/viproxy > /tmp/viproxy.log &
elif [ "${service}" = "/star-randsrv" ]; then
    domain_socket="/tmp/network.sock"
    /enclave/gvproxy \
        -listen "vsock://:1024" \
        -listen "unix://${domain_socket}" &
    # give gvproxy a second to start
    sleep 1
    # run vsock relay to proxy incoming requests
    /enclave/vsock-relay \
        -s "0.0.0.0:443,0.0.0.0:9443,0.0.0.0:9090" \
        -d "4:443,4:9443,4:9090" \
        -c 1000 \
        --host-ip-provider-port 6161 &
fi
