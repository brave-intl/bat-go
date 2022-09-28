package skus

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/awa/go-iap/appstore"
	"github.com/awa/go-iap/playstore"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
)

const (
	androidPaymentStatePending int64 = iota
	androidPaymentStatePaid
	androidPaymentStateTrial
	androidPaymentStatePendingDeferred

	androidCancelReasonUser      int64 = 0
	androidCancelReasonSystem    int64 = 1
	androidCancelReasonReplaced  int64 = 2
	androidCancelReasonDeveloper int64 = 3
)

var (
	receiptValidationFns = map[Vendor]func(context.Context, interface{}) (string, error){
		appleVendor:  validateIOSReceipt,
		googleVendor: validateAndroidReceipt,
	}
	iosClient              *appstore.Client
	androidClient          *playstore.Client
	errClientMisconfigured = errors.New("misconfigured client")

	errPurchaseUserCanceled      = errors.New("purchase is canceled by user")
	errPurchaseSystemCanceled    = errors.New("purchase is canceled by google playstore")
	errPurchaseReplacedCanceled  = errors.New("purchase is canceled and replaced")
	errPurchaseDeveloperCanceled = errors.New("purchase is canceled by developer")

	errPurchasePending       = errors.New("purchase is pending")
	errPurchaseDeferred      = errors.New("purchase is deferred")
	errPurchaseStatusUnknown = errors.New("purchase status is unknown")
	errPurchaseFailed        = errors.New("purchase failed")

	errPurchaseExpired = errors.New("purchase expired")

	purchasePendingErrCode       = "purchase_pending"
	purchaseDeferredErrCode      = "purchase_deferred"
	purchaseStatusUnknownErrCode = "purchase_status_unknown"
	purchaseFailedErrCode        = "purchase_failed"
	purchaseValidationErrCode    = "validation_failed"
)

type dumpTransport struct{}

func (dt *dumpTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	logger := logging.Logger(r.Context(), "skus").With().Str("func", "RoundTrip").Logger()

	dump, err := httputil.DumpRequestOut(r, true)
	if err != nil {
		logger.Error().Err(err).Msg("failed to dump request")
	}
	logger.Debug().Msgf("****REQUEST****\n%q\n", dump)

	resp, rtErr := http.DefaultTransport.RoundTrip(r)

	dump, err = httputil.DumpResponse(resp, true)
	if err != nil {
		logger.Error().Err(err).Msg("failed to dump response")
	}
	logger.Debug().Msgf("****RESPONSE****\n%q\n****************\n\n", dump)

	return resp, rtErr
}

func initClients(ctx context.Context) {

	var logClient = &http.Client{
		Transport: &dumpTransport{},
	}

	logger := logging.Logger(ctx, "skus").With().Str("func", "initClients").Logger()
	iosClient = appstore.New()

	if jsonKey, ok := ctx.Value(appctx.PlaystoreJSONKeyCTXKey).([]byte); ok {
		var err error
		androidClient, err = playstore.NewWithClient(jsonKey, logClient)
		if err != nil {
			logger.Error().Err(err).Msg("failed to initialize android client")
		}
	}
}

// validateIOSReceipt - validate apple receipt with their apis
func validateIOSReceipt(ctx context.Context, receipt interface{}) (string, error) {
	logger := logging.Logger(ctx, "skus").With().Str("func", "validateIOSReceipt").Logger()

	// get the shared key from the context
	sharedKey, sharedKeyOK := ctx.Value(appctx.AppleReceiptSharedKeyCTXKey).(string)

	if iosClient != nil {
		// handle v1 receipt type
		if v, ok := receipt.(SubmitReceiptRequestV1); ok {
			req := appstore.IAPRequest{
				ReceiptData:            v.Blob,
				ExcludeOldTransactions: true,
			}
			if sharedKeyOK && len(sharedKey) > 0 {
				req.Password = sharedKey
			}
			resp := &appstore.IAPResponse{}
			if err := iosClient.Verify(ctx, req, resp); err != nil {
				logger.Error().Err(err).Msg("failed to verify receipt")
				return "", fmt.Errorf("failed to verify receipt: %w", err)
			}
			logger.Debug().Msg(fmt.Sprintf("%+v", resp))
			// get the transaction id back
			if len(resp.Receipt.InApp) < 1 {
				logger.Error().Msg("failed to verify receipt, no in app info")
				return "", fmt.Errorf("failed to verify receipt, no in app info in response")
			}
			return resp.Receipt.InApp[0].TransactionID, nil
		}
	}
	logger.Error().Msg("client is not configured")
	return "", errClientMisconfigured
}

// validateAndroidReceipt - validate android receipt with their apis
func validateAndroidReceipt(ctx context.Context, receipt interface{}) (string, error) {
	logger := logging.Logger(ctx, "skus").With().Str("func", "validateAndroidReceipt").Logger()
	if androidClient != nil {
		if v, ok := receipt.(SubmitReceiptRequestV1); ok {
			logger.Debug().Str("receipt", fmt.Sprintf("%+v", v)).Msg("about to verify subscription")
			// handle v1 receipt type
			resp, err := androidClient.VerifySubscription(ctx, v.Package, v.SubscriptionID, v.Blob)
			if err != nil {
				logger.Error().Err(err).Msg("failed to verify subscription")
				return "", errPurchaseFailed
			}

			// is order expired?
			if time.Unix(0, resp.ExpiryTimeMillis*int64(time.Millisecond)).Before(time.Now()) {
				return "", errPurchaseExpired
			}

			// is there a cancel reason?
			switch resp.CancelReason {
			case androidCancelReasonUser:
				return "", errPurchaseUserCanceled
			case androidCancelReasonSystem:
				return "", errPurchaseSystemCanceled
			case androidCancelReasonReplaced:
				return "", errPurchaseReplacedCanceled
			case androidCancelReasonDeveloper:
				return "", errPurchaseDeveloperCanceled
			}

			logger.Debug().Msgf("resp: %+v", resp)
			// check that the order was paid
			switch resp.PaymentState {
			case androidPaymentStatePaid, androidPaymentStateTrial:
				break
			case androidPaymentStatePending:
				return "", errPurchasePending
			case androidPaymentStatePendingDeferred:
				return "", errPurchaseDeferred
			default:
				return "", errPurchaseStatusUnknown
			}
			return v.Blob, nil
		}
	}
	logger.Error().Msg("client is not configured")
	return "", errClientMisconfigured
}
