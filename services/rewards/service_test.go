package rewards

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"

	"github.com/brave-intl/bat-go/services/rewards/model"
)

func TestService_GetCards(t *testing.T) {
	type tcGiven struct {
		cfg *Config
		s3g s3Getter
	}

	type tcExpected struct {
		cards CardBytes
		err   error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "error_get_object",
			given: tcGiven{
				cfg: &Config{
					Cards: &CardsConfig{},
				},
				s3g: &mockS3Getter{
					fnGetObject: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
						return nil, model.Error("error")
					},
				},
			},
			exp: tcExpected{
				err: model.Error("error"),
			},
		},

		{
			name: "success",
			given: tcGiven{
				cfg: &Config{
					Cards: &CardsConfig{},
				},
				s3g: &mockS3Getter{
					fnGetObject: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
						cards := CardBytes(`{ "card": [{"title": "<string>", "description": "<string>", "url": "<string>", "thumbnail": "<string>"}] }`)

						out := &s3.GetObjectOutput{
							Body: io.NopCloser(bytes.NewReader(cards)),
						}

						return out, nil
					},
				},
			},
			exp: tcExpected{
				cards: CardBytes(`{ "card": [{"title": "<string>", "description": "<string>", "url": "<string>", "thumbnail": "<string>"}] }`),
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			s := &Service{
				cfg: tc.given.cfg,
				s3g: tc.given.s3g,
			}

			ctx := context.Background()

			actual, err := s.GetCardsAsBytes(ctx)

			assert.ErrorIs(t, err, tc.exp.err)
			assert.Equal(t, tc.exp.cards, actual)
		})
	}
}

type mockS3Getter struct {
	fnGetObject func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

func (m *mockS3Getter) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m.fnGetObject == nil {
		return &s3.GetObjectOutput{}, nil
	}

	return m.fnGetObject(ctx, params, optFns...)
}

func TestConfig_isDevelopment(t *testing.T) {
	type tcGiven struct {
		c *Config
	}

	type tcExpected struct {
		val bool
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "empty",
		},

		{
			name: "false",
			given: tcGiven{
				c: &Config{
					Env: "env",
				},
			},
		},

		{
			name: "true",
			given: tcGiven{
				c: &Config{
					Env: "development",
				},
			},
			exp: tcExpected{
				val: true,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.c.isDevelopment()
			assert.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestConfig_isStaging(t *testing.T) {
	type tcGiven struct {
		c *Config
	}

	type tcExpected struct {
		val bool
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "empty",
		},

		{
			name: "false",
			given: tcGiven{
				c: &Config{
					Env: "env",
				},
			},
		},

		{
			name: "true",
			given: tcGiven{
				c: &Config{
					Env: "staging",
				},
			},
			exp: tcExpected{
				val: true,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.c.isStaging()
			assert.Equal(t, tc.exp.val, actual)
		})
	}
}
