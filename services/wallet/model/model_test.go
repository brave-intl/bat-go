package model

import (
	"testing"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

func TestAllowListEntry_IsAllowed(t *testing.T) {
	type tcGiven struct {
		allow     AllowListEntry
		paymentID uuid.UUID
	}

	type exp struct {
		isAllowed bool
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   exp
	}

	tests := []testCase{
		{
			name: "default_allow_list_entry",
			given: tcGiven{
				paymentID: uuid.FromStringOrNil("dc5a802a-87e9-47a5-9d9c-af4c8f171bf3"),
			},
			exp: exp{isAllowed: false},
		},
		{
			name: "payment_id_nil",
			given: tcGiven{
				paymentID: uuid.Nil,
			},
			exp: exp{isAllowed: false},
		},
		{
			name: "payment_ids_not_equal",
			given: tcGiven{
				allow: AllowListEntry{
					PaymentID: uuid.FromStringOrNil("d1359406-42f1-4364-99b7-77840e8594e8"),
				},
				paymentID: uuid.FromStringOrNil("dc5a802a-87e9-47a5-9d9c-af4c8f171bf3"),
			},
			exp: exp{isAllowed: false},
		},
		{
			name: "payment_ids_are_equal",
			given: tcGiven{
				allow: AllowListEntry{
					PaymentID: uuid.FromStringOrNil("356a634a-dbae-4f95-b276-f3f0f0a53509"),
				},
				paymentID: uuid.FromStringOrNil("356a634a-dbae-4f95-b276-f3f0f0a53509"),
			},
			exp: exp{isAllowed: true},
		},
	}
	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.allow.IsAllowed(tc.given.paymentID)
			assert.Equal(t, tc.exp.isAllowed, actual)
		})
	}
}

func TestChallenge_IsValid(t *testing.T) {
	type tcGiven struct {
		chl Challenge
		now time.Time
	}

	type testCase struct {
		name      string
		given     tcGiven
		assertErr assert.ErrorAssertionFunc
	}

	tests := []testCase{
		{
			name: "expired",
			given: tcGiven{
				chl: Challenge{
					CreatedAt: time.Now(),
				},
				now: time.Now().Add(6 * time.Minute),
			},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, ErrChallengeExpired)
			},
		},
		{
			name: "valid",
			given: tcGiven{
				chl: Challenge{
					CreatedAt: time.Now(),
				},
				now: time.Now(),
			},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.NoError(t, err)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.given.chl.IsValid(tc.given.now)
			tc.assertErr(t, err)
		})
	}
}
