Kafka HTTP bridge
=================

This tool exposes an HTTP API that takes as input anonymized IP addresses
and their associated wallet IDs, which are then turned into Kafka messages and
subsequently sent to our Kafka cluster.  The tool effectively serves as an
HTTP-to-Kafka bridge by exposing two endpoints:

1. POST /addresses  
   This endpoint is called from within our AWS Nitro enclave.  The HTTP
   request's JSON body maps key IDs to UUIDs (which are wallet IDs), which map
   to a list of anonymized IP addresses that we have seen the wallets use:
   ```
   {
     "keyid": {
       "b12cf1a7281d65f7c736": {
         "addrs": {
            "00000000-0000-0000-0000-000000000000": ["1.1.1.1", "2.2.2.2"],
            "11111111-1111-1111-1111-111111111111": ["3.3.3.3"]
         }
       }
     }
   }
   ```

2. GET /status  
   This endpoint returns statistics about the number of requests that came from
   the enclave, the number of requests that were successfully forwarded to the
   Kafka broker, and the number of requests that we failed to forward to the
   Kafka broker.

We use this tool as part of our enclave-enabled IP address anonymization
project.

Architecture
------------

Incoming addresses are immediately forwarded to the Kafka broker, i.e., the
receiving and sending of anonymized addresses is tightly coupled.  We may have
to reconsider this if there are going to be performance issues.
