package skus

import (
	"context"
	"net/http"
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
			name:  "errPurchasePending",
			given: errPurchasePending,
			exp: &handlers.AppError{
				Message:   "Error " + errPurchasePending.Error(),
				Code:      http.StatusBadRequest,
				ErrorCode: purchasePendingErrCode,
				Data: map[string]interface{}{
					"validationErrors": map[string]interface{}{"receiptErrors": errPurchasePending.Error()},
				},
			},
		},
		{
			name:  "errPurchaseExpired",
			given: errPurchaseExpired,
			exp: &handlers.AppError{
				Message:   "Error " + errPurchaseExpired.Error(),
				Code:      http.StatusBadRequest,
				ErrorCode: purchaseExpiredErrCode,
				Data: map[string]interface{}{
					"validationErrors": map[string]interface{}{"receiptErrors": errPurchaseExpired.Error()},
				},
			},
		},
		{
			name:  "errSomethingElse",
			given: model.Error("something else"),
			exp: &handlers.AppError{
				Message:   "Error something else",
				Code:      http.StatusBadRequest,
				ErrorCode: purchaseValidationErrCode,
				Data: map[string]interface{}{
					"validationErrors": map[string]interface{}{"receiptErrors": "something else"},
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
