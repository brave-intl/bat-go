package payment

import (
	"fmt"
	"testing"
)

func TestIssuerID(t *testing.T) {

	cases := []struct {
		MerchantID string
		SKU        string
	}{
		{
			MerchantID: "brave.com",
			SKU:        "anon-card-vote",
		},
		{
			MerchantID: "",
			SKU:        "anon-card-vote",
		},
		{
			MerchantID: "brave.com",
			SKU:        "",
		},
		{
			MerchantID: "",
			SKU:        "",
		},
	}

	for _, v := range cases {

		issuerID, err := encodeIssuerID(v.MerchantID, v.SKU)
		if err != nil {
			t.Error("failed to encode: ", err)
		}

		merchantIDPrime, skuPrime, err := decodeIssuerID(issuerID)
		if err != nil {
			t.Error("failed to encode: ", err)
		}

		if v.MerchantID != merchantIDPrime {
			t.Error(
				fmt.Sprintf("merchantID does not match decoded: %s != %s", v.MerchantID, merchantIDPrime))
		}

		if v.SKU != skuPrime {
			t.Error(
				fmt.Sprintf("sku does not match decoded: %s != %s", v.SKU, skuPrime))
		}
	}
}
