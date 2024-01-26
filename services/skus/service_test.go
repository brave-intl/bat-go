//go:build integration

package skus

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/ptr"
	"github.com/shopspring/decimal"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/libs/datastore"
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
							Buffer:  30,
							Overlap: 5,
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
							Buffer:  30,
							Overlap: 5,
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
				IssuerTokenBuffer: 10,
			},
			exp: tcExpected{
				result: &model.OrderItem{
					SKU:                       "sku",
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
						Buffer:  10,
						Overlap: 5,
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
