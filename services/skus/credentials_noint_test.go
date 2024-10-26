package skus

import (
	"strconv"
	"testing"

	uuid "github.com/satori/go.uuid"
	should "github.com/stretchr/testify/assert"

	"github.com/brave-intl/bat-go/libs/datastore"

	"github.com/brave-intl/bat-go/services/skus/model"
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

func genNumStrings(n int) []string {
	result := make([]string, n)

	for i := 0; i < n; i++ {
		result[i] = strconv.Itoa(i)
	}

	return result
}
