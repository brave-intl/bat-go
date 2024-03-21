package skus

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/api/idtoken"
)

func TestNewGogglePushNotificationValidator_IsValid(t *testing.T) {
	type tcGiven struct {
		req            *http.Request
		cfg            gcpValidatorConfig
		tokenValidator gcpTokenValidator
	}

	type testCase struct {
		name      string
		given     tcGiven
		assertErr assert.ErrorAssertionFunc
	}

	testCases := []testCase{
		{
			name: "disabled",
			given: tcGiven{
				cfg: gcpValidatorConfig{disabled: true},
			},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.NoError(t, err)
			},
		},
		{
			name: "invalid_no_authorization_header",
			given: tcGiven{
				req: newRequest(""),
			},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, errAuthHeaderEmpty)
			},
		},
		{
			name: "invalid_authorization_header_format",
			given: tcGiven{
				req: newRequest("some-random-header-value"),
			},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, errAuthHeaderFormat)
			},
		},
		{
			name: "invalid_authentication_token",
			given: tcGiven{
				req: newRequest("Bearer: some-token"),
				tokenValidator: mockGcpTokenValidator{fnValidate: func(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error) {
					return nil, errors.New("error")
				}},
			},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorContains(t, err, "invalid authentication token: ")
			},
		},
		{
			name: "invalid_issuer_empty",
			given: tcGiven{
				req: newRequest("Bearer: some-token"),
				tokenValidator: mockGcpTokenValidator{fnValidate: func(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error) {
					return &idtoken.Payload{}, nil
				}},
			},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, errInvalidIssuer)
			},
		},
		{
			name: "invalid_issuer_not_equal",
			given: tcGiven{
				req: newRequest("Bearer: some-token"),
				cfg: gcpValidatorConfig{
					issuer: "issuer-1",
				},
				tokenValidator: mockGcpTokenValidator{fnValidate: func(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error) {
					return &idtoken.Payload{Issuer: "issuer-2"}, nil
				}},
			},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, errInvalidIssuer)
			},
		},
		{
			name: "invalid_email",
			given: tcGiven{
				req: newRequest("Bearer: some-token"),
				cfg: gcpValidatorConfig{
					issuer:         "issuer-1",
					serviceAccount: "service-account-1",
				},
				tokenValidator: mockGcpTokenValidator{fnValidate: func(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error) {
					issuer := "issuer-1"
					claims := map[string]interface{}{"email": "service-account-2"}
					return &idtoken.Payload{Issuer: issuer, Claims: claims}, nil
				}},
			},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, errInvalidEmail)
			},
		},
		{
			name: "invalid_email_not_verified",
			given: tcGiven{
				req: newRequest("Bearer: some-token"),
				cfg: gcpValidatorConfig{
					issuer:         "issuer-1",
					serviceAccount: "service-account-1",
				},
				tokenValidator: mockGcpTokenValidator{fnValidate: func(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error) {
					issuer := "issuer-1"
					claims := map[string]interface{}{"email": "service-account-1"}
					return &idtoken.Payload{Issuer: issuer, Claims: claims}, nil
				}},
			},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, errEmailNotVerified)
			},
		},
		{
			name: "valid_request",
			given: tcGiven{
				req: newRequest("Bearer: some-token"),
				cfg: gcpValidatorConfig{
					issuer:         "issuer-1",
					serviceAccount: "service-account-1",
				},
				tokenValidator: mockGcpTokenValidator{fnValidate: func(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error) {
					issuer := "issuer-1"
					claims := map[string]interface{}{"email": "service-account-1", "email_verified": true}
					return &idtoken.Payload{Issuer: issuer, Claims: claims}, nil
				}},
			},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.NoError(t, err)
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			v := newGcpPushNotificationValidator(tc.given.tokenValidator, tc.given.cfg)
			actual := v.validate(context.TODO(), tc.given.req)
			tc.assertErr(t, actual)
		})
	}
}

func newRequest(headerValue string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "https://some-url.com", nil)
	r.Header.Add("authorization", headerValue)
	return r
}

type mockGcpTokenValidator struct {
	fnValidate func(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error)
}

func (m mockGcpTokenValidator) Validate(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error) {
	if m.fnValidate == nil {
		return nil, nil
	}
	return m.fnValidate(ctx, idToken, audience)
}
