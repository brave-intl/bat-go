package skus

import (
	"testing"
)

func TestDeduplicateCredentialBindings(t *testing.T) {

	var tokens = []CredentialBinding{
		{
			TokenPreimage: "totally_random",
		},
		{
			TokenPreimage: "totally_random_1",
		},
		{
			TokenPreimage: "totally_random",
		},
		{
			TokenPreimage: "totally_random_2",
		},
	}
	var seen = []CredentialBinding{}

	var result = DeduplicateCredentialBindings(tokens...)
	if len(result) > len(tokens) {
		t.Error("result should be less than number of tokens")
	}

	for _, v := range result {
		for _, vv := range seen {
			if v == vv {
				t.Error("Deduplication of tokens didn't work")
			}
			seen = append(seen, v)
		}
	}
}

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
			t.Errorf(
				"merchantID does not match decoded: %s != %s", v.MerchantID, merchantIDPrime)
		}

		if v.SKU != skuPrime {
			t.Errorf(
				"sku does not match decoded: %s != %s", v.SKU, skuPrime)
		}
	}
}
