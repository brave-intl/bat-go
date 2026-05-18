package repository

import (
	"testing"

	"github.com/lib/pq"
	should "github.com/stretchr/testify/assert"
)

func TestIsErrExtensionInvalidLimit(t *testing.T) {
	type tcGiven struct {
		err error
	}

	type tcExpected struct {
		res bool
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "not_pq_error",
		},

		{
			name: "not_order_items",
			given: tcGiven{
				err: &pq.Error{
					Table: "table",
				},
			},
		},

		{
			name: "not_severity_error",
			given: tcGiven{
				err: &pq.Error{
					Table:    "order_items",
					Severity: "INFO",
				},
			},
		},

		{
			name: "not_constraint_code",
			given: tcGiven{
				err: &pq.Error{
					Table:    "order_items",
					Severity: "ERROR",
					Code:     pq.ErrorCode("0"),
				},
			},
		},

		{
			name: "not_correct_constraint_name",
			given: tcGiven{
				err: &pq.Error{
					Table:      "order_items",
					Severity:   "ERROR",
					Code:       pq.ErrorCode("23514"),
					Constraint: "constraint",
				},
			},
		},

		{
			name: "success",
			given: tcGiven{
				err: &pq.Error{
					Table:      "order_items",
					Severity:   "ERROR",
					Code:       pq.ErrorCode("23514"),
					Constraint: "order_items_max_active_batches_tlv2_creds_sanity",
				},
			},
			exp: tcExpected{
				res: true,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := isErrExtensionInvalidLimit(tc.given.err)
			should.Equal(t, tc.exp.res, actual)
		})
	}
}
