package cbr

import "testing"

func TestDeduplicateCredentialRedemptions(t *testing.T) {

	var tokens = []CredentialRedemption{
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
	var seen = []CredentialRedemption{}

	var result = DeduplicateCredentialRedemptions(tokens...)
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
