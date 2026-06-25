package skus

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/libs/datastore"
	"github.com/brave-intl/bat-go/services/skus/model"
	"github.com/brave-intl/bat-go/services/skus/storage/repository"
)

func TestCheckTLV2BatchLimit(t *testing.T) {
	type tcGiven struct {
		lim  int
		nact int
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   error
	}

	tests := []testCase{
		{
			name:  "both_zero",
			given: tcGiven{},
			exp:   ErrCredsAlreadyExist,
		},

		{
			name:  "under_limit",
			given: tcGiven{lim: 10, nact: 1},
		},

		{
			name:  "at_limit",
			given: tcGiven{lim: 10, nact: 10},
			exp:   ErrCredsAlreadyExist,
		},

		{
			name:  "above",
			given: tcGiven{lim: 10, nact: 11},
			exp:   ErrCredsAlreadyExist,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := checkTLV2BatchLimit(tc.given.lim, tc.given.nact)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestTruncateTLV2BCreds(t *testing.T) {
	type tcGiven struct {
		ord      *model.Order
		item     *model.OrderItem
		srcCreds []string
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   []string
	}

	tests := []testCase{
		{
			name: "no_trunc_not_leo",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Metadata: datastore.Metadata{
						"numIntervals":   int(33),
						"numPerInterval": int(2),
					},
				},
				item: &model.OrderItem{
					ID:      uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
					OrderID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					SKU:     "brave-vpn-premium",
					SKUVnt:  "brave-vpn-premium",
				},
				srcCreds: genNumStrings(2 * 33),
			},
			exp: genNumStrings(2 * 33),
		},

		{
			name: "no_trunc_leo_3_192",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Metadata: datastore.Metadata{
						"numIntervals":   int(3),
						"numPerInterval": int(192),
					},
				},
				item: &model.OrderItem{
					ID:      uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
					OrderID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					SKU:     "brave-leo-premium",
					SKUVnt:  "brave-leo-premium",
				},
				srcCreds: genNumStrings(3 * 192),
			},
			exp: genNumStrings(3 * 192),
		},

		{
			name: "trunc_leo_8_192",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Metadata: datastore.Metadata{
						"numIntervals":   int(8),
						"numPerInterval": int(192),
					},
				},
				item: &model.OrderItem{
					ID:      uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
					OrderID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					SKU:     "brave-leo-premium",
					SKUVnt:  "brave-leo-premium",
				},
				srcCreds: genNumStrings(8 * 192),
			},
			exp: genNumStrings(3 * 192),
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := truncateTLV2BCreds(tc.given.ord, tc.given.item, len(tc.given.srcCreds), tc.given.srcCreds)
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestShouldTruncateTLV2Creds(t *testing.T) {
	type tcGiven struct {
		ord    *model.Order
		item   *model.OrderItem
		ncreds int
	}

	type tcExpected struct {
		val int
		ok  bool
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "not_leo",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Metadata: datastore.Metadata{
						"numIntervals":   int(33),
						"numPerInterval": int(2),
					},
				},
				item: &model.OrderItem{
					ID:      uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
					OrderID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					SKU:     "brave-vpn-premium",
					SKUVnt:  "brave-vpn-premium",
				},
				ncreds: 2 * 33,
			},
			exp: tcExpected{},
		},

		{
			name: "false_3_192",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Metadata: datastore.Metadata{
						"numIntervals":   int(3),
						"numPerInterval": int(192),
					},
				},
				item: &model.OrderItem{
					ID:      uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
					OrderID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					SKU:     "brave-leo-premium",
					SKUVnt:  "brave-leo-premium",
				},
				ncreds: 3 * 192,
			},
			exp: tcExpected{},
		},

		{
			name: "true_8_192",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					Metadata: datastore.Metadata{
						"numIntervals":   int(8),
						"numPerInterval": int(192),
					},
				},
				item: &model.OrderItem{
					ID:      uuid.Must(uuid.FromString("f100ded0-0000-4000-a000-000000000000")),
					OrderID: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
					SKU:     "brave-leo-premium",
					SKUVnt:  "brave-leo-premium",
				},
				ncreds: 8 * 192,
			},
			exp: tcExpected{
				val: 3 * 192,
				ok:  true,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, ok := shouldTruncateTLV2Creds(tc.given.ord, tc.given.item, tc.given.ncreds)
			should.Equal(t, tc.exp.ok, ok)
			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestTlV2CredExtender_GetNextExtIfValid(t *testing.T) {
	type tcGiven struct {
		item     *model.OrderItem
		now      time.Time
		policies model.CredExtensionPolicies
		tlv2Repo tlv2Store
	}

	type tcExpected struct {
		nxt NextExtension
		err error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "error_unsupported_cred_type",
			given: tcGiven{
				item: &model.OrderItem{},
			},
			exp: tcExpected{
				err: model.ErrUnsupportedCredType,
			},
		},

		{
			name: "error_get_policy",
			given: tcGiven{
				item: &model.OrderItem{
					CredentialType: timeLimitedV2,
				},
			},
			exp: tcExpected{
				err: model.ErrNoExtensionPolicy,
			},
		},

		{
			name: "error_extension_max_per_item",
			given: tcGiven{
				item: &model.OrderItem{
					NumSelfExtensions: 1,
					SKUVnt:            "some_sku",
					CredentialType:    timeLimitedV2,
				},
				policies: model.CredExtensionPolicies{
					"some_sku": model.CredExtensionPolicy{
						MaxPerItem: 1,
					},
				},
			},
			exp: tcExpected{
				err: model.ErrExtensionMaxPerItem,
			},
		},

		{
			name: "error_check_next_allowed_extension_time",
			given: tcGiven{
				item: &model.OrderItem{
					NumSelfExtensions:   1,
					LastSelfExtensionAt: ptrTo(time.Date(2026, 06, 06, 0, 0, 0, 0, time.UTC)),
					SKUVnt:              "some_sku",
					CredentialType:      timeLimitedV2,
				},
				policies: model.CredExtensionPolicies{
					"some_sku": model.CredExtensionPolicy{
						MaxPerItem:         2,
						MinIntervalSeconds: 1,
					},
				},
			},
			exp: tcExpected{
				err: model.ErrExtensionRateLimited,
			},
		},

		{
			name: "error_unique_batches",
			given: tcGiven{
				item: &model.OrderItem{
					NumSelfExtensions: 1,
					SKUVnt:            "some_sku",
					CredentialType:    timeLimitedV2,
				},
				policies: model.CredExtensionPolicies{
					"some_sku": model.CredExtensionPolicy{
						MaxPerItem: 2,
					},
				},
				tlv2Repo: &repository.MockTLV2{
					FnUniqBatches: func(ctx context.Context, dbi sqlx.QueryerContext, orderID, itemID uuid.UUID, from, to time.Time) (int, error) {
						return 0, model.Error("error_unique_batches")
					},
				},
			},
			exp: tcExpected{
				err: model.Error("error_unique_batches"),
			},
		},

		{
			name: "error_active_not_at_limit",
			given: tcGiven{
				item: &model.OrderItem{
					NumSelfExtensions: 1,
					SKUVnt:            "some_sku",
					CredentialType:    timeLimitedV2,
				},
				policies: model.CredExtensionPolicies{
					"some_sku": model.CredExtensionPolicy{
						MaxPerItem: 2,
					},
				},
				tlv2Repo: &repository.MockTLV2{
					FnUniqBatches: func(ctx context.Context, dbi sqlx.QueryerContext, orderID, itemID uuid.UUID, from, to time.Time) (int, error) {
						return 0, nil
					},
				},
			},
			exp: tcExpected{
				err: model.ErrExtensionNotAtLimit,
			},
		},

		{
			name: "success",
			given: tcGiven{
				item: &model.OrderItem{
					MaxActiveBatchesTLV2Creds: ptrTo(13),
					NumSelfExtensions:         1,
					SKUVnt:                    "some_sku",
					CredentialType:            timeLimitedV2,
				},
				policies: model.CredExtensionPolicies{
					"some_sku": model.CredExtensionPolicy{
						SlotsPerGrant: 3,
						MaxPerItem:    2,
					},
				},
				tlv2Repo: &repository.MockTLV2{
					FnUniqBatches: func(ctx context.Context, dbi sqlx.QueryerContext, orderID, itemID uuid.UUID, from, to time.Time) (int, error) {
						return 13, nil
					},
				},
			},
			exp: tcExpected{
				nxt: NextExtension{
					maxActiveBatches: 16,
					numSelfExt:       2,
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ext := &TLV2CredExtender{
				policies: tc.given.policies,
				tlv2Repo: tc.given.tlv2Repo,
			}

			ctx := context.Background()

			actual, err := ext.GetNextExtIfValid(ctx, nil, tc.given.item, tc.given.now)
			must.ErrorIs(t, err, tc.exp.err)

			should.Equal(t, tc.exp.nxt, actual)
		})
	}
}

func TestCheckNextAllowedExtensionTime(t *testing.T) {
	type tcGiven struct {
		pol  model.CredExtensionPolicy
		item *model.OrderItem
		now  time.Time
	}

	type tcExpected struct {
		err error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "last_self_extension_at_nil",
			given: tcGiven{
				item: &model.OrderItem{},
			},
		},

		{
			name: "now_before_next_allowed",
			given: tcGiven{
				pol: model.CredExtensionPolicy{
					MinIntervalSeconds: 1,
				},
				item: &model.OrderItem{
					LastSelfExtensionAt: ptrTo(time.Date(2026, 1, 1, 1, 1, 10, 0, time.UTC)),
				},
				now: time.Date(2026, 1, 1, 1, 1, 1, 0, time.UTC),
			},
			exp: tcExpected{
				err: model.ErrExtensionRateLimited,
			},
		},

		{
			name: "now_same_as_next_allowed",
			given: tcGiven{
				pol: model.CredExtensionPolicy{
					MinIntervalSeconds: 1,
				},
				item: &model.OrderItem{
					LastSelfExtensionAt: ptrTo(time.Date(2026, 1, 1, 1, 1, 10, 0, time.UTC)),
				},
				now: time.Date(2026, 1, 1, 1, 1, 10, 0, time.UTC),
			},
			exp: tcExpected{
				err: model.ErrExtensionRateLimited,
			},
		},

		{
			name: "now_after_next_allowed",
			given: tcGiven{
				pol: model.CredExtensionPolicy{
					MinIntervalSeconds: 1,
				},
				item: &model.OrderItem{
					LastSelfExtensionAt: ptrTo(time.Date(2026, 1, 1, 1, 1, 10, 0, time.UTC)),
				},
				now: time.Date(2026, 1, 1, 1, 1, 30, 0, time.UTC),
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := checkNextAllowedExtensionTime(tc.given.pol, tc.given.item, tc.given.now)
			should.Equal(t, tc.exp.err, actual)
		})
	}
}

func genNumStrings(n int) []string {
	result := make([]string, n)

	for i := 0; i < n; i++ {
		result[i] = strconv.Itoa(i)
	}

	return result
}
