package gemini

import (
	"testing"

	should "github.com/stretchr/testify/assert"
)

func TestCountryForDocByPrecedence(t *testing.T) {
	type testCase struct {
		name  string
		given []ValidDocument
		exp   string
	}

	tests := []testCase{
		{
			name: "empty",
		},

		{
			name: "one_passport",
			given: []ValidDocument{
				{
					Type:           "passport",
					IssuingCountry: "US",
				},
			},
			exp: "US",
		},

		{
			name: "two_docs",
			given: []ValidDocument{
				{
					Type:           "passport",
					IssuingCountry: "US",
				},

				{
					Type:           "drivers_license",
					IssuingCountry: "CA",
				},
			},
			exp: "US",
		},

		{
			name: "two_docs_reverse",
			given: []ValidDocument{
				{
					Type:           "drivers_license",
					IssuingCountry: "CA",
				},

				{
					Type:           "passport",
					IssuingCountry: "US",
				},
			},
			exp: "US",
		},

		{
			name: "no_valid_document_type",
			given: []ValidDocument{
				{
					Type:           "invalid_type",
					IssuingCountry: "US",
				},
			},
			exp: "",
		},

		{
			name: "valid_and_invalid_document_type_lower_case",
			given: []ValidDocument{
				{
					Type:           "invalid_type",
					IssuingCountry: "US",
				},
				{
					Type:           "passport",
					IssuingCountry: "uk",
				},
			},
			exp: "UK",
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			act := countryForDocByPrecedence(documentTypePrecedence, tc.given)
			should.Equal(t, tc.exp, act)
		})
	}
}
