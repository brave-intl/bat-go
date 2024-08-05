package skus

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/awa/go-iap/appstore"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
	"google.golang.org/api/androidpublisher/v3"

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
				ProductID:             "braveleo.monthly",
				OriginalTransactionID: "720000000000000",
				ExpiresDate: appstore.ExpiresDate{
					ExpiresDateMS: strconv.FormatInt(time.Now().Add(15*24*time.Hour).UnixMilli(), 10),
				},
			},
		}

		return nil
	}

	return c.fnVerify(ctx, req, result)
}

type mockPSClient struct {
	fnVerifySubscription func(ctx context.Context, pkgName, subID, token string) (*androidpublisher.SubscriptionPurchase, error)
}

func (c *mockPSClient) VerifySubscription(ctx context.Context, pkgName, subID, token string) (*androidpublisher.SubscriptionPurchase, error) {
	if c.fnVerifySubscription == nil {
		result := &androidpublisher.SubscriptionPurchase{
			PaymentState:     ptrTo[int64](1),
			ExpiryTimeMillis: time.Now().Add(15 * 24 * time.Hour).UnixMilli(),
		}

		return result, nil
	}

	return c.fnVerifySubscription(ctx, pkgName, subID, token)
}

func TestReceiptVerifier_validateGoogleTime(t *testing.T) {
	type tcGiven struct {
		cl  *mockPSClient
		req model.ReceiptRequest
		now time.Time
	}

	type tcExpected struct {
		val model.ReceiptData
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
				cl: &mockPSClient{
					fnVerifySubscription: func(ctx context.Context, pkgName, subID, token string) (*androidpublisher.SubscriptionPurchase, error) {
						return nil, model.Error("something_went_wrong")
					},
				},
				req: model.ReceiptRequest{
					Type:           model.VendorGoogle,
					Blob:           "blob",
					Package:        "package",
					SubscriptionID: "sub_id",
				},
				now: time.Now(),
			},
			exp: tcExpected{
				err: model.Error("something_went_wrong"),
			},
		},

		{
			name: "has_expired",
			given: tcGiven{
				cl: &mockPSClient{
					fnVerifySubscription: func(ctx context.Context, pkgName, subID, token string) (*androidpublisher.SubscriptionPurchase, error) {
						result := &androidpublisher.SubscriptionPurchase{
							PaymentState:     ptrTo[int64](1),
							ExpiryTimeMillis: time.Now().Add(-1 * time.Hour).UnixMilli(),
						}

						return result, nil
					},
				},
				req: model.ReceiptRequest{
					Type:           model.VendorGoogle,
					Blob:           "blob",
					Package:        "package",
					SubscriptionID: "sub_id",
				},
				now: time.Now(),
			},
			exp: tcExpected{
				err: errGPSSubPurchaseExpired,
			},
		},

		{
			name: "is_pending",
			given: tcGiven{
				cl: &mockPSClient{
					fnVerifySubscription: func(ctx context.Context, pkgName, subID, token string) (*androidpublisher.SubscriptionPurchase, error) {
						result := &androidpublisher.SubscriptionPurchase{
							PaymentState:     ptrTo[int64](0),
							ExpiryTimeMillis: time.Now().Add(15 * 24 * time.Hour).UnixMilli(),
						}

						return result, nil
					},
				},
				req: model.ReceiptRequest{
					Type:           model.VendorGoogle,
					Blob:           "blob",
					Package:        "package",
					SubscriptionID: "sub_id",
				},
				now: time.Now(),
			},
			exp: tcExpected{
				err: errGPSSubPurchasePending,
			},
		},

		{
			name: "success",
			given: tcGiven{
				cl: &mockPSClient{
					fnVerifySubscription: func(ctx context.Context, pkgName, subID, token string) (*androidpublisher.SubscriptionPurchase, error) {
						result := &androidpublisher.SubscriptionPurchase{
							PaymentState:     ptrTo[int64](1),
							ExpiryTimeMillis: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(),
						}

						return result, nil
					},
				},
				req: model.ReceiptRequest{
					Type:           model.VendorGoogle,
					Blob:           "blob",
					Package:        "package",
					SubscriptionID: "sub_id",
				},
				now: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: tcExpected{
				val: model.ReceiptData{
					Type:      model.VendorGoogle,
					ProductID: "sub_id",
					ExtID:     "blob",
					ExpiresAt: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			vrf := &receiptVerifier{playStoreCl: tc.given.cl}

			actual, err := vrf.validateGoogleTime(context.Background(), tc.given.req, tc.given.now)
			must.Equal(t, true, errors.Is(err, tc.exp.err))

			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestReceiptVerifier_validateAppleTime(t *testing.T) {
	type tcGiven struct {
		key string
		cl  *mockASClient
		req model.ReceiptRequest
		now time.Time
	}

	type tcExpected struct {
		val model.ReceiptData
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
						if req.Password == "" {
							return model.Error("unexpected")
						}

						return model.Error("some_error")
					},
				},
				now: time.Now(),
			},
			exp: tcExpected{err: model.Error("some_error")},
		},

		{
			name: "single_purchase_not_found",
			given: tcGiven{
				req: model.ReceiptRequest{
					Type:           model.VendorApple,
					Package:        "com.brave.ios.browser",
					Blob:           "blob",
					SubscriptionID: "bravevpn.monthly",
				},

				cl:  &mockASClient{},
				now: time.Now(),
			},
			exp: tcExpected{err: errIOSPurchaseNotFound},
		},

		{
			name: "multiple_purchases_not_found",
			given: tcGiven{
				req: model.ReceiptRequest{
					Type:           model.VendorApple,
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
								ProductID:             "braveleo.monthly",
								OriginalTransactionID: "720000000000001",
								ExpiresDate: appstore.ExpiresDate{
									ExpiresDateMS: strconv.FormatInt(time.Now().Add(15*24*time.Hour).UnixMilli(), 10),
								},
							},

							{
								ProductID:             "bravevpn.yearly",
								OriginalTransactionID: "720000000000002",
								ExpiresDate: appstore.ExpiresDate{
									ExpiresDateMS: strconv.FormatInt(time.Now().Add(-1*time.Hour).UnixMilli(), 10),
								},
							},
						}

						return nil
					},
				},

				now: time.Now(),
			},
			exp: tcExpected{err: errIOSPurchaseNotFound},
		},

		{
			name: "single_purchase_found",
			given: tcGiven{
				req: model.ReceiptRequest{
					Type:           model.VendorApple,
					Package:        "com.brave.ios.browser",
					Blob:           "blob",
					SubscriptionID: "braveleo.monthly",
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
								ProductID:             "braveleo.monthly",
								OriginalTransactionID: "720000000000000",
								ExpiresDate: appstore.ExpiresDate{
									ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
								},
							},
						}

						return nil
					},
				},
				now: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: tcExpected{
				val: model.ReceiptData{
					Type:      model.VendorApple,
					ProductID: "braveleo.monthly",
					ExtID:     "720000000000000",
					ExpiresAt: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
				},
			},
		},

		{
			name: "single_purchase_expired",
			given: tcGiven{
				req: model.ReceiptRequest{
					Type:           model.VendorApple,
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
								ProductID:             "bravevpn.yearly",
								OriginalTransactionID: "720000000000002",
								ExpiresDate: appstore.ExpiresDate{
									ExpiresDateMS: strconv.FormatInt(time.Now().Add(-1*time.Hour).UnixMilli(), 10),
								},
							},
						}

						return nil
					},
				},

				now: time.Now(),
			},
			exp: tcExpected{err: errIOSPurchaseNotFound},
		},

		{
			name: "multiple_purchases_found",
			given: tcGiven{
				req: model.ReceiptRequest{
					Type:           model.VendorApple,
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
								ProductID:             "braveleo.monthly",
								OriginalTransactionID: "720000000000001",
								ExpiresDate: appstore.ExpiresDate{
									ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
								},
							},

							{
								ProductID:             "bravevpn.monthly",
								OriginalTransactionID: "720000000000002",
								ExpiresDate: appstore.ExpiresDate{
									ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 2, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
								},
							},
						}

						return nil
					},
				},

				now: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: tcExpected{
				val: model.ReceiptData{
					Type:      model.VendorApple,
					ProductID: "bravevpn.monthly",
					ExtID:     "720000000000002",
					ExpiresAt: time.Date(2024, time.February, 2, 0, 0, 0, 0, time.UTC),
				},
			},
		},

		{
			name: "multiple_purchases_found_latest_receipt_info",
			given: tcGiven{
				req: model.ReceiptRequest{
					Type:           model.VendorApple,
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
						resp.LatestReceiptInfo = []appstore.InApp{
							{
								ProductID:             "braveleo.monthly",
								OriginalTransactionID: "720000000000001",
								ExpiresDate: appstore.ExpiresDate{
									ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
								},
							},

							{
								ProductID:             "bravevpn.monthly",
								OriginalTransactionID: "720000000000002",
								ExpiresDate: appstore.ExpiresDate{
									ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 2, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
								},
							},
						}

						return nil
					},
				},

				now: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: tcExpected{
				val: model.ReceiptData{
					Type:      model.VendorApple,
					ProductID: "bravevpn.monthly",
					ExtID:     "720000000000002",
					ExpiresAt: time.Date(2024, time.February, 2, 0, 0, 0, 0, time.UTC),
				},
			},
		},

		{
			name: "multiple_purchases_mixed_found_latest_receipt_info",
			given: tcGiven{
				req: model.ReceiptRequest{
					Type:           model.VendorApple,
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
								ProductID:             "braveleo.monthly",
								OriginalTransactionID: "720000000000001",
								ExpiresDate: appstore.ExpiresDate{
									ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
								},
							},
						}

						resp.LatestReceiptInfo = []appstore.InApp{
							{
								ProductID:             "bravevpn.monthly",
								OriginalTransactionID: "720000000000002",
								ExpiresDate: appstore.ExpiresDate{
									ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 2, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
								},
							},
						}

						return nil
					},
				},

				now: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: tcExpected{
				val: model.ReceiptData{
					Type:      model.VendorApple,
					ProductID: "bravevpn.monthly",
					ExtID:     "720000000000002",
					ExpiresAt: time.Date(2024, time.February, 2, 0, 0, 0, 0, time.UTC),
				},
			},
		},

		{
			name: "legacy_multiple_purchases_mixed_found_receipt",
			given: tcGiven{
				req: model.ReceiptRequest{
					Type:           model.VendorApple,
					Package:        "com.brave.ios.browser",
					Blob:           "blob",
					SubscriptionID: "brave-firewall-vpn-premium",
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
								ProductID:             "bravevpn.monthly",
								OriginalTransactionID: "720000000000001",
								ExpiresDate: appstore.ExpiresDate{
									ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
								},
							},
						}

						resp.LatestReceiptInfo = []appstore.InApp{
							{
								ProductID:             "braveleo.monthly",
								OriginalTransactionID: "720000000000002",
								ExpiresDate: appstore.ExpiresDate{
									ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 2, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
								},
							},
						}

						return nil
					},
				},

				now: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: tcExpected{
				val: model.ReceiptData{
					Type:      model.VendorApple,
					ProductID: "bravevpn.monthly",
					ExtID:     "720000000000001",
					ExpiresAt: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
				},
			},
		},

		{
			name: "legacy_multiple_purchases_mixed_found_latest_receipt_info",
			given: tcGiven{
				req: model.ReceiptRequest{
					Type:           model.VendorApple,
					Package:        "com.brave.ios.browser",
					Blob:           "blob",
					SubscriptionID: "brave-firewall-vpn-premium-year",
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
								ProductID:             "braveleo.monthly",
								OriginalTransactionID: "720000000000001",
								ExpiresDate: appstore.ExpiresDate{
									ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
								},
							},
						}

						resp.LatestReceiptInfo = []appstore.InApp{
							{
								ProductID:             "bravevpn.yearly",
								OriginalTransactionID: "720000000000002",
								ExpiresDate: appstore.ExpiresDate{
									ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 2, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
								},
							},
						}

						return nil
					},
				},

				now: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: tcExpected{
				val: model.ReceiptData{
					Type:      model.VendorApple,
					ProductID: "bravevpn.yearly",
					ExtID:     "720000000000002",
					ExpiresAt: time.Date(2024, time.February, 2, 0, 0, 0, 0, time.UTC),
				},
			},
		},

		{
			name: "ios_vpn_bug_monthly_yearly",
			given: tcGiven{
				req: model.ReceiptRequest{
					Type:           model.VendorApple,
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
								ProductID:             "bravevpn.yearly",
								OriginalTransactionID: "720000000000001",
								ExpiresDate: appstore.ExpiresDate{
									ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
								},
							},
						}

						return nil
					},
				},

				now: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: tcExpected{
				val: model.ReceiptData{
					Type:      model.VendorApple,
					ProductID: "bravevpn.yearly",
					ExtID:     "720000000000001",
					ExpiresAt: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
				},
			},
		},

		{
			name: "ios_vpn_bug_monthly_yearly_latest_receipt_info",
			given: tcGiven{
				req: model.ReceiptRequest{
					Type:           model.VendorApple,
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
								ProductID:             "braveleo.yearly",
								OriginalTransactionID: "720000000000001",
								ExpiresDate: appstore.ExpiresDate{
									ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
								},
							},
						}

						resp.LatestReceiptInfo = []appstore.InApp{
							{
								ProductID:             "bravevpn.yearly",
								OriginalTransactionID: "720000000000002",
								ExpiresDate: appstore.ExpiresDate{
									ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 2, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
								},
							},
						}

						return nil
					},
				},

				now: time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: tcExpected{
				val: model.ReceiptData{
					Type:      model.VendorApple,
					ProductID: "bravevpn.yearly",
					ExtID:     "720000000000002",
					ExpiresAt: time.Date(2024, time.February, 2, 0, 0, 0, 0, time.UTC),
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			vrf := &receiptVerifier{asKey: tc.given.key, appStoreCl: tc.given.cl}

			actual, err := vrf.validateAppleTime(context.Background(), tc.given.req, tc.given.now)
			must.Equal(t, true, errors.Is(err, tc.exp.err))

			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestFindInAppBySubIDIAP(t *testing.T) {
	type tcGiven struct {
		resp  *appstore.IAPResponse
		subID string
		now   time.Time
	}

	type tcExpected struct {
		val *wrapAppStoreInApp
		ok  bool
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "empty",
			given: tcGiven{
				subID: "braveleo.monthly",
				resp:  &appstore.IAPResponse{},
			},
		},

		{
			name: "found_latest_receipt_info",
			given: tcGiven{
				resp: &appstore.IAPResponse{
					LatestReceiptInfo: []appstore.InApp{
						{
							ProductID: "bravevpn.yearly",
							ExpiresDate: appstore.ExpiresDate{
								ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
							},
						},
					},
				},
				subID: "bravevpn.yearly",
				now:   time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: tcExpected{
				val: &wrapAppStoreInApp{
					InApp: &appstore.InApp{
						ProductID: "bravevpn.yearly",
						ExpiresDate: appstore.ExpiresDate{
							ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
						},
					},
					expt: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
				},
				ok: true,
			},
		},

		{
			name: "found_receipt",
			given: tcGiven{
				resp: &appstore.IAPResponse{
					Receipt: appstore.Receipt{
						InApp: []appstore.InApp{
							{
								ProductID: "bravevpn.monthly",
								ExpiresDate: appstore.ExpiresDate{
									ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
								},
							},
						},
					},
				},
				subID: "bravevpn.monthly",
				now:   time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: tcExpected{
				val: &wrapAppStoreInApp{
					InApp: &appstore.InApp{
						ProductID: "bravevpn.monthly",
						ExpiresDate: appstore.ExpiresDate{
							ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
						},
					},
					expt: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
				},
				ok: true,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, ok := findInAppBySubIDIAP(tc.given.resp, tc.given.subID, tc.given.now)
			must.Equal(t, tc.exp.ok, ok)

			if !tc.exp.ok {
				return
			}

			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestFindInAppBySubID(t *testing.T) {
	type tcGiven struct {
		iap   []appstore.InApp
		subID string
		now   time.Time
	}

	type tcExpected struct {
		val *wrapAppStoreInApp
		ok  bool
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "empty",
			given: tcGiven{
				iap:   []appstore.InApp{},
				subID: "braveleo.monthly",
				now:   time.Now(),
			},
			exp: tcExpected{},
		},

		{
			name: "one_item_not_found_product_id_sub_id_mismatch",
			given: tcGiven{
				iap: []appstore.InApp{
					{
						ProductID: "bravevpn.yearly",
						ExpiresDate: appstore.ExpiresDate{
							ExpiresDateMS: strconv.FormatInt(time.Now().Add(15*24*time.Hour).UnixMilli(), 10),
						},
					},
				},

				subID: "braveleo.monthly",
				now:   time.Now(),
			},
			exp: tcExpected{},
		},

		{
			name: "two_items_not_found",
			given: tcGiven{
				iap: []appstore.InApp{
					{
						ProductID: "bravevpn.yearly",
						ExpiresDate: appstore.ExpiresDate{
							ExpiresDateMS: strconv.FormatInt(time.Now().Add(15*24*time.Hour).UnixMilli(), 10),
						},
					},

					{
						ProductID: "bravevpn.monthly",
						ExpiresDate: appstore.ExpiresDate{
							ExpiresDateMS: strconv.FormatInt(time.Now().Add(15*24*time.Hour).UnixMilli(), 10),
						},
					},
				},

				subID: "braveleo.monthly",
				now:   time.Now(),
			},
			exp: tcExpected{},
		},

		{
			name: "one_item_not_found_expired",
			given: tcGiven{
				iap: []appstore.InApp{
					{
						ProductID: "braveleo.monthly",
						ExpiresDate: appstore.ExpiresDate{
							ExpiresDateMS: strconv.FormatInt(time.Now().Add(-1*time.Hour).UnixMilli(), 10),
						},
					},
				},

				subID: "braveleo.monthly",
				now:   time.Now(),
			},
			exp: tcExpected{},
		},

		{
			name: "one_item_found",
			given: tcGiven{
				iap: []appstore.InApp{
					{
						ProductID: "braveleo.monthly",
						ExpiresDate: appstore.ExpiresDate{
							ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
						},
					},
				},

				subID: "braveleo.monthly",
				now:   time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: tcExpected{
				val: &wrapAppStoreInApp{
					InApp: &appstore.InApp{
						ProductID: "braveleo.monthly",
						ExpiresDate: appstore.ExpiresDate{
							ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
						},
					},
					expt: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
				},
				ok: true,
			},
		},

		{
			name: "two_items_found",
			given: tcGiven{
				iap: []appstore.InApp{
					{
						ProductID: "bravevpn.monthly",
						ExpiresDate: appstore.ExpiresDate{
							ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
						},
					},

					{
						ProductID: "braveleo.monthly",
						ExpiresDate: appstore.ExpiresDate{
							ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
						},
					},
				},

				subID: "braveleo.monthly",
				now:   time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: tcExpected{
				val: &wrapAppStoreInApp{
					InApp: &appstore.InApp{
						ProductID: "braveleo.monthly",
						ExpiresDate: appstore.ExpiresDate{
							ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
						},
					},
					expt: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
				},
				ok: true,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, ok := findInAppBySubID(tc.given.iap, tc.given.subID, tc.given.now)
			must.Equal(t, tc.exp.ok, ok)

			if !tc.exp.ok {
				return
			}

			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestFindInAppBySubIDLegacy(t *testing.T) {
	type tcGiven struct {
		resp  *appstore.IAPResponse
		subID string
		now   time.Time
	}

	type tcExpected struct {
		val *wrapAppStoreInApp
		ok  bool
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "empty",
			given: tcGiven{
				resp:  &appstore.IAPResponse{},
				subID: "brave-firewall-vpn-premium",
			},
		},

		{
			name: "unsupported",
			given: tcGiven{
				resp: &appstore.IAPResponse{
					Receipt: appstore.Receipt{
						InApp: []appstore.InApp{
							{ProductID: "bravevpn.monthly"},
						},
					},
				},
				subID: "bravevpn.monthly",
			},
		},

		{
			name: "found_receipt",
			given: tcGiven{
				resp: &appstore.IAPResponse{
					Receipt: appstore.Receipt{
						InApp: []appstore.InApp{
							{
								ProductID: "bravevpn.monthly",
								ExpiresDate: appstore.ExpiresDate{
									ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
								},
							},
						},
					},
				},
				subID: "brave-firewall-vpn-premium",
				now:   time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: tcExpected{
				val: &wrapAppStoreInApp{
					InApp: &appstore.InApp{
						ProductID: "bravevpn.monthly",
						ExpiresDate: appstore.ExpiresDate{
							ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
						},
					},
					expt: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
				},
				ok: true,
			},
		},

		{
			name: "found_latest_receipt_info",
			given: tcGiven{
				resp: &appstore.IAPResponse{
					LatestReceiptInfo: []appstore.InApp{
						{
							ProductID: "bravevpn.yearly",
							ExpiresDate: appstore.ExpiresDate{
								ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
							},
						},
					},
				},
				subID: "brave-firewall-vpn-premium-year",
				now:   time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: tcExpected{
				val: &wrapAppStoreInApp{
					InApp: &appstore.InApp{
						ProductID: "bravevpn.yearly",
						ExpiresDate: appstore.ExpiresDate{
							ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
						},
					},
					expt: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
				},
				ok: true,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, ok := findInAppBySubIDLegacy(tc.given.resp, tc.given.subID, tc.given.now)
			must.Equal(t, tc.exp.ok, ok)

			if !tc.exp.ok {
				return
			}

			should.Equal(t, tc.exp.val, actual)
		})
	}
}

func TestFindInAppVPNLegacy(t *testing.T) {
	type tcGiven struct {
		iap   []appstore.InApp
		subID string
		now   time.Time
	}

	type tcExpected struct {
		val *wrapAppStoreInApp
		ok  bool
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "unsupported",
			given: tcGiven{
				iap: []appstore.InApp{
					{ProductID: "bravevpn.monthly"},
				},
				subID: "bravevpn.monthly",
			},
			exp: tcExpected{},
		},

		{
			name: "vpn_monthly",
			given: tcGiven{
				iap: []appstore.InApp{
					{
						ProductID: "bravevpn.monthly",
						ExpiresDate: appstore.ExpiresDate{
							ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
						},
					},
				},
				subID: "brave-firewall-vpn-premium",
				now:   time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: tcExpected{
				val: &wrapAppStoreInApp{
					InApp: &appstore.InApp{
						ProductID: "bravevpn.monthly",
						ExpiresDate: appstore.ExpiresDate{
							ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
						},
					},
					expt: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
				},
				ok: true,
			},
		},

		{
			name: "vpn_annual_bug_v1.61.1",
			given: tcGiven{
				iap: []appstore.InApp{
					{
						ProductID: "bravevpn.yearly",
						ExpiresDate: appstore.ExpiresDate{
							ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
						},
					},
				},
				subID: "brave-firewall-vpn-premium",
				now:   time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: tcExpected{
				val: &wrapAppStoreInApp{
					InApp: &appstore.InApp{
						ProductID: "bravevpn.yearly",
						ExpiresDate: appstore.ExpiresDate{
							ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
						},
					},
					expt: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
				},
				ok: true,
			},
		},

		{
			name: "vpn_annual",
			given: tcGiven{
				iap: []appstore.InApp{
					{
						ProductID: "bravevpn.yearly",
						ExpiresDate: appstore.ExpiresDate{
							ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
						},
					},
				},
				subID: "brave-firewall-vpn-premium-year",
				now:   time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
			},
			exp: tcExpected{
				val: &wrapAppStoreInApp{
					InApp: &appstore.InApp{
						ProductID: "bravevpn.yearly",
						ExpiresDate: appstore.ExpiresDate{
							ExpiresDateMS: strconv.FormatInt(time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), 10),
						},
					},
					expt: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
				},
				ok: true,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, ok := findInAppVPNLegacy(tc.given.iap, tc.given.subID, tc.given.now)
			must.Equal(t, tc.exp.ok, ok)

			if !tc.exp.ok {
				return
			}

			should.Equal(t, tc.exp.val, actual)
		})
	}
}
