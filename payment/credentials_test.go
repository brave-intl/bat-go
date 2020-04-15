package payment

import (
	"fmt"
	"testing"
)

func TestIssuerID(t *testing.T) {
	merchantID := "brave.com"
	sku := "anon-card-vote"

	issuerID, err := encodeIssuerID(merchantID, sku)
	if err != nil {
		t.Error("failed to encode: ", err)
	}

	fmt.Println("issuerID: ", issuerID)

	merchantIDPrime, skuPrime, err := decodeIssuerID(issuerID)
	if err != nil {
		t.Error("failed to encode: ", err)
	}

	if merchantID != merchantIDPrime {
		t.Error(
			fmt.Sprintf("merchantID does not match decoded: %s != %s", merchantID, merchantIDPrime))
	}

	if sku != skuPrime {
		t.Error(
			fmt.Sprintf("sku does not match decoded: %s != %s", sku, skuPrime))
	}

}
