package skus

import (
	"context"
	"errors"
	"fmt"

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
)

var (
	receiptValidationFns = map[Vendor]func(context.Context, interface{}) (string, error){
		appleVendor:  validateIOSReceipt,
		googleVendor: validateAndroidReceipt,
	}
	iosClient              *appstore.Client
	androidClient          *playstore.Client
	errClientMisconfigured = errors.New("misconfigured client")

	errPurchasePending       = errors.New("purchase is pending")
	errPurchaseDeferred      = errors.New("purchase is deferred")
	errPurchaseStatusUnknown = errors.New("purchase status is unknown")
	errPurchaseFailed        = errors.New("purchase failed")

	purchasePendingErrCode       = "purchase_pending"
	purchaseDeferredErrCode      = "purchase_deferred"
	purchaseStatusUnknownErrCode = "purchase_status_unknown"
	purchaseFailedErrCode        = "purchase_failed"
	purchaseValidationErrCode    = "validation_failed"
)

func initClients(ctx context.Context) {
	logger := logging.Logger(ctx, "skus").With().Str("func", "initClients").Logger()
	iosClient = appstore.New()

	if jsonKey, ok := ctx.Value(appctx.PlaystoreJSONKeyCTXKey).([]byte); ok {
		var err error
		androidClient, err = playstore.New(jsonKey)
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
		// FIXME: what is package and subscription id
		if v, ok := receipt.(SubmitReceiptRequestV1); ok {
			logger.Debug().Str("receipt", fmt.Sprintf("%+v", v)).Msg("about to verify subscription")
			// handle v1 receipt type
			resp, err := androidClient.VerifySubscription(ctx, v.Package, v.SubscriptionID, v.Blob)
			if err != nil {
				logger.Error().Err(err).Msg("failed to verify subscription")
				return "", errPurchaseFailed
			}
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
