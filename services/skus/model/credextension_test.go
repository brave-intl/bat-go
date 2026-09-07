package model

import (
	"testing"
	"time"

	should "github.com/stretchr/testify/assert"
)

func TestCredExtensionPolicies_GetPolicy(t *testing.T) {
	type tcGiven struct {
		item *OrderItem
	}

	type tcExpected struct {
		pol CredExtensionPolicy
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
				item: &OrderItem{},
			},
			exp: tcExpected{
				err: ErrUnsupportedCredType,
			},
		},

		{
			name: "error_no_extension_policy",
			given: tcGiven{
				item: &OrderItem{CredentialType: "time-limited-v2"},
			},
			exp: tcExpected{
				err: ErrNoExtensionPolicy,
			},
		},

		{
			name: "success_origin_policy",
			given: tcGiven{
				item: &OrderItem{SKUVnt: "brave-origin-premium-perpetual-license", CredentialType: "time-limited-v2"},
			},
			exp: tcExpected{
				pol: CredExtensionPolicy{
					SlotsPerGrant:      3,
					MinIntervalSeconds: 30 * 24 * 60 * 60, // 30 Days.
					MaxPerItem:         5,
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			policies := NewPoliciesBySKUVnt()

			actual, err := policies.GetPolicy(tc.given.item)
			should.ErrorIs(t, err, tc.exp.err)
			should.Equal(t, tc.exp.pol, actual)
		})
	}
}

func TestExtensionState_AtLimit(t *testing.T) {
	type tcGiven struct {
		st ExtensionState
	}

	type tcExpected struct {
		atLimit bool
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "not_at_limit",
			given: tcGiven{
				st: ExtensionState{
					ActiveBatches: 0,
					Limit:         1,
				},
			},
		},

		{
			name: "at_limit_equal",
			given: tcGiven{
				st: ExtensionState{
					ActiveBatches: 1,
					Limit:         1,
				},
			},
			exp: tcExpected{atLimit: true},
		},

		{
			name: "at_limit_greater_than",
			given: tcGiven{
				st: ExtensionState{
					ActiveBatches: 2,
					Limit:         1,
				},
			},
			exp: tcExpected{atLimit: true},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.st.AtLimit()
			should.Equal(t, tc.exp.atLimit, actual)
		})
	}
}

func TestCredExtension_AtLimit(t *testing.T) {
	type tcGiven struct {
		pol CredExtensionPolicy
		st  ExtensionState
		now time.Time
	}

	type tcExpected struct {
		atLimit bool
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "not_at_limit_item_nil",
		},

		{
			name: "not_at_limit_state",
			given: tcGiven{
				st: ExtensionState{
					Item:          &OrderItem{},
					ActiveBatches: 1,
					Limit:         2,
				},
			},
		},

		{
			name: "at_limit_state",
			given: tcGiven{
				st: ExtensionState{
					Item:          &OrderItem{},
					ActiveBatches: 2,
					Limit:         2,
				},
			},
			exp: tcExpected{
				atLimit: true,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ext := NewCredExtension(tc.given.pol, tc.given.st, tc.given.now)

			actual := ext.AtLimit()
			should.Equal(t, tc.exp.atLimit, actual)
		})
	}
}

func TestCredExtension_CanExtend(t *testing.T) {
	type tcGiven struct {
		pol CredExtensionPolicy
		st  ExtensionState
		now time.Time
	}

	type tcExpected struct {
		canExt bool
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "false",
		},

		{
			name: "true",
			given: tcGiven{
				pol: CredExtensionPolicy{
					MaxPerItem: 1,
				},
				st: ExtensionState{
					Item:          &OrderItem{},
					ActiveBatches: 1,
					Limit:         1,
				},
			},
			exp: tcExpected{
				canExt: true,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ext := NewCredExtension(tc.given.pol, tc.given.st, tc.given.now)

			actual := ext.CanExtend()
			should.Equal(t, tc.exp.canExt, actual)
		})
	}
}

func TestCredExtension_Grant(t *testing.T) {
	type tcGiven struct {
		pol CredExtensionPolicy
		st  ExtensionState
		now time.Time
	}

	type tcExpected struct {
		grant ExtensionGrant
		err   error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "item_nil",
			exp: tcExpected{
				err: ErrExtensionStateItem,
			},
		},

		{
			name: "self_ext_equal_to_max",
			given: tcGiven{
				pol: CredExtensionPolicy{
					MaxPerItem: 1,
				},
				st: ExtensionState{
					Item: &OrderItem{
						NumSelfExtensions: 1,
					},
				},
			},
			exp: tcExpected{
				err: ErrExtensionMaxPerItem,
			},
		},

		{
			name: "self_ext_greater_than_max",
			given: tcGiven{
				pol: CredExtensionPolicy{
					MaxPerItem: 1,
				},
				st: ExtensionState{
					Item: &OrderItem{
						NumSelfExtensions: 2,
					},
				},
			},
			exp: tcExpected{
				err: ErrExtensionMaxPerItem,
			},
		},

		{
			name: "rate_limited",
			given: tcGiven{
				pol: CredExtensionPolicy{
					MaxPerItem:         2,
					MinIntervalSeconds: 10,
				},
				st: ExtensionState{
					Item: &OrderItem{
						NumSelfExtensions:   1,
						LastSelfExtensionAt: ptrTo(time.Date(2026, 1, 1, 1, 1, 10, 1, time.UTC)),
					},
				},
				now: time.Date(2026, 1, 1, 1, 1, 0, 1, time.UTC),
			},
			exp: tcExpected{
				err: ErrExtensionRateLimited,
			},
		},

		{
			name: "ext_not_at_limit",
			given: tcGiven{
				pol: CredExtensionPolicy{
					MaxPerItem: 2,
				},
				st: ExtensionState{
					Item: &OrderItem{
						NumSelfExtensions: 1,
					},
					ActiveBatches: 1,
					Limit:         2,
				},
			},
			exp: tcExpected{
				err: ErrExtensionNotAtLimit,
			},
		},

		{
			name: "success",
			given: tcGiven{
				pol: CredExtensionPolicy{
					SlotsPerGrant: 3,
					MaxPerItem:    2,
				},
				st: ExtensionState{
					Item: &OrderItem{
						NumSelfExtensions: 1,
					},
					ActiveBatches: 2,
					Limit:         2,
				},
			},
			exp: tcExpected{
				grant: ExtensionGrant{
					NextMaxActiveBatchLimit: 5,
					NextNumSelfExt:          2,
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ext := NewCredExtension(tc.given.pol, tc.given.st, tc.given.now)

			actual, err := ext.Grant()
			should.Equal(t, tc.exp.grant, actual)
			should.ErrorIs(t, err, tc.exp.err)
		})
	}
}

func TestCredExtension_rateLimited(t *testing.T) {
	type tcGiven struct {
		pol CredExtensionPolicy
		st  ExtensionState
		now time.Time
	}

	type tcExpected struct {
		rateLimited bool
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "rate_limited_last_self_ext_at_nil",
			given: tcGiven{
				st: ExtensionState{
					Item: &OrderItem{},
				},
			},
		},

		{
			name: "rate_limited_time_before",
			given: tcGiven{
				pol: CredExtensionPolicy{
					MinIntervalSeconds: 10,
				},
				st: ExtensionState{
					Item: &OrderItem{
						LastSelfExtensionAt: ptrTo(time.Date(2026, 1, 1, 1, 1, 1, 1, time.UTC)),
					},
				},
				now: time.Date(2026, 1, 1, 1, 1, 1, 1, time.UTC),
			},
			exp: tcExpected{
				rateLimited: true,
			},
		},

		{
			name: "not_rate_limited_time_after",
			given: tcGiven{
				pol: CredExtensionPolicy{
					MinIntervalSeconds: 10,
				},
				st: ExtensionState{
					Item: &OrderItem{
						LastSelfExtensionAt: ptrTo(time.Date(2026, 1, 1, 1, 1, 1, 1, time.UTC)),
					},
				},
				now: time.Date(2026, 1, 1, 1, 1, 20, 1, time.UTC),
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ext := NewCredExtension(tc.given.pol, tc.given.st, tc.given.now)

			actual := ext.rateLimited()
			should.Equal(t, tc.exp.rateLimited, actual)
		})
	}
}

func ptrTo[T any](v T) *T {
	return &v
}
