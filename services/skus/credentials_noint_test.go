package skus

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/services/skus/model"
)

func TestService_DeleteTLV2(t *testing.T) {
	type tcGiven struct {
		ord   *model.Order
		reqID uuid.UUID
	}

	type tcExpected struct {
		args []gomock.Matcher
		err  error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	now := time.Now()

	tests := []testCase{
		{
			name: "request_id_specified",
			given: tcGiven{
				ord:   &model.Order{ID: uuid.Must(uuid.FromString("a6b72f11-c886-49ee-b4f4-913eaa0984ae"))},
				reqID: uuid.Must(uuid.FromString("d10cf2ae-30d8-4ada-965c-11c582968f26")),
			},
			exp: tcExpected{
				args: []gomock.Matcher{
					gomock.Eq(context.Background()),
					gomock.Nil(), // dbi
					gomock.Eq(uuid.Must(uuid.FromString("a6b72f11-c886-49ee-b4f4-913eaa0984ae"))),
					gomock.Eq([]uuid.UUID{uuid.Must(uuid.FromString("d10cf2ae-30d8-4ada-965c-11c582968f26"))}),
					gomock.Eq(now),
				},
			},
		},

		{
			name: "request_id_nil",
			given: tcGiven{
				ord: &model.Order{
					ID: uuid.Must(uuid.FromString("d7b4f524-ebe3-48c1-b60d-d6f548b25aae")),
					Items: []model.OrderItem{
						{ID: uuid.Must(uuid.FromString("3882ec99-73fb-476b-b176-1f4b40a9b767"))},
						{ID: uuid.Must(uuid.FromString("ed437d36-182b-460f-8213-2ce3d4bb5c93"))},
						{ID: uuid.Must(uuid.FromString("d3e62075-996f-4bed-bbc7-f6cd324b83e0"))},
					},
				},
				reqID: uuid.Nil,
			},
			exp: tcExpected{
				args: []gomock.Matcher{
					gomock.Eq(context.Background()),
					gomock.Nil(), // dbi
					gomock.Eq(uuid.Must(uuid.FromString("d7b4f524-ebe3-48c1-b60d-d6f548b25aae"))),
					gomock.Eq([]uuid.UUID{
						uuid.Must(uuid.FromString("3882ec99-73fb-476b-b176-1f4b40a9b767")),
						uuid.Must(uuid.FromString("ed437d36-182b-460f-8213-2ce3d4bb5c93")),
						uuid.Must(uuid.FromString("d3e62075-996f-4bed-bbc7-f6cd324b83e0")),
					}),
					gomock.Eq(now),
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			must.Equal(t, 5, len(tc.exp.args))

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ds := NewMockDatastore(ctrl)

			svc := &Service{Datastore: ds}

			ctx := context.Background()
			if tc.given.reqID != uuid.Nil {
				ds.EXPECT().GetCountActiveOrderCreds(
					tc.exp.args[0], tc.exp.args[1], tc.exp.args[2], tc.exp.args[4],
				).Return(0, nil)
			}

			ds.EXPECT().DeleteTimeLimitedV2OrderCredsByOrderTx(
				tc.exp.args[0], tc.exp.args[1], tc.exp.args[2], tc.exp.args[3],
			).Return(nil)

			actual := svc.deleteTLV2(ctx, nil, tc.given.ord, tc.given.reqID, now)
			should.Equal(t, tc.exp.err, actual)
		})
	}
}
