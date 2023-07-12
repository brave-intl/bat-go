#!/bin/bash

while read line
do
    read -r issuerid privkey <<< $( echo $line | awk -F',' '{print $1 " " $2}' )
    pubkey=$(echo -n "${privkey}" | base64 -d | openssl pkey -pubout -outform DER | tail -c +13 | openssl base64)
    echo "${issuerid},${privkey},${pubkey}"
done<"${1:-/dev/stdin}"
