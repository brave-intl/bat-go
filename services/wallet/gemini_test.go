package wallet

import (
	"testing"

	"github.com/brave-intl/bat-go/libs/clients/gemini"
	should "github.com/stretchr/testify/assert"
)

func Test_getIssuingCountry(t *testing.T) {
	type tcGiven struct {
		gx       *geminix
		validAcc gemini.ValidatedAccount
		fallback bool
	}

	tests := []struct {
		name     string
		given    tcGiven
		expected string
	}{
		{
			name: "has_prior_linking_no_valid_documents",
			given: tcGiven{
				gx: newGeminix("passport"),
				validAcc: gemini.ValidatedAccount{
					CountryCode: "US",
				},
				fallback: true,
			},
			expected: "US",
		},
		{
			name: "has_prior_linking_and_valid_documents",
			given: tcGiven{
				gx: newGeminix("passport"),
				validAcc: gemini.ValidatedAccount{
					CountryCode: "US",
					ValidDocuments: []gemini.ValidDocument{
						{
							Type:           "passport",
							IssuingCountry: "PT",
						},
					},
				},
				fallback: true,
			},
			expected: "PT",
		},
		{
			name: "has_no_prior_linking_and_no_valid_documents",
			given: tcGiven{
				gx: newGeminix("passport"),
				validAcc: gemini.ValidatedAccount{
					CountryCode: "US",
				},
				fallback: false,
			},
			expected: "",
		},
		{
			name: "has_no_prior_linking_and_valid_documents",
			given: tcGiven{
				gx: newGeminix("passport"),
				validAcc: gemini.ValidatedAccount{
					CountryCode: "US",
					ValidDocuments: []gemini.ValidDocument{
						{
							Type:           "passport",
							IssuingCountry: "PT",
						},
					},
				},
				fallback: false,
			},
			expected: "PT",
		},
		{
			name: "has_prior_linking_and_no_country_code_and_no_valid_documents",
			given: tcGiven{
				gx:       newGeminix("passport"),
				validAcc: gemini.ValidatedAccount{},
				fallback: false,
			},
			expected: "",
		},
		{
			name: "has_prior_linking_and_no_country_code_and_valid_documents",
			given: tcGiven{
				gx: newGeminix("passport"),
				validAcc: gemini.ValidatedAccount{
					ValidDocuments: []gemini.ValidDocument{
						{
							Type:           "passport",
							IssuingCountry: "PT",
						},
					},
				},
				fallback: true,
			},
			expected: "PT",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.gx.GetIssuingCountry(tc.given.validAcc, tc.given.fallback)
			should.Equal(t, tc.expected, actual)
		})
	}
}

func Test_countryForDocByPrecedence(t *testing.T) {
	type testCase struct {
		name  string
		given []gemini.ValidDocument
		exp   string
	}

	tests := []testCase{
		{
			name: "empty",
		},

		{
			name: "one_passport",
			given: []gemini.ValidDocument{
				{
					Type:           "passport",
					IssuingCountry: "US",
				},
			},
			exp: "US",
		},

		{
			name: "two_docs",
			given: []gemini.ValidDocument{
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
			given: []gemini.ValidDocument{
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
			given: []gemini.ValidDocument{
				{
					Type:           "invalid_type",
					IssuingCountry: "US",
				},
			},
			exp: "",
		},

		{
			name: "valid_and_invalid_document_type_lower_case",
			given: []gemini.ValidDocument{
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
			pres := []string{
				"passport",
				"drivers_license",
				"national_identity_card",
				"passport_card",
			}
			act := countryForDocByPrecedence(pres, tc.given)
			should.Equal(t, tc.exp, act)
		})
	}
}
