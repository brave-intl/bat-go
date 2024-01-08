package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
