
# Brave SKUs Service

### Getting Started

1. Begin with development setup steps 1 through 4 from the [bat-go readme](https://github.com/brave-intl/bat-go/blob/master/README.md)

2. Bring up the SKUs containers locally with ```make docker-refresh-payment```

3. View container logs with ```docker logs grant-payment-refresh```   

4. SKUs API will be available at localhost:3335

5. Commit code and refresh the containers with ```docker restart grant-payment-refresh```!

### SKU Tokens 

SKU Tokens represent cookie-like objects with domain specific caveats, new tokens can be created following the instructions in [this readme](https://github.com/brave-intl/bat-go/tree/master/cmd#create-a-macaroon)

This is an example of one possible [SKU token](https://github.com/brave-intl/bat-go/blob/brave-together-dev/cmd/macaroon/brave-together/brave_together_paid_dev.yaml): 

```
tokens:
  - id: "brave-together-paid sku token v1"
    version: 1
    location: "together.bsg.brave.software"
    first_party_caveats:
      - sku: "brave-together-paid"
      - price: 5
      - currency: "USD"
      - description: "One month paid subscription for Brave Together"
      - credential_type: "time-limited"
      - credential_valid_duration: "P1M"
      - payment_methods: ["stripe"]
```

### Creating a Free Trial 

Given the knowledge of a free trial SKU unlimited numbers of trials can be created.  Care must be taken to keep free trial SKUs secret.

Certain SKU tokens do not have a price, and once users have created an order for them they may redeem credentials to access the related service. In this example, we will create a free trial order for Brave Talk: 

Construct a `POST` request to ```/v1/orders``` with the following metadata

```
{
	"items": [{
		"sku": "MDAyOWxvY2F0aW9uIHRvZ2V0aGVyLmJzZy5icmF2ZS5zb2Z0d2FyZQowMDMwaWRlbnRpZmllciBicmF2ZS10b2dldGhlci1mcmVlIHNrdSB0b2tlbiB2MQowMDIwY2lkIHNrdT1icmF2ZS10b2dldGhlci1mcmVlCjAwMTBjaWQgcHJpY2U9MAowMDE1Y2lkIGN1cnJlbmN5PUJBVAowMDNjY2lkIGRlc2NyaXB0aW9uPU9uZSBtb250aCBmcmVlIHRyaWFsIGZvciBCcmF2ZSBUb2dldGhlcgowMDI1Y2lkIGNyZWRlbnRpYWxfdHlwZT10aW1lLWxpbWl0ZWQKMDAyNmNpZCBjcmVkZW50aWFsX3ZhbGlkX2R1cmF0aW9uPVAxTQowMDJmc2lnbmF0dXJlIGebBXoPnj06tvlJkPEDLp9nfWo6Wfc1Txj6jTlgxjrQCg==",
		"quantity": 1
	}]
}
```

Upon GET request to ```/v1/orders/:orderId```, server will respond with:

```
{
	"id": "92aafa4b-da7e-46b1-99c1-e8d30b26bc0f",
	"createdAt": "2021-04-12T00:11:51.954386Z",
	"currency": "BAT",
	"updatedAt": "2021-04-12T00:11:51.954386Z",
	"totalPrice": "0",
	"merchantId": "brave.com",
	"location": "together.bsg.brave.software",
	"status": "paid",
	"items": [{
		"id": "0b573a13-3c3d-40b7-bdac-879c531d31fb",
		"orderId": "92aafa4b-da7e-46b1-99c1-e8d30b26bc0f",
		"sku": "brave-together-free",
		"createdAt": "2021-04-12T00:11:51.954386Z",
		"updatedAt": "2021-04-12T00:11:51.954386Z",
		"currency": "BAT",
		"quantity": 1,
		"price": "0",
		"subtotal": "0",
		"location": "together.bsg.brave.software",
		"description": "One month free trial for Brave Together",
		"type": "time-limited"
	}]
}
```

Because the status on this order is _paid_ we may create and request credentials 

Construct a `POST` request to ```/v1/orders/:orderId/credentials``` with the following metadata

```
{
	"itemId": <itemId>,
	"blindedCreds": [<Base64 Encoded Blinded Credentials>]
}
```

Server will respond with status 200 OK. To retrieve these credentials construct a `GET` request to ```/v1/orders/:orderId/credentials```

Server will respond with the following payload:

```
[
  {
    "id": "0b573a13-3c3d-40b7-bdac-879c531d31fb",
    "orderId": "92aafa4b-da7e-46b1-99c1-e8d30b26bc0f",
    "issuedAt": "2021-04-12",
    "expiresAt": "2021-05-17",
    "token": "ZCtG5A8lvArgJtBOR4I4tfHmDsM+pBrb9STaa1k1qbOhGHaYO2HFA2MUvoJ9edGX"
  }
]
```

### Creating a Paid Order

Similarly to a free order, we will submit a creation request, however we will include an email which will be needed for management of subscriptions

Construct a `POST` request to ```/v1/orders``` with the following metadata

```
{"items": [
{"sku": "MDAyOWxvY2F0aW9uIHRvZ2V0aGVyLmJzZy5icmF2ZS5zb2Z0d2FyZQowMDMwaWRlbnRpZmllciBicmF2ZS10b2dldGhlci1wYWlkIHNrdSB0b2tlbiB2MQowMDIwY2lkIHNrdT1icmF2ZS10b2dldGhlci1wYWlkCjAwMTBjaWQgcHJpY2U9NQowMDE1Y2lkIGN1cnJlbmN5PVVTRAowMDQzY2lkIGRlc2NyaXB0aW9uPU9uZSBtb250aCBwYWlkIHN1YnNjcmlwdGlvbiBmb3IgQnJhdmUgVG9nZXRoZXIKMDAyNWNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMU0KMDAyZnNpZ25hdHVyZSDKLJ7NuuzP3KdmTdVnn0dI3JmIfNblQKmY+WBJOqnQJAo=", "quantity": 1}],
 "email": "customeremail@gmail.com"
}
```

Upon `GET` request to ```/v1/orders/:orderId```, server will respond with: 

```
{
	"id": "89ded3d9-55e1-4e26-bc18-228e76cf03ca",
	"createdAt": "2021-04-12T00:51:44.61976Z",
	"currency": "USD",
	"updatedAt": "2021-04-12T00:51:45.515351Z",
	"totalPrice": "5",
	"merchantId": "brave.com",
	"location": "together.bsg.brave.software",
	"status": "pending",
	"items": [{
		"id": "355ae321-0ceb-4698-9fed-158190da6fa4",
		"orderId": "89ded3d9-55e1-4e26-bc18-228e76cf03ca",
		"sku": "brave-together-paid",
		"createdAt": "2021-04-12T00:51:44.61976Z",
		"updatedAt": "2021-04-12T00:51:44.61976Z",
		"currency": "USD",
		"quantity": 1,
		"price": "5",
		"subtotal": "5",
		"location": "together.bsg.brave.software",
		"description": "One month paid subscription for Brave Together",
		"type": "time-limited"
	}],
	"stripeCheckoutSessionId": "cs_test_a1g0n8FWNT3ClB2p9shKlpchGTw7cDKQCOqLJ1dSBcRd9ZsLssrtDLbZgM"
}
```

Because this order is Stripe payable, we will receive a Stripe Checkout Session Id, this identifier can be used to fulfill the order via a credit card. For example if we construct the following sample script: 

```html
<html>

<head>
    <title>Buy cool new product</title>
    <script src="https://js.stripe.com/v3/"></script>
</head>

<body>
    <button id="checkout-button">Checkout</button>
    <script type="text/javascript">
        // Create an instance of the Stripe object with your publishable API key
        var stripe = Stripe('pk_test_51HlmudHof20bphG64WGsYJhhJ3OsgZw5DyVx5mM7PdW4K7gLJS5KoMiA624HEJIWXImp0DEj7nKg2x8l7nGT6zhk00dtatBliN');
        var checkoutButton = document.getElementById('checkout-button');
        checkoutButton.addEventListener('click', function () {
            // Create a new Checkout Session using the server-side endpoint you
            // created in step 3.
            return stripe.redirectToCheckout({ sessionId: 'cs_test_a1g0n8FWNT3ClB2p9shKlpchGTw7cDKQCOqLJ1dSBcRd9ZsLssrtDLbZgM' });
        });
    </script>
</body>

</html>
```

And host locally, we can redirect to a page similar to this: 

<img width="1226" alt="Screen Shot 2021-04-11 at 9 18 10 PM" src="https://user-images.githubusercontent.com/4713771/114328725-6d8f8580-9b0b-11eb-8827-1cbb575381a8.png">

A credit card can then be entered, and upon successful payment, the corresponding order will become paid. Once paid, credentials may be created and requested as in the free trial. 

### Architecture 

Documentation above refers to a general use of the payment service. In the context of Brave Talk, the order of various calls and the service making the call is outlined in the diagram below: 

<img width="1065" alt="Screen Shot 2021-04-30 at 1 58 22 PM" src="https://user-images.githubusercontent.com/4713771/116735903-2afbf300-a9bd-11eb-97fc-a384cece6346.png">


### Stripe Integration 

For the stripe integration details refer to the diagram below with numbered interactions.

[stripe integration diagram](docs/StripeSKUsIntegration.pdf)

### Submitting a Receipt

In some circumstances it is desirable to submit a receipt for a particular order to prove it was
paid, and have the skus service handle the validation of said receipt.  Currently implemented
there are two receipt providers of which SKUs is capable of validating payment was collected,
android and ios.

```
curl -XPOST /v1/orders/<order_id>/submit-receipt -d '<base64 encoded json payload>'
```

The payload of the above call is a Base64 encoded string of a JSON document.  Two examples follow:

```json
{
    "type": "ios",
    "raw_receipt": "<vendor specific receipt string>",
    "package": "com.brave...", // android specific,
    "subscription_id": "brave-firewall-vpn-premium", // the sku string value of the subscription
}
```

The and example POST payload of the API call to submit receipt is the above json base64 encoded. 
### Submitting a Receipt

In some circumstances it is desirable to submit a receipt for a particular order to prove it was
paid, and have the skus service handle the validation of said receipt.  Currently implemented
there are two receipt providers of which SKUs is capable of validating payment was collected,
android and ios.

```
curl -XPOST /v1/orders/<order_id>/submit-receipt -d '<base64 encoded json payload>'
```

The payload of the above call is a Base64 encoded string of a JSON document.  Two examples follow:

```json
{
    "type": "ios",
    "raw_receipt": "<vendor specific receipt string>",
    "package": "com.brave...", // android specific,
    "subscription_id": "brave-firewall-vpn-premium", // the sku string value of the subscription
}
```

The and example POST payload of the API call to submit receipt is the above json base64 encoded. 
