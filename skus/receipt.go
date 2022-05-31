package skus

import (
	"context"
	"errors"
	"fmt"

	"github.com/awa/go-iap/appstore"
	"github.com/awa/go-iap/playstore"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
)

var (
	receiptValidationFns = map[Vendor]func(context.Context, string) (string, error){
		appleVendor:  validateIOSReceipt,
		googleVendor: validateAndroidReceipt,
	}
	iosClient              *appstore.Client
	androidClient          *playstore.Client
	errClientMisconfigured = errors.New("misconfigured client")
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
func validateIOSReceipt(ctx context.Context, receipt string) (string, error) {
	logger := logging.Logger(ctx, "skus").With().Str("func", "validateIOSReceipt").Logger()

	if iosClient != nil {
		req := appstore.IAPRequest{
			ReceiptData: receipt,
		}
		resp := &appstore.IAPResponse{}
		if err := iosClient.Verify(ctx, req, resp); err != nil {
			logger.Error().Err(err).Msg("failed to verify receipt")
			return "", fmt.Errorf("failed to verify receipt: %w", err)
		}
		// get the transaction id back
		if len(resp.Receipt.InApp) < 1 {
			logger.Error().Msg("failed to verify receipt, no in app info")
			return "", fmt.Errorf("failed to verify receipt, no in app info in response")
		}

		return resp.Receipt.InApp[0].TransactionID, nil
	}
	logger.Error().Msg("client is not configured")
	return "", errClientMisconfigured
}

// validateAndroidReceipt - validate android receipt with their apis
func validateAndroidReceipt(ctx context.Context, receipt string) (string, error) {
	logger := logging.Logger(ctx, "skus").With().Str("func", "validateAndroidReceipt").Logger()
	if androidClient != nil {
		// FIXME: what is package and subscription id
		resp, err := androidClient.VerifySubscription(ctx, "package", "subscriptionID", receipt)
		if err != nil {
			logger.Error().Err(err).Msg("client is not configured")
			return "", fmt.Errorf("failed to verify subscription: %w", err)
		}
		return resp.OrderId, nil
	}
	logger.Error().Msg("client is not configured")
	return "", errClientMisconfigured
}
