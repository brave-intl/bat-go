package wallet

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"

	"github.com/brave-intl/bat-go/services/wallet/model"
)

func TestSolAddrsChecker_IsAllowed(t *testing.T) {
	type tcGiven struct {
		addrs string
		sac   *solAddrsChecker
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
			name: "aws_error",
			given: tcGiven{
				addrs: "solana_address",
				sac: &solAddrsChecker{
					s3h: &mockS3Header{fnHeadObject: func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
						return nil, model.Error("s3_svc_error")
					}},
				},
			},
			exp: tcExpected{
				model.Error("s3_svc_error"),
			},
		},

		{
			name: "not_allowed",
			given: tcGiven{
				addrs: "solana_address",
				sac: &solAddrsChecker{
					s3h: &mockS3Header{fnHeadObject: func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
						return nil, nil
					}},
				},
			},
			exp: tcExpected{
				model.ErrSolAddrsNotAllowed,
			},
		},

		{
			name: "allowed",
			given: tcGiven{
				addrs: "solana_address",
				sac: &solAddrsChecker{
					s3h: &mockS3Header{fnHeadObject: func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
						return nil, &types.NotFound{
							Message:           nil,
							ErrorCodeOverride: nil,
						}
					}},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			actual := tc.given.sac.IsAllowed(ctx, tc.given.addrs)
			assert.ErrorIs(t, actual, tc.exp.err)
		})
	}
}

type mockS3Header struct {
	fnHeadObject func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
}

func (s *mockS3Header) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if s.fnHeadObject == nil {
		return nil, nil
	}

	return s.fnHeadObject(ctx, params, optFns...)
}
