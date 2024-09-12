package skus

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/go-playground/validator/v10"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/libs/handlers"

	"github.com/brave-intl/bat-go/services/skus/model"
)

func TestCollectValidationErrors_ReceiptRequest(t *testing.T) {
	type tcExpected struct {
		result map[string]string
		ok     bool
		noErr  bool
	}

	type testCase struct {
		name  string
		given any
		exp   tcExpected
	}

	tests := []testCase{
		{
			name:  "invalid_type",
			given: map[string]struct{}{},
			exp:   tcExpected{},
		},

		{
			name: "no_errors_01",
			given: model.ReceiptRequest{
				Type:           model.VendorGoogle,
				Blob:           "some_data",
				Package:        "some_package",
				SubscriptionID: "some_subscription",
			},

			exp: tcExpected{noErr: true},
		},

		{
			name: "no_errors_02",
			given: model.ReceiptRequest{
				Type:           model.VendorApple,
				Blob:           "some_data",
				Package:        "some_package",
				SubscriptionID: "some_subscription",
			},

			exp: tcExpected{noErr: true},
		},

		{
			name: "invalid_vendor",
			given: model.ReceiptRequest{
				Type:           model.Vendor("brave"),
				Blob:           "some_data",
				Package:        "some_package",
				SubscriptionID: "some_subscription",
			},

			exp: tcExpected{
				result: map[string]string{
					"Type": "Key: 'ReceiptRequest.Type' Error:Field validation for 'Type' failed on the 'oneof' tag",
				},
				ok: true,
			},
		},

		{
			name: "no_blob",
			given: model.ReceiptRequest{
				Type:           model.VendorApple,
				Package:        "some_package",
				SubscriptionID: "some_subscription",
			},

			exp: tcExpected{
				result: map[string]string{
					"Blob": "Key: 'ReceiptRequest.Blob' Error:Field validation for 'Blob' failed on the 'required' tag",
				},
				ok: true,
			},
		},

		{
			name: "both_fields_error",
			given: model.ReceiptRequest{
				Type:           model.Vendor("brave"),
				Package:        "some_package",
				SubscriptionID: "some_subscription",
			},

			exp: tcExpected{
				result: map[string]string{
					"Type": "Key: 'ReceiptRequest.Type' Error:Field validation for 'Type' failed on the 'oneof' tag",
					"Blob": "Key: 'ReceiptRequest.Blob' Error:Field validation for 'Blob' failed on the 'required' tag",
				},
				ok: true,
			},
		},
	}

	valid := validator.New()

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			verr := valid.StructCtx(context.TODO(), tc.given)
			must.Equal(t, tc.exp.noErr, verr == nil)

			act, ok := collectValidationErrors(verr)

			should.Equal(t, tc.exp.ok, ok)
			should.Equal(t, tc.exp.result, act)
		})
	}
}

func TestParseSubmitReceiptRequest(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		const raw = `ewogICAgInBhY2thZ2UiOiAiY29tLmJyYXZlLmJyb3dzZXJfbmlnaHRseSIsCiAgICAicmF3X3JlY2VpcHQiOiAiYWFuaWRmY3BuY2dsbmpnaGttZmxna2toLkFPLUoxT3pxOUJMZFJ4YVVER2lOZ2JHaG5yaUNjUmpMYWNGZEFxdWNlbWJkMVMxV0JiaXZvREd1d1VsWGd3NkFYWVhvRWV2VXBUSHNmSXJLUDFJRU45WEpRQmhiOHhXX1VSTnlYdHVGSEFzOGktTGZ5MHJNVEU0IiwKICAgICJzdWJzY3JpcHRpb25faWQiOiAibmlnaHRseS5icmF2ZXZwbi5tb250aGx5IiwKICAgICJ0eXBlIjogImFuZHJvaWQiCn0KCg==`

		exp := model.ReceiptRequest{
			Type:           model.VendorGoogle,
			Blob:           "aanidfcpncglnjghkmflgkkh.AO-J1Ozq9BLdRxaUDGiNgbGhnriCcRjLacFdAqucembd1S1WBbivoDGuwUlXgw6AXYXoEevUpTHsfIrKP1IEN9XJQBhb8xW_URNyXtuFHAs8i-Lfy0rMTE4",
			Package:        "com.brave.browser_nightly",
			SubscriptionID: "nightly.bravevpn.monthly",
		}

		actual, err := parseSubmitReceiptRequest([]byte(raw))
		must.Equal(t, nil, err)

		should.Equal(t, exp, actual)
	})
}

func TestHandleReceiptErr(t *testing.T) {
	tests := []struct {
		name  string
		given error
		exp   *handlers.AppError
	}{
		{
			name: "nil",
			exp: &handlers.AppError{
				Message: "Unexpected error",
				Code:    http.StatusInternalServerError,
				Data:    map[string]interface{}{},
			},
		},

		{
			name:  "errIOSPurchaseNotFound",
			given: errIOSPurchaseNotFound,
			exp: &handlers.AppError{
				Message:   "Error " + errIOSPurchaseNotFound.Error(),
				Code:      http.StatusBadRequest,
				ErrorCode: "purchase_not_found",
				Data: map[string]interface{}{
					"validationErrors": map[string]interface{}{"receiptErrors": errIOSPurchaseNotFound.Error()},
				},
			},
		},

		{
			name:  "errExpiredGPSSubPurchase",
			given: errGPSSubPurchaseExpired,
			exp: &handlers.AppError{
				Message:   "Error " + errGPSSubPurchaseExpired.Error(),
				Code:      http.StatusBadRequest,
				ErrorCode: "purchase_expired",
				Data: map[string]interface{}{
					"validationErrors": map[string]interface{}{"receiptErrors": errGPSSubPurchaseExpired.Error()},
				},
			},
		},

		{
			name:  "errPendingGPSSubPurchase",
			given: errGPSSubPurchasePending,
			exp: &handlers.AppError{
				Message:   "Error " + errGPSSubPurchasePending.Error(),
				Code:      http.StatusBadRequest,
				ErrorCode: "purchase_pending",
				Data: map[string]interface{}{
					"validationErrors": map[string]interface{}{"receiptErrors": errGPSSubPurchasePending.Error()},
				},
			},
		},

		{
			name:  "errSomethingElse",
			given: model.Error("something_else"),
			exp: &handlers.AppError{
				Message:   "Error something_else",
				Code:      http.StatusBadRequest,
				ErrorCode: "validation_failed",
				Data: map[string]interface{}{
					"validationErrors": map[string]interface{}{"receiptErrors": "something_else"},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := handleReceiptErr(tc.given)

			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestParseVerifyCredRequestV2(t *testing.T) {
	type tcExpected struct {
		val     *model.VerifyCredentialRequestV2
		mustErr must.ErrorAssertionFunc
	}

	type testCase struct {
		name  string
		given []byte
		exp   tcExpected
	}

	tests := []testCase{
		{
			name:  "error_malformed_payload",
			given: []byte(`nonsense`),
			exp: tcExpected{
				mustErr: func(tt must.TestingT, err error, i ...interface{}) {
					must.Equal(tt, true, err != nil)
				},
			},
		},

		{
			name:  "error_malformed_credential",
			given: []byte(`{"sku":"sku","merchantId":"merchantId"}`),
			exp: tcExpected{
				mustErr: func(tt must.TestingT, err error, i ...interface{}) {
					must.Equal(tt, true, err != nil)
				},
			},
		},

		{
			name:  "success_complete",
			given: []byte(`{"sku": "sku","merchantId": "merchantId","credential":"eyJ0eXBlIjoidGltZS1saW1pdGVkLXYyIiwicHJlc2VudGF0aW9uIjoiVG1GMGRYSmxJR0ZpYUc5eWN5QmhJSFpoWTNWMWJTNEsifQo="}`),
			exp: tcExpected{
				val: &model.VerifyCredentialRequestV2{
					SKU:        "sku",
					MerchantID: "merchantId",
					Credential: "eyJ0eXBlIjoidGltZS1saW1pdGVkLXYyIiwicHJlc2VudGF0aW9uIjoiVG1GMGRYSmxJR0ZpYUc5eWN5QmhJSFpoWTNWMWJTNEsifQo=",
					CredentialOpaque: &model.VerifyCredentialOpaque{
						Type:         "time-limited-v2",
						Presentation: "TmF0dXJlIGFiaG9ycyBhIHZhY3V1bS4K",
					},
				},
				mustErr: func(tt must.TestingT, err error, i ...interface{}) {
					must.Equal(tt, true, err == nil)
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, err := parseVerifyCredRequestV2(tc.given)
			tc.exp.mustErr(t, err)

			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestValidateVerifyCredRequestV2(t *testing.T) {
	type tcGiven struct {
		valid *validator.Validate
		req   *model.VerifyCredentialRequestV2
	}

	tests := []struct {
		name  string
		given tcGiven
		exp   error
	}{
		{
			name: "error_credential_opaque_nil",
			given: tcGiven{
				valid: validator.New(),
				req: &model.VerifyCredentialRequestV2{
					SKU:        "sku",
					MerchantID: "merchantId",
					Credential: "eyJ0eXBlIjoic2luZ2xlLXVzZSIsInByZXNlbnRhdGlvbiI6IlRtRjBkWEpsSUdGaWFHOXljeUJoSUhaaFkzVjFiUzRLIn0K",
				},
			},
			exp: &validator.InvalidValidationError{Type: reflect.TypeOf((*model.VerifyCredentialOpaque)(nil))},
		},

		{
			name: "valid_single_use",
			given: tcGiven{
				valid: validator.New(),
				req: &model.VerifyCredentialRequestV2{
					SKU:        "sku",
					MerchantID: "merchantId",
					Credential: "eyJ0eXBlIjoic2luZ2xlLXVzZSIsInByZXNlbnRhdGlvbiI6IlRtRjBkWEpsSUdGaWFHOXljeUJoSUhaaFkzVjFiUzRLIn0K",
					CredentialOpaque: &model.VerifyCredentialOpaque{
						Type:         "single-use",
						Presentation: "TmF0dXJlIGFiaG9ycyBhIHZhY3V1bS4K",
					},
				},
			},
		},

		{
			name: "valid_time_limited",
			given: tcGiven{
				valid: validator.New(),
				req: &model.VerifyCredentialRequestV2{
					SKU:        "sku",
					MerchantID: "merchantId",
					Credential: "eyJ0eXBlIjoidGltZS1saW1pdGVkIiwicHJlc2VudGF0aW9uIjoiVG1GMGRYSmxJR0ZpYUc5eWN5QmhJSFpoWTNWMWJTNEsifQo=",
					CredentialOpaque: &model.VerifyCredentialOpaque{
						Type:         "time-limited",
						Presentation: "TmF0dXJlIGFiaG9ycyBhIHZhY3V1bS4K",
					},
				},
			},
		},

		{
			name: "valid_time_limited_v2",
			given: tcGiven{
				valid: validator.New(),
				req: &model.VerifyCredentialRequestV2{
					SKU:        "sku",
					MerchantID: "merchantId",
					Credential: "eyJ0eXBlIjoidGltZS1saW1pdGVkLXYyIiwicHJlc2VudGF0aW9uIjoiVG1GMGRYSmxJR0ZpYUc5eWN5QmhJSFpoWTNWMWJTNEsifQo=",
					CredentialOpaque: &model.VerifyCredentialOpaque{
						Type:         "time-limited-v2",
						Presentation: "TmF0dXJlIGFiaG9ycyBhIHZhY3V1bS4K",
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := validateVerifyCredRequestV2(tc.given.valid, tc.given.req)
			should.Equal(t, tc.exp, actual)
		})
	}
}
