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
	"google.golang.org/api/androidpublisher/v3"

	"github.com/brave-intl/bat-go/libs/logging"

	"github.com/brave-intl/bat-go/services/skus/model"
)

const (
	androidPaymentStatePending int64 = iota
	androidPaymentStatePaid
	androidPaymentStateTrial
	androidPaymentStatePendingDeferred
)

const (
	androidCancelReasonUser      int64 = 0
	androidCancelReasonSystem    int64 = 1
	androidCancelReasonReplaced  int64 = 2
	androidCancelReasonDeveloper int64 = 3

	purchasePendingErrCode       = "purchase_pending"
	purchaseDeferredErrCode      = "purchase_deferred"
	purchaseStatusUnknownErrCode = "purchase_status_unknown"
	purchaseFailedErrCode        = "purchase_failed"
	purchaseValidationErrCode    = "validation_failed"
)

const (
	errNoInAppTx           model.Error = "no in app info in response"
	errIOSPurchaseNotFound model.Error = "ios: purchase not found"
)

var (
	errPurchaseUserCanceled      = errors.New("purchase is canceled by user")
	errPurchaseSystemCanceled    = errors.New("purchase is canceled by google playstore")
	errPurchaseReplacedCanceled  = errors.New("purchase is canceled and replaced")
	errPurchaseDeveloperCanceled = errors.New("purchase is canceled by developer")

	errPurchasePending       = errors.New("purchase is pending")
	errPurchaseDeferred      = errors.New("purchase is deferred")
	errPurchaseStatusUnknown = errors.New("purchase status is unknown")
	errPurchaseFailed        = errors.New("purchase failed")

	errPurchaseExpired = errors.New("purchase expired")
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

type appStoreVerifier interface {
	Verify(ctx context.Context, req appstore.IAPRequest, result interface{}) error
}

type playStoreVerifier interface {
	VerifySubscription(ctx context.Context, pkgName, subID, token string) (*androidpublisher.SubscriptionPurchase, error)
}

type receiptVerifier struct {
	asKey       string
	appStoreCl  appStoreVerifier
	playStoreCl playStoreVerifier
}

func newReceiptVerifier(cl *http.Client, asKey string, playKey []byte) (*receiptVerifier, error) {
	result := &receiptVerifier{
		asKey:      asKey,
		appStoreCl: appstore.NewWithClient(cl),
	}

	if playKey != nil && len(playKey) != 0 {
		gpCl, err := playstore.NewWithClient(playKey, cl)
		if err != nil {
			return nil, err
		}

		result.playStoreCl = gpCl
	}

	return result, nil
}

// validateApple validates Apple App Store receipt.
func (v *receiptVerifier) validateApple(ctx context.Context, req model.ReceiptRequest) (string, error) {
	asreq := appstore.IAPRequest{
		Password:               v.asKey,
		ReceiptData:            req.Blob,
		ExcludeOldTransactions: true,
	}

	resp := &appstore.IAPResponse{}
	if err := v.appStoreCl.Verify(ctx, asreq, resp); err != nil {
		return "", fmt.Errorf("failed to verify receipt: %w", err)
	}

	// ProductID on an InApp object must match the SubscriptionID.
	//
	// By doing so we:
	// - find the purchase that is being verified (i.e. to disambiguate VPN from Leo);
	// - utilise Apple verification to make sure the client supplied data (SubscriptionID) is valid and to be trusted.
	item, ok := findInAppBySubID(resp.Receipt.InApp, req.SubscriptionID)
	if ok {
		return item.OriginalTransactionID, nil
	}

	// Try finding in latest_receipt_info.
	item, ok = findInAppBySubID(resp.LatestReceiptInfo, req.SubscriptionID)
	if ok {
		return item.OriginalTransactionID, nil
	}

	// Special case for VPN.
	// The client may send bravevpn.monthly as subscription_id for bravevpn.yearly product.
	if req.SubscriptionID == "bravevpn.monthly" {
		item, ok := findInAppBySubID(resp.Receipt.InApp, "bravevpn.yearly")
		if ok {
			return item.OriginalTransactionID, nil
		}

		item, ok = findInAppBySubID(resp.LatestReceiptInfo, "bravevpn.yearly")
		if ok {
			return item.OriginalTransactionID, nil
		}
	}

	// Handle legacy iOS versions predating the release that started using proper values for subscription_id.
	// This only applies to VPN.
	item, ok = findInAppBySubIDLegacy(resp, req.SubscriptionID)
	if !ok {
		return "", errIOSPurchaseNotFound
	}

	return item.OriginalTransactionID, nil
}

// validateGoogle validates Google Store receipt.
func (v *receiptVerifier) validateGoogle(ctx context.Context, req model.ReceiptRequest) (string, error) {
	l := logging.Logger(ctx, "skus").With().Str("func", "validateReceiptGoogle").Logger()

	l.Debug().Str("receipt", fmt.Sprintf("%+v", req)).Msg("about to verify subscription")

	resp, err := v.playStoreCl.VerifySubscription(ctx, req.Package, req.SubscriptionID, req.Blob)
	if err != nil {
		l.Error().Err(err).Msg("failed to verify subscription")
		return "", errPurchaseFailed
	}

	// Check order expiration.
	if time.Unix(0, resp.ExpiryTimeMillis*int64(time.Millisecond)).Before(time.Now()) {
		return "", errPurchaseExpired
	}

	l.Debug().Msgf("resp: %+v", resp)

	if resp.PaymentState == nil {
		l.Error().Err(err).Msg("failed to verify subscription: no payment state")
		return "", errPurchaseFailed
	}

	// Check that the order was paid.
	switch *resp.PaymentState {
	case androidPaymentStatePaid, androidPaymentStateTrial:
		return req.Blob, nil

	case androidPaymentStatePending:
		// Check for cancel reason.
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
		return "", errPurchasePending

	case androidPaymentStatePendingDeferred:
		return "", errPurchaseDeferred
	default:
		return "", errPurchaseStatusUnknown
	}
}

func findInAppBySubID(iap []appstore.InApp, subID string) (*appstore.InApp, bool) {
	for i := range iap {
		if iap[i].ProductID == subID {
			return &iap[i], true
		}
	}

	return nil, false
}

func findInAppBySubIDLegacy(resp *appstore.IAPResponse, subID string) (*appstore.InApp, bool) {
	item, ok := findInAppVPNLegacy(resp.Receipt.InApp, subID)
	if ok {
		return item, true
	}

	return findInAppVPNLegacy(resp.LatestReceiptInfo, subID)
}

func findInAppVPNLegacy(iap []appstore.InApp, subID string) (*appstore.InApp, bool) {
	switch subID {
	case "brave-firewall-vpn-premium":
		return findInAppBySubID(iap, "bravevpn.monthly")
	case "brave-firewall-vpn-premium-year":
		return findInAppBySubID(iap, "bravevpn.yearly")
	default:
		return nil, false
	}
}
