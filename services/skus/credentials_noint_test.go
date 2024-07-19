package skus

import (
	"testing"

	should "github.com/stretchr/testify/assert"
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
