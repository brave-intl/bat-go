# Wallet Microservice

## Purpose

To provide BAT-Wallet API.

### to run

go run ../main.go serve wallet rest

## Custodian Linking Toggle

We have the ability to turn off/on individually custodial linking capabilities
for rewards wallets.  The following response can be expected for each of the
custodian linking endpoints if we have administratively turned off linking capabilities.

### Uphold Linking Toggle

```
POST /v3/wallet/uphold/<payment_id>/claim
...

HTTP2/0 400 Bad Request
{
    "message": "Error validating Connecting Brave Rewards to Uphold is temporarily unavailable.  Please try again later",
    "code": 400,
    "data": {
        "validationErrors": null
    }
}
```

### Gemini Linking Toggle

```
POST /v3/wallet/gemini/<payment_id>/claim
...

HTTP2/0 400 Bad Request
{
    "message": "Error validating Connecting Brave Rewards to Gemini is temporarily unavailable.  Please try again later",
    "code": 400,
    "data": {
        "validationErrors": null
    }
}
```

### Bitflyer Linking Toggle

```
POST /v3/wallet/bitflyer/<payment_id>/claim
...

HTTP2/0 400 Bad Request
{
    "message": "Error validating Connecting Brave Rewards to Bitflyer is temporarily unavailable.  Please try again later",
    "code": 400,
    "data": {
        "validationErrors": null
    }
}
```
