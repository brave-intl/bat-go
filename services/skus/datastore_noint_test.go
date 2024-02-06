package skus

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
	should "github.com/stretchr/testify/assert"
)

type mockGetContext struct {
	getContext func(ctx context.Context, dest interface{}, query string, args ...interface{}) error
}

func (mgc *mockGetContext) GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	if mgc.getContext != nil {
		return mgc.getContext(ctx, dest, query, args)
	}
	return nil
}

func TestAreTimeLimitedV2CredsSubmitted(t *testing.T) {
	type tcExpected struct {
		result map[string]bool
		noErr  bool
	}

	type testCase struct {
		name  string
		dbi   getContext
		given uuid.UUID
		exp   tcExpected
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []testCase{
		{
			name: "already_submitted",
			dbi: &mockGetContext{
				getContext: func(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
					*dest.(*AreTimeLimitedV2CredsSubmittedResult) = AreTimeLimitedV2CredsSubmittedResult{
						AlreadySubmitted: true,
						Mismatch:         false,
					}
					return nil
				},
			},
			given: uuid.Must(uuid.FromString("8f51f9ca-b593-4200-9bfb-91ac34748e09")),
			exp: tcExpected{
				noErr: true,
				result: map[string]bool{
					"alreadySubmitted": true,
					"mismatch": false,
				},
			},
		},
		{
			name: "mismatch",
			dbi: &mockGetContext{
				getContext: func(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
					*dest.(*AreTimeLimitedV2CredsSubmittedResult) = AreTimeLimitedV2CredsSubmittedResult{
						AlreadySubmitted: false,
						Mismatch:         true,
					}
					return nil
				},
			},
			given: uuid.Must(uuid.FromString("8f51f9ca-b593-4200-9bfb-91ac34748e09")),
			exp: tcExpected{
				noErr: true,
				result: map[string]bool{
					"alreadySubmitted": false,
					"mismatch": true,
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			result, err := areTimeLimitedV2CredsSubmitted(context.TODO(), tc.dbi, tc.given, "")
			should.Equal(t, tc.exp.result["alreadySubmitted"], result.AlreadySubmitted)
			should.Equal(t, tc.exp.result["mismatch"], result.Mismatch)
			should.Equal(t, tc.exp.noErr, err == nil)
		})
	}
}
