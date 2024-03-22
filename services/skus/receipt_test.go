package skus

import (
	"context"
	"errors"
	"testing"

	"github.com/awa/go-iap/appstore"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/services/skus/model"
)

type mockASClient struct {
	fnVerify func(ctx context.Context, req appstore.IAPRequest, result interface{}) error
}

func (c *mockASClient) Verify(ctx context.Context, req appstore.IAPRequest, result interface{}) error {
	if c.fnVerify == nil {
		resp, ok := result.(*appstore.IAPResponse)
		if !ok {
			return model.Error("invalid response type")
		}

		resp.Receipt.BundleID = "com.brave.ios.browser"
		resp.Receipt.InApp = []appstore.InApp{
			{
				OriginalTransactionID: "720000000000000",
				ProductID:             "braveleo.monthly",
			},
		}

		return nil
	}

	return c.fnVerify(ctx, req, result)
}

func TestReceiptVerifier_validateApple(t *testing.T) {
	type tcGiven struct {
		key string
		cl  *mockASClient
		req model.ReceiptRequest
	}

	type tcExpected struct {
		val string
		err error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "client_error",
			given: tcGiven{
				key: "key",
				cl: &mockASClient{
					fnVerify: func(ctx context.Context, req appstore.IAPRequest, result interface{}) error {
						return model.Error("some_error")
					},
				},
			},
			exp: tcExpected{err: model.Error("some_error")},
		},

		{
			name: "empty_inapp",
			given: tcGiven{
				key: "key",
				cl: &mockASClient{
					fnVerify: func(ctx context.Context, req appstore.IAPRequest, result interface{}) error {
						return nil
					},
				},
			},
			exp: tcExpected{err: errNoInAppTx},
		},

		{
			name: "single_purchase_not_found",
			given: tcGiven{
				req: model.ReceiptRequest{
					Package:        "com.brave.ios.browser",
					Blob:           "blob",
					SubscriptionID: "bravevpn.monthly",
				},

				cl: &mockASClient{},
			},
			exp: tcExpected{err: errIOSPurchaseNotFound},
		},

		{
			name: "multiple_purchases_not_found",
			given: tcGiven{
				req: model.ReceiptRequest{
					Package:        "com.brave.ios.browser",
					Blob:           "blob",
					SubscriptionID: "bravevpn.monthly",
				},

				cl: &mockASClient{
					fnVerify: func(ctx context.Context, req appstore.IAPRequest, result interface{}) error {
						resp, ok := result.(*appstore.IAPResponse)
						if !ok {
							return model.Error("invalid response type")
						}

						resp.Receipt.BundleID = "com.brave.ios.browser"
						resp.Receipt.InApp = []appstore.InApp{
							{
								OriginalTransactionID: "720000000000001",
								ProductID:             "braveleo.monthly",
							},

							{
								OriginalTransactionID: "720000000000002",
								ProductID:             "bravevpn.yearly",
							},
						}

						return nil
					},
				},
			},
			exp: tcExpected{err: errIOSPurchaseNotFound},
		},

		{
			name: "single_purchase_found",
			given: tcGiven{
				req: model.ReceiptRequest{
					Package:        "com.brave.ios.browser",
					Blob:           "blob",
					SubscriptionID: "braveleo.monthly",
				},

				cl: &mockASClient{},
			},
			exp: tcExpected{val: "720000000000000"},
		},

		{
			name: "multiple_purchases_found",
			given: tcGiven{
				req: model.ReceiptRequest{
					Package:        "com.brave.ios.browser",
					Blob:           "blob",
					SubscriptionID: "bravevpn.monthly",
				},

				cl: &mockASClient{
					fnVerify: func(ctx context.Context, req appstore.IAPRequest, result interface{}) error {
						resp, ok := result.(*appstore.IAPResponse)
						if !ok {
							return model.Error("invalid response type")
						}

						resp.Receipt.BundleID = "com.brave.ios.browser"
						resp.Receipt.InApp = []appstore.InApp{
							{
								OriginalTransactionID: "720000000000001",
								ProductID:             "braveleo.monthly",
							},

							{
								OriginalTransactionID: "720000000000002",
								ProductID:             "bravevpn.monthly",
							},
						}

						return nil
					},
				},
			},
			exp: tcExpected{val: "720000000000002"},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			vrf := &receiptVerifier{asKey: tc.given.key, appStoreCl: tc.given.cl}

			actual, err := vrf.validateApple(context.Background(), tc.given.req)
			must.Equal(t, true, errors.Is(err, tc.exp.err))

			should.Equal(t, tc.exp.val, actual)
		})
	}
}
