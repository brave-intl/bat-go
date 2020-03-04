package payment

import "testing"

func TestCreateOrderItemFromMacaroon(t *testing.T) {
	sku := ""
	orderItem, err := CreateOrderItemFromMacaroon(sku, 1)
	if(err != nil) {
		t.Error("Error creating macaroon")
	}

	if(orderItem.Currency != "BAT") {
		t.Error("Expected currency to be BAT")
	}
}
