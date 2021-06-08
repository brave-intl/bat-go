### Drain Info Endpoint Operation

You are able to lookup the status of a wallet's drains by performing this API call
and including an environment specific simple secret access token.

```
curl -H"Authorization: Bearer <token>" "http://<host>/v1/promotions/custodian-drain-info/<payment id>"
HTTP/1.1 200 OK
Connection: close
Content-Type: application/json

{
  "status": "success",
  "drains": [
    {
      "batch_id": "ca72d2e2-14b2-44da-b8e2-e5fce7f87263",
      "custodian": {
        "provider": "bitflyer",
        "deposit_destination": "f2b3cc8a-597d-4eeb-a2f9-23f65dbd2495"
      },
      "promotions_drained": [
        {
          "promotion_id": "daf95421-4388-4c7e-9ac3-4b476f8a5c79",
          "state": "errored",
          "errcode": "reputation-failed",
          "value": "0.25"
        }
      ],
      "value": "0.25"
    }
  ]
}
```
