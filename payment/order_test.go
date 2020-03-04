package payment

import "testing"

func TestCreateOrderItemFromMacaroon(t *testing.T) {
	sku := "MDAxZmxvY2F0aW9uIGh0dHBzOi8vYnJhdmUuY29tCjAwMWFpZGVudGlmaWVyIGlkZW50aWZpZXIKMDAyNGNpZCBleHBpcnkgPSAiMjAyMC0wNC0wMVQwMDowMCIKMDAxOWNpZCBjdXJyZW5jeSA9ICJCQVQiCjAwMzRjaWQgaWQgPSAiNWM4NDZkYTEtODNjZC00ZTE1LTk4ZGQtOGUxNDdhNTZiNmZhIgowMDJkY2lkIGRlc2NyaXB0aW9uID0gImNvZmZlZSBjb2ZmZWUgY29mZmVlIgowMDE3Y2lkIHByaWNlID0gIjIuMDAiCjAwMmZzaWduYXR1cmUgNYqjg0ZCIva_7Qj6p55jyPaanErBvuC9HN4AKfy-StQK"
	// identifier: identifier
	// rootKey: secret
	// location: brave.com
	// caveats:
	//     id = "5c846da1-83cd-4e15-98dd-8e147a56b6fa"
	//     currency = "BAT"
	//     price = "0.25"
	//     description = "coffee coffee coffee"
	//     expiry = "2020-04-01"

	orderItem, err := CreateOrderItemFromMacaroon(sku, 1)
	if err != nil {
		t.Error("Error creating macaroon")
	}

	if orderItem.Currency != "BAT" {
		t.Error("Expected currency to be BAT")
	}
}
