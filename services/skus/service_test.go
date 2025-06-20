package skus

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/brave-intl/bat-go/libs/datastore"
	"github.com/brave-intl/bat-go/libs/ptr"
	timeutils "github.com/brave-intl/bat-go/libs/time"
	"github.com/brave-intl/bat-go/services/skus/model"
)

func TestCredChunkFn(t *testing.T) {
	// Jan 1, 2021
	issued := time.Date(2021, time.January, 20, 0, 0, 0, 0, time.UTC)

	// 1 day
	day, err := timeutils.ParseDuration("P1D")
	if err != nil {
		t.Errorf("failed to parse 1 day: %s", err.Error())
	}

	// 1 month
	mo, err := timeutils.ParseDuration("P1M")
	if err != nil {
		t.Errorf("failed to parse 1 month: %s", err.Error())
	}

	this, next := credChunkFn(*day)(issued)
	if this.Day() != 20 {
		t.Errorf("day - the next day should be 2")
	}
	if this.Month() != 1 {
		t.Errorf("day - the next month should be 1")
	}
	if next.Day() != 21 {
		t.Errorf("day - the next day should be 2")
	}
	if next.Month() != 1 {
		t.Errorf("day - the next month should be 1")
	}

	this, next = credChunkFn(*mo)(issued)
	if this.Day() != 1 {
		t.Errorf("mo - the next day should be 1")
	}
	if this.Month() != 1 {
		t.Errorf("mo - the next month should be 2")
	}
	if next.Day() != 1 {
		t.Errorf("mo - the next day should be 1")
	}
	if next.Month() != 2 {
		t.Errorf("mo - the next month should be 2")
	}
}

func TestCreateOrderItems(t *testing.T) {
	type tcExpected struct {
		result []model.OrderItem
		err    error
	}

	type testCase struct {
		name  string
		given *model.CreateOrderRequestNew
		exp   tcExpected
	}

	tests := []testCase{
		{
			name:  "empty",
			given: &model.CreateOrderRequestNew{},
			exp: tcExpected{
				result: []model.OrderItem{},
			},
		},

		{
			name: "invalid_CredentialValidDurationEach",
			given: &model.CreateOrderRequestNew{
				Items: []model.OrderItemRequestNew{
					{
						CredentialValidDuration:     "P1M",
						CredentialValidDurationEach: ptr.To("rubbish"),
					},
				},
			},
			exp: tcExpected{
				err: timeutils.ErrUnsupportedFormat,
			},
		},

		{
			name: "ensure_currency_set",
			given: &model.CreateOrderRequestNew{
				Currency: "USD",
				Items: []model.OrderItemRequestNew{
					{
						Location:                "location",
						Description:             "description",
						CredentialValidDuration: "P1M",
					},

					{
						Location:                "location",
						Description:             "description",
						CredentialValidDuration: "P1M",
					},
				},
			},
			exp: tcExpected{
				result: []model.OrderItem{
					{
						Currency:    "USD",
						ValidForISO: ptr.To("P1M"),
						Location: datastore.NullString{
							NullString: sql.NullString{
								Valid:  true,
								String: "location",
							},
						},
						Description: datastore.NullString{
							NullString: sql.NullString{
								Valid:  true,
								String: "description",
							},
						},
						IssuerConfig: &model.IssuerConfig{
							Buffer: 1,
						},
						Subtotal: decimal.NewFromInt(0),
					},

					{
						Currency:    "USD",
						ValidForISO: ptr.To("P1M"),
						Location: datastore.NullString{
							NullString: sql.NullString{
								Valid:  true,
								String: "location",
							},
						},
						Description: datastore.NullString{
							NullString: sql.NullString{
								Valid:  true,
								String: "description",
							},
						},

						IssuerConfig: &model.IssuerConfig{
							Buffer: 1,
						},
						Subtotal: decimal.NewFromInt(0),
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			act, err := createOrderItems(tc.given)
			must.Equal(t, true, errors.Is(err, tc.exp.err))

			if tc.exp.err != nil {
				return
			}

			must.Equal(t, len(tc.exp.result), len(act))

			// Override ValidFor because it's not deterministic.
			for j := range act {
				tc.exp.result[j].ValidFor = act[j].ValidFor
			}

			should.Equal(t, tc.exp.result, act)
		})
	}
}

func TestCreateOrderItem(t *testing.T) {
	type tcExpected struct {
		result *model.OrderItem
		err    error
	}

	type testCase struct {
		name  string
		given *model.OrderItemRequestNew
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "invalid_CredentialValidDurationEach",
			given: &model.OrderItemRequestNew{
				CredentialValidDurationEach: ptr.To("rubbish"),
			},
			exp: tcExpected{
				err: timeutils.ErrUnsupportedFormat,
			},
		},

		{
			name: "invalid_CredentialValidDuration",
			given: &model.OrderItemRequestNew{
				CredentialValidDuration:     "rubbish",
				CredentialValidDurationEach: ptr.To("P1M"),
			},
			exp: tcExpected{
				err: timeutils.ErrUnsupportedFormat,
			},
		},

		{
			name: "full_example",
			given: &model.OrderItemRequestNew{
				SKU:                         "sku",
				SKUVnt:                      "sku_vnt",
				CredentialType:              "credential_type",
				CredentialValidDuration:     "P1M",
				CredentialValidDurationEach: ptr.To("P1D"),
				IssuanceInterval:            ptr.To("P1M"),
				Price:                       decimal.NewFromInt(10),
				Location:                    "location",
				Description:                 "description",
				Quantity:                    2,
				StripeMetadata: &model.ItemStripeMetadata{
					ProductID: "product_id",
					ItemID:    "item_id",
				},
			},
			exp: tcExpected{
				result: &model.OrderItem{
					SKU:                       "sku",
					SKUVnt:                    "sku_vnt",
					CredentialType:            "credential_type",
					ValidFor:                  mustDurationFromISO("P1M"),
					ValidForISO:               ptr.To("P1M"),
					EachCredentialValidForISO: ptr.To("P1D"),
					IssuanceIntervalISO:       ptr.To("P1M"),
					Price:                     decimal.NewFromInt(10),
					Location: datastore.NullString{
						NullString: sql.NullString{
							Valid:  true,
							String: "location",
						},
					},
					Description: datastore.NullString{
						NullString: sql.NullString{
							Valid:  true,
							String: "description",
						},
					},
					Quantity: 2,
					Metadata: map[string]interface{}{
						"stripe_product_id": "product_id",
						"stripe_item_id":    "item_id",
					},
					Subtotal: decimal.NewFromInt(10).Mul(decimal.NewFromInt(int64(2))),
					IssuerConfig: &model.IssuerConfig{
						Buffer: 1,
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			act, err := createOrderItem(tc.given)
			must.Equal(t, true, errors.Is(err, tc.exp.err))

			if tc.exp.err != nil {
				return
			}

			// Override ValidFor because it's not deterministic.
			tc.exp.result.ValidFor = act.ValidFor

			should.Equal(t, tc.exp.result, act)
		})
	}
}

func TestFilterActiveCreds(t *testing.T) {
	type tcGiven struct {
		creds []TimeAwareSubIssuedCreds
		now   time.Time
	}

	type tcExpected struct {
		activeCreds []TimeAwareSubIssuedCreds
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "valid_creds",
			given: tcGiven{
				creds: []TimeAwareSubIssuedCreds{
					{
						ValidTo: time.Date(2026, time.January, 20, 0, 0, 0, 0, time.UTC),
					},
				},
				now: time.Date(2025, time.January, 20, 0, 0, 0, 0, time.UTC),
			},
			exp: tcExpected{
				activeCreds: []TimeAwareSubIssuedCreds{
					{
						ValidTo: time.Date(2026, time.January, 20, 0, 0, 0, 0, time.UTC),
					},
				},
			},
		},

		{
			name: "expired_creds",
			given: tcGiven{
				creds: []TimeAwareSubIssuedCreds{
					{
						ValidTo: time.Date(2020, time.January, 20, 0, 0, 0, 0, time.UTC),
					},
				},
				now: time.Date(2021, time.January, 20, 0, 0, 0, 0, time.UTC),
			},
			exp: tcExpected{
				activeCreds: []TimeAwareSubIssuedCreds{},
			},
		},

		{
			name: "expired_and_active_mix",
			given: tcGiven{
				creds: []TimeAwareSubIssuedCreds{
					{
						ValidTo: time.Date(2020, time.January, 20, 0, 0, 0, 0, time.UTC),
					},

					{
						ValidTo: time.Date(2026, time.January, 20, 0, 0, 0, 0, time.UTC),
					},
				},
				now: time.Date(2025, time.January, 20, 0, 0, 0, 0, time.UTC),
			},
			exp: tcExpected{
				activeCreds: []TimeAwareSubIssuedCreds{
					{
						ValidTo: time.Date(2026, time.January, 20, 0, 0, 0, 0, time.UTC),
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := filterActiveCreds(tc.given.creds, tc.given.now)
			should.Equal(t, tc.exp.activeCreds, actual)
		})
	}
}

func mustDurationFromISO(v string) *time.Duration {
	result, err := durationFromISO(v)
	if err != nil {
		panic(err)
	}

	return &result
}

func TestTimeChunking(t *testing.T) {
	type tcGiven struct {
		issuerID string
		secret   cryptography.TimeLimitedSecret
		ord      *model.Order
		item     *model.OrderItem
		duration string
		interval string
		start    time.Time
	}

	type tcExpected struct {
		numCreds int
		err      error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "search_monthly",
			given: tcGiven{
				issuerID: "monthly",
				secret:   cryptography.NewTimeLimitedSecret([]byte("tester")),
				ord: &model.Order{
					ID:         uuid.FromStringOrNil("107a26ef-847d-4040-a95f-d34857c8c5bd"),
					LastPaidAt: ptrTo(time.Date(2025, time.June, 1, 0, 0, 0, 0, time.UTC)),
				},
				item: &model.OrderItem{
					ID: uuid.FromStringOrNil("bfc3e99e-cf85-4985-b16d-3959c37b1722"),
				},
				duration: "P1M",
				interval: "P1M",
				start:    time.Date(2025, time.June, 1, 0, 0, 0, 0, time.UTC),
			},
			exp: tcExpected{
				numCreds: 2,
			},
		},

		{
			name: "talk_monthly",
			given: tcGiven{
				issuerID: "monthly",
				secret:   cryptography.NewTimeLimitedSecret([]byte("tester")),
				ord: &model.Order{
					ID:         uuid.FromStringOrNil("107a26ef-847d-4040-a95f-d34857c8c5bd"),
					LastPaidAt: ptrTo(time.Date(2025, time.June, 1, 0, 0, 0, 0, time.UTC)),
				},
				item: &model.OrderItem{
					ID: uuid.FromStringOrNil("bfc3e99e-cf85-4985-b16d-3959c37b1722"),
				},
				duration: "P1M",
				interval: "P1D",
				start:    time.Date(2025, time.June, 1, 0, 0, 0, 0, time.UTC),
			},
			exp: tcExpected{
				numCreds: 35,
			},
		},

		{
			name: "annual_expires_at_is_nil",
			given: tcGiven{
				issuerID: "annual_search",
				secret:   cryptography.NewTimeLimitedSecret([]byte("tester")),
				ord: &model.Order{
					ID:         uuid.FromStringOrNil("107a26ef-847d-4040-a95f-d34857c8c5bd"),
					LastPaidAt: ptrTo(time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)),
				},
				item: &model.OrderItem{
					ID:     uuid.FromStringOrNil("bfc3e99e-cf85-4985-b16d-3959c37b1722"),
					SKUVnt: "brave-search-premium-year",
				},
				duration: "P1M",
				interval: "P1M",
			},
			exp: tcExpected{
				err: model.Error("skus: time chunking: order expires at cannot be nil"),
			},
		},

		{
			name: "search_annual",
			given: tcGiven{
				issuerID: "annual_search",
				secret:   cryptography.NewTimeLimitedSecret([]byte("tester")),
				ord: &model.Order{
					ID:         uuid.FromStringOrNil("107a26ef-847d-4040-a95f-d34857c8c5bd"),
					LastPaidAt: ptrTo(time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)),
					ExpiresAt:  ptrTo(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)),
				},
				item: &model.OrderItem{
					ID:     uuid.FromStringOrNil("bfc3e99e-cf85-4985-b16d-3959c37b1722"),
					SKUVnt: "brave-search-premium-year",
				},
				duration: "P1M",
				interval: "P1M",
				start:    time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
			},
			exp: tcExpected{
				numCreds: 13,
			},
		},

		{
			name: "search_annual_three_months_used",
			given: tcGiven{
				issuerID: "annual_search",
				secret:   cryptography.NewTimeLimitedSecret([]byte("tester")),
				ord: &model.Order{
					ID:         uuid.FromStringOrNil("107a26ef-847d-4040-a95f-d34857c8c5bd"),
					LastPaidAt: ptrTo(time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)),
					ExpiresAt:  ptrTo(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)),
				},
				item: &model.OrderItem{
					ID:     uuid.FromStringOrNil("bfc3e99e-cf85-4985-b16d-3959c37b1722"),
					SKUVnt: "brave-search-premium-year",
				},
				duration: "P1M",
				interval: "P1M",
				start:    time.Date(2025, time.April, 1, 0, 0, 0, 0, time.UTC),
			},
			exp: tcExpected{
				numCreds: 10,
			},
		},

		{
			name: "annual_talk",
			given: tcGiven{
				issuerID: "annual_talk",
				secret:   cryptography.NewTimeLimitedSecret([]byte("tester")),
				ord: &model.Order{
					ID:         uuid.FromStringOrNil("107a26ef-847d-4040-a95f-d34857c8c5bd"),
					LastPaidAt: ptrTo(time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)),
					ExpiresAt:  ptrTo(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)),
				},
				item: &model.OrderItem{
					ID:     uuid.FromStringOrNil("bfc3e99e-cf85-4985-b16d-3959c37b1722"),
					SKUVnt: "brave-talk-premium-year",
				},
				duration: "P1M",
				interval: "P1D",
				start:    time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
			},
			exp: tcExpected{
				numCreds: 370,
			},
		},

		{
			name: "annual_talk_ten_months_used",
			given: tcGiven{
				issuerID: "annual_talk",
				secret:   cryptography.NewTimeLimitedSecret([]byte("tester")),
				ord: &model.Order{
					ID:         uuid.FromStringOrNil("107a26ef-847d-4040-a95f-d34857c8c5bd"),
					LastPaidAt: ptrTo(time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)),
					ExpiresAt:  ptrTo(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)),
				},
				item: &model.OrderItem{
					ID:     uuid.FromStringOrNil("bfc3e99e-cf85-4985-b16d-3959c37b1722"),
					SKUVnt: "brave-talk-premium-year",
				},
				duration: "P1M",
				interval: "P1D",
				start:    time.Date(2025, time.December, 1, 0, 0, 0, 0, time.UTC), //36
			},
			exp: tcExpected{
				numCreds: 36,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			duration, err := timeutils.ParseDuration(tc.given.duration)
			must.NoError(t, err)

			issuanceInterval, err := timeutils.ParseDuration(tc.given.interval)
			must.NoError(t, err)

			actual, err := timeChunking(ctx, tc.given.issuerID, tc.given.secret, tc.given.ord, tc.given.item, *duration, *issuanceInterval, tc.given.start)
			should.Equal(t, tc.exp.numCreds, len(actual))
			should.Equal(t, tc.exp.err, err)
		})
	}
}
