package xstripe

import (
	"testing"

	should "github.com/stretchr/testify/assert"
	"github.com/stripe/stripe-go/v72"
)

func TestCustomerEmailFromSession(t *testing.T) {
	tests := []struct {
		name  string
		exp   string
		given *stripe.CheckoutSession
	}{
		{
			name:  "nil_customer_no_email",
			given: &stripe.CheckoutSession{},
		},

		{
			name: "customer_empty_email_no_email",
			given: &stripe.CheckoutSession{
				Customer: &stripe.Customer{},
			},
		},

		{
			name: "customer_empty_email_email",
			given: &stripe.CheckoutSession{
				Customer:      &stripe.Customer{},
				CustomerEmail: "you@example.com",
			},
			exp: "you@example.com",
		},

		{
			name: "customer_no_email",
			given: &stripe.CheckoutSession{
				Customer: &stripe.Customer{
					Email: "me@example.com",
				},
			},
			exp: "me@example.com",
		},

		{
			name: "customer_email",
			given: &stripe.CheckoutSession{
				Customer: &stripe.Customer{
					Email: "me@example.com",
				},
				CustomerEmail: "you@example.com",
			},
			exp: "me@example.com",
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := CustomerEmailFromSession(tc.given)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestCustomerIDFromSession(t *testing.T) {
	tests := []struct {
		name  string
		exp   string
		given *stripe.CheckoutSession
	}{
		{
			name:  "nil_customer_no_email",
			given: &stripe.CheckoutSession{},
		},

		{
			name: "customer_empty_email",
			given: &stripe.CheckoutSession{
				Customer: &stripe.Customer{},
			},
		},

		{
			name: "customer_email_no_id",
			given: &stripe.CheckoutSession{
				Customer: &stripe.Customer{
					Email: "me@example.com",
				},
			},
		},

		{
			name: "customer_email_id",
			given: &stripe.CheckoutSession{
				Customer: &stripe.Customer{
					ID:    "cus_id",
					Email: "me@example.com",
				},
			},
			exp: "cus_id",
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := CustomerIDFromSession(tc.given)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestLocaleValidator_IsLocaleSupported(t *testing.T) {
	type testCase struct {
		name  string
		given string
		exp   bool
	}

	tests := []testCase{
		{
			name: "invalid_empty",
		},

		{
			name:  "auto",
			given: "auto",
			exp:   true,
		},

		{
			name:  "bulgarian",
			given: "bg",
			exp:   true,
		},

		{
			name:  "czech",
			given: "cs",
			exp:   true,
		},

		{
			name:  "danish",
			given: "da",
			exp:   true,
		},

		{
			name:  "german",
			given: "de",
			exp:   true,
		},

		{
			name:  "greek",
			given: "el",
			exp:   true,
		},

		{
			name:  "english",
			given: "en",
			exp:   true,
		},

		{
			name:  "english_united_kingdom",
			given: "en-GB",
			exp:   true,
		},

		{
			name:  "spanish",
			given: "es",
			exp:   true,
		},

		{
			name:  "spanish_latin_america",
			given: "es-419",
			exp:   true,
		},

		{
			name:  "estonian",
			given: "et",
			exp:   true,
		},

		{
			name:  "finnish",
			given: "fi",
			exp:   true,
		},

		{
			name:  "filipino",
			given: "fil",
			exp:   true,
		},

		{
			name:  "french",
			given: "fr",
			exp:   true,
		},

		{
			name:  "french_canada",
			given: "fr-CA",
			exp:   true,
		},

		{
			name:  "croatian",
			given: "hr",
			exp:   true,
		},

		{
			name:  "hungarian",
			given: "hu",
			exp:   true,
		},

		{
			name:  "hungarian",
			given: "id",
			exp:   true,
		},

		{
			name:  "italian",
			given: "it",
			exp:   true,
		},

		{
			name:  "japanese",
			given: "ja",
			exp:   true,
		},

		{
			name:  "korean",
			given: "ko",
			exp:   true,
		},

		{
			name:  "lithuanian",
			given: "lt",
			exp:   true,
		},

		{
			name:  "latvian",
			given: "lv",
			exp:   true,
		},

		{
			name:  "malay",
			given: "ms",
			exp:   true,
		},

		{
			name:  "maltese",
			given: "mt",
			exp:   true,
		},

		{
			name:  "norwegian",
			given: "nb",
			exp:   true,
		},

		{
			name:  "dutch",
			given: "nl",
			exp:   true,
		},

		{
			name:  "polish",
			given: "pl",
			exp:   true,
		},

		{
			name:  "portuguese_brazil",
			given: "pt-BR",
			exp:   true,
		},

		{
			name:  "portuguese",
			given: "pt",
			exp:   true,
		},

		{
			name:  "romanian",
			given: "ro",
			exp:   true,
		},

		{
			name:  "russian",
			given: "ru",
			exp:   true,
		},

		{
			name:  "slovak",
			given: "sk",
			exp:   true,
		},

		{
			name:  "slovenian",
			given: "sl",
			exp:   true,
		},

		{
			name:  "swedish",
			given: "sv",
			exp:   true,
		},

		{
			name:  "thai",
			given: "th",
			exp:   true,
		},

		{
			name:  "turkish",
			given: "tr",
			exp:   true,
		},

		{
			name:  "vietnamese",
			given: "vi",
			exp:   true,
		},

		{
			name:  "chinese_simplified",
			given: "zh",
			exp:   true,
		},

		{
			name:  "chinese_traditional_hong_kong",
			given: "zh-HK",
			exp:   true,
		},

		{
			name:  "chinese_traditional_taiwan",
			given: "zh-TW",
			exp:   true,
		},

		{
			name:  "invalid_numbers",
			given: "12345",
		},

		{
			name:  "invalid_letters",
			given: "uk",
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			slv := NewLocaleValidator()

			actual := slv.IsLocaleSupported(tc.given)
			should.Equal(t, tc.exp, actual)
		})
	}
}
