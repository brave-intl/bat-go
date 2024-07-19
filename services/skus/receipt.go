package skus

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/awa/go-iap/appstore"
	"github.com/awa/go-iap/playstore"
	"google.golang.org/api/androidpublisher/v3"

	"github.com/brave-intl/bat-go/services/skus/model"
)

const (
	errNoInAppTx           model.Error = "no in app info in response"
	errIOSPurchaseNotFound model.Error = "ios: purchase not found"
)

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
func (v *receiptVerifier) validateApple(ctx context.Context, req model.ReceiptRequest) (model.ReceiptData, error) {
	return v.validateAppleTime(ctx, req, time.Now())
}

func (v *receiptVerifier) validateAppleTime(ctx context.Context, req model.ReceiptRequest, now time.Time) (model.ReceiptData, error) {
	asreq := appstore.IAPRequest{
		Password:               v.asKey,
		ReceiptData:            req.Blob,
		ExcludeOldTransactions: true,
	}

	resp := &appstore.IAPResponse{}
	if err := v.appStoreCl.Verify(ctx, asreq, resp); err != nil {
		return model.ReceiptData{}, fmt.Errorf("failed to verify receipt: %w", err)
	}

	// ProductID on an InApp object must match the SubscriptionID.
	//
	// By doing so we:
	// - find the purchase that is being verified (i.e. to disambiguate VPN from Leo);
	// - utilise Apple verification to make sure the client supplied data (SubscriptionID) is valid and to be trusted.
	item, ok := findInAppBySubID(resp.Receipt.InApp, req.SubscriptionID, now)
	if ok {
		return newReceiptDataApple(req, item), nil
	}

	// Try finding in latest_receipt_info.
	item, ok = findInAppBySubID(resp.LatestReceiptInfo, req.SubscriptionID, now)
	if ok {
		return newReceiptDataApple(req, item), nil
	}

	// Special case for VPN.
	// The client may send bravevpn.monthly as subscription_id for bravevpn.yearly product.
	if req.SubscriptionID == "bravevpn.monthly" {
		item, ok := findInAppBySubID(resp.Receipt.InApp, "bravevpn.yearly", now)
		if ok {
			return newReceiptDataApple(req, item), nil
		}

		item, ok = findInAppBySubID(resp.LatestReceiptInfo, "bravevpn.yearly", now)
		if ok {
			return newReceiptDataApple(req, item), nil
		}
	}

	// Handle legacy iOS versions predating the release that started using proper values for subscription_id.
	// This only applies to VPN.
	item, ok = findInAppBySubIDLegacy(resp, req.SubscriptionID, now)
	if !ok {
		return model.ReceiptData{}, errIOSPurchaseNotFound
	}

	return newReceiptDataApple(req, item), nil
}

// validateGoogle validates a Play Store receipt.
func (v *receiptVerifier) validateGoogle(ctx context.Context, req model.ReceiptRequest) (model.ReceiptData, error) {
	return v.validateGoogleTime(ctx, req, time.Now())
}

func (v *receiptVerifier) validateGoogleTime(ctx context.Context, req model.ReceiptRequest, now time.Time) (model.ReceiptData, error) {
	sub, err := v.fetchSubPlayStore(ctx, req.Package, req.SubscriptionID, req.Blob)
	if err != nil {
		return model.ReceiptData{}, fmt.Errorf("failed to fetch subscription purchase: %w", err)
	}

	psub := (*playStoreSubPurchase)(sub)
	if psub.hasExpired(now) {
		return model.ReceiptData{}, errGPSSubPurchaseExpired
	}

	if psub.isPending() {
		return model.ReceiptData{}, errGPSSubPurchasePending
	}

	return newReceiptDataGoogle(req, psub), nil
}

func (v *receiptVerifier) fetchSubPlayStore(ctx context.Context, pkgName, subID, token string) (*androidpublisher.SubscriptionPurchase, error) {
	return v.playStoreCl.VerifySubscription(ctx, pkgName, subID, token)
}

func findInAppBySubID(iap []appstore.InApp, subID string, now time.Time) (*wrapAppStoreInApp, bool) {
	for i := range iap {
		if iap[i].ProductID == subID {
			item := newWrapAppStoreInApp(&iap[i])

			if !item.hasExpired(now) {
				return item, true
			}
		}
	}

	return nil, false
}

func findInAppBySubIDLegacy(resp *appstore.IAPResponse, subID string, now time.Time) (*wrapAppStoreInApp, bool) {
	item, ok := findInAppVPNLegacy(resp.Receipt.InApp, subID, now)
	if ok {
		return item, true
	}

	return findInAppVPNLegacy(resp.LatestReceiptInfo, subID, now)
}

func findInAppVPNLegacy(iap []appstore.InApp, subID string, now time.Time) (*wrapAppStoreInApp, bool) {
	switch subID {
	case "brave-firewall-vpn-premium":
		item, ok := findInAppBySubID(iap, "bravevpn.monthly", now)
		if ok {
			return item, true
		}

		// Quick fix for linking coming from iOS v1.61.1 and below.
		// The old clients might send brave-firewall-vpn-premium for bravevpn.yearly.
		return findInAppBySubID(iap, "bravevpn.yearly", now)

	case "brave-firewall-vpn-premium-year":
		return findInAppBySubID(iap, "bravevpn.yearly", now)
	default:
		return nil, false
	}
}
