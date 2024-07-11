package skus

import (
	"time"

	"google.golang.org/api/androidpublisher/v3"

	"github.com/brave-intl/bat-go/services/skus/model"
)

const (
	errExpiredGPSSubPurchase = model.Error("playstore: subscription purchase expired")
	errPendingGPSSubPurchase = model.Error("playstore: subscription purchase pending")
)

type playStoreSubPurchase androidpublisher.SubscriptionPurchase

func (x *playStoreSubPurchase) hasExpired(now time.Time) bool {
	return x.ExpiryTimeMillis < now.UnixMilli()
}

func (x *playStoreSubPurchase) isPending() bool {
	// The payment state is not present for canceled or expired subscriptions.
	if x.PaymentState == nil {
		return false
	}

	const pending, pendingDef = int64(0), int64(3)

	state := *x.PaymentState

	return state == pending || state == pendingDef
}
