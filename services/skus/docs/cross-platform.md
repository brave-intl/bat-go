# Cross Platform Receipt Validation

To allow for in-app-purchases the SKUs Service herein allows for submission and
verification of vendor specific proof of purchases.  The receipt submission API
consumes a base64 encoded request body, and returns the vendor specific external
identifier if the embedded `raw_receipt` is valid.

## Receipt Submission API

The body structure of the receipt submission is a base64 encoded json payload as
shown in the two examples.  Within the `skus-sdk` there is a `submit-receipt` method
which will take this base64 encoded json payload and perform a submission to the SKUs
API.  The un-encoded json payload for the receipt submission API is shown below.

```json
{
    "type":[android|ios],
    "raw_receipt":"[raw receipt payload from vendor]",
    "package":"[android package name of project]", // android only required
    "subscription_id":"[android subscription name purchased]" // android only required
}
```

### Possible Responses

- 400 - failed to decode id: id is not a uuid // the order uuid in url is not a valid uuid
- 400 - failed to decode id: id cannot be empty // the order uuid in url is empty
- 400 - failed to decode input base64 // the request data is not base64 encoded
- 400 - failed to decode input json // the b64 decoded payload sent is not json
- 400 - failed to validate structure // the payload json is not well formed
- 400 - failed to validate vendor // vendor in payload json is not ios or android
- 400 - purchase is still pending // vendor says this purchase is still pending, not paid yet
- 400 - purchase is deferred // vendor says this purchase is deferred
- 400 - purchase status unknown // unknown purchase status from vendor
- 400 - failed to verify subscription // vendor error verifying this subscription
- 400 - misconfigured client // issue with configuration on server
- 404 - order not found // the order does not exist
- 500 - failed to store status of order

### Example Android Submission

```bash
curl -XPOST https://payment.rewards.brave.software/v1/orders/submit-receipt \
-d'joiYW5kcm9pZCIsInJhd19yZWNla.......' -D -

HTTP2 200
Content-Type: application/json
...
{
   "externalId": "[receipt-data]",
   "vendor": "android",
}
```

## Webhook API

In order to get updated subscription information for various orders pushed from android we have
created a webhook which takes in the payload structure defined
[here](https://developer.android.com/google/play/billing/rtdn-reference)
and then perform a new validation on the existing receipt as per recommendations from the android
developer documentation.  The receipt "externalId" metadata is stored on the order in order
for us to lookup which order the notification is referring to in order to validate the status
of said order and reset the internal status accordingly.

The webhook routes are:

```
/v1/webhooks/android
```
