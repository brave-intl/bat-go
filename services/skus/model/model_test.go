package model_test

import (
	"context"
	"testing"
	"time"

	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/libs/clients/radom"
	"github.com/brave-intl/bat-go/services/skus/model"
)

func TestOrder_CreateRadomCheckoutSessionWithTime(t *testing.T) {
	type tcGiven struct {
		order     *model.Order
		client    *radom.MockClient
		saddr     string
		expiresAt time.Time
	}

	type tcExpected struct {
		val model.CreateCheckoutSessionResponse
		err error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "no_items",
			given: tcGiven{
				order:  &model.Order{},
				client: &radom.MockClient{},
			},
			exp: tcExpected{
				err: model.ErrInvalidOrderNoItems,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.TODO()

			act, err := tc.given.order.CreateRadomCheckoutSessionWithTime(
				ctx,
				tc.given.client,
				tc.given.saddr,
				tc.given.expiresAt,
			)
			must.Equal(t, tc.exp.err, err)

			if tc.exp.err != nil {
				return
			}

			should.Equal(t, tc.exp.val, act)
		})
	}
}
