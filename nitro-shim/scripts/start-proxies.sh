#!/bin/bash

service="${1}"
CID="${2}"
PARENT_CID="3" # the CID of the EC2 instance

echo "cid is ${CID}"
# at this point the enclave is up.  depending on what service we're running,
# it's now time to set up proxy tools
if [ "${service}" = "/payments" ]; then
    # setup inbound traffic proxy
    export IN_ADDRS=":8080,:8443"
    export OUT_ADDRS="${CID}:8080,${CID}:8443"
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
    # instruct gvproxy to forward port 443 to the enclave
    curl \
        -X POST \
        --unix-socket "$domain_socket" \
        -d '{"local":":443","remote":"192.168.127.2:443"}' \
        "http:/unix/services/forwarder/expose"
fi
