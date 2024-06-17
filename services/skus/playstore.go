package skus

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/androidpublisher/v3"
	"google.golang.org/api/idtoken"

	"github.com/brave-intl/bat-go/services/skus/model"
)

const (
	errGPSSubPurchaseExpired = model.Error("playstore: subscription purchase expired")
	errGPSSubPurchasePending = model.Error("playstore: subscription purchase pending")
	errGPSDisabled           = model.Error("playstore: notifications disabled")
	errGPSAuthHeaderEmpty    = model.Error("playstore: gcp authorization header is empty")
	errGPSAuthHeaderFmt      = model.Error("playstore: gcp authorization header invalid format")
	errGPSInvalidIssuer      = model.Error("playstore: gcp invalid issuer")
	errGPSInvalidEmail       = model.Error("playstore: gcp invalid email")
	errGPSEmailNotVerified   = model.Error("playstore: gcp email not verified")
	errGPSNoPurchaseToken    = model.Error("playstore: notification has no purchase")
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

type gpsValidatorConfig struct {
	aud      string
	iss      string
	svcAcct  string
	disabled bool
}

type gpsTokenValidator interface {
	Validate(ctx context.Context, token, aud string) (*idtoken.Payload, error)
}

type gpsNtfAuthenticator struct {
	cfg   gpsValidatorConfig
	valid gpsTokenValidator
}

func newGPSNtfAuthenticator(cfg gpsValidatorConfig, valid gpsTokenValidator) *gpsNtfAuthenticator {
	result := &gpsNtfAuthenticator{
		cfg:   cfg,
		valid: valid,
	}

	return result
}

func (x *gpsNtfAuthenticator) authenticate(ctx context.Context, hdr string) error {
	if x.cfg.disabled {
		return errGPSDisabled
	}

	if hdr == "" {
		return errGPSAuthHeaderEmpty
	}

	token := strings.Split(hdr, " ")
	if len(token) != 2 {
		return errGPSAuthHeaderFmt
	}

	p, err := x.valid.Validate(ctx, token[1], x.cfg.aud)
	if err != nil {
		return fmt.Errorf("invalid authentication token: %w", err)
	}

	if p.Issuer == "" || p.Issuer != x.cfg.iss {
		return errGPSInvalidIssuer
	}

	if p.Claims["email"] != x.cfg.svcAcct {
		return errGPSInvalidEmail
	}

	if p.Claims["email_verified"] != true {
		return errGPSEmailNotVerified
	}

	return nil
}

// playStoreDevNotification represents a notification from Play Store.
//
// For details, see https://developer.android.com/google/play/billing/rtdn-reference.
type playStoreDevNotification struct {
	PackageName       string                      `json:"packageName"`
	EventTimeMilli    json.Number                 `json:"eventTimeMillis"`
	SubscriptionNtf   *playStoreSubscriptionNtf   `json:"subscriptionNotification"`
	VoidedPurchaseNtf *playStoreVoidedPurchaseNtf `json:"voidedPurchaseNotification"`

	// Only presense of these matters. The content is ignored.
	OneTimeProductNtf *struct{} `json:"oneTimeProductNotification"`
	TestNtf           *struct{} `json:"testNotification"`
}

func (x *playStoreDevNotification) shouldProcess() bool {
	switch {
	case x.SubscriptionNtf != nil:
		return x.SubscriptionNtf.shouldProcess()

	case x.VoidedPurchaseNtf != nil:
		return x.VoidedPurchaseNtf.shouldProcess()

	case x.OneTimeProductNtf != nil:
		return false

	case x.TestNtf != nil:
		return false

	default:
		return false
	}
}

func (x *playStoreDevNotification) ntfType() string {
	switch {
	case x.SubscriptionNtf != nil:
		return "subscription"

	case x.VoidedPurchaseNtf != nil:
		return "voided_purchase"

	case x.OneTimeProductNtf != nil:
		return "one_time_product"

	case x.TestNtf != nil:
		return "test"

	default:
		return "unknown"
	}
}

func (x playStoreDevNotification) ntfSubType() int {
	switch {
	case x.SubscriptionNtf != nil:
		return x.SubscriptionNtf.Type

	case x.VoidedPurchaseNtf != nil:
		return x.VoidedPurchaseNtf.ProductType

	case x.OneTimeProductNtf != nil:
		return 0

	case x.TestNtf != nil:
		return 0

	default:
		return 0
	}
}

func (x *playStoreDevNotification) effect() string {
	switch {
	case x.SubscriptionNtf != nil:
		if x.SubscriptionNtf.shouldRenew() {
			return "renew"
		}

		if x.SubscriptionNtf.shouldCancel() {
			return "cancel"
		}

		return "skip"

	case x.VoidedPurchaseNtf != nil:
		if x.VoidedPurchaseNtf.shouldProcess() {
			return "cancel"
		}

		return "skip"

	default:
		return "skip"
	}
}

func (x *playStoreDevNotification) isBeforeCutoff() bool {
	ems, err := x.EventTimeMilli.Int64()
	if err != nil {
		return true
	}

	cot := time.Date(2024, time.June, 1, 0, 0, 0, 0, time.UTC)

	// Assumption: server time is UTC.
	event := time.UnixMilli(ems)

	return event.Before(cot)
}

func (x *playStoreDevNotification) purchaseToken() (string, bool) {
	switch {
	case x.SubscriptionNtf != nil:
		return x.SubscriptionNtf.PurchaseToken, true

	case x.VoidedPurchaseNtf != nil:
		return x.VoidedPurchaseNtf.PurchaseToken, true

	default:
		return "", false
	}
}

func parsePlayStoreDevNotification(raw []byte) (*playStoreDevNotification, error) {
	wrap := &struct {
		// This might be useful in the futuere for determining environment/channel.
		Sub string `json:"subscription"`

		Message struct {
			Data      string `json:"data"`
			MessageID string `json:"messageId"`
		} `json:"message"`
	}{}

	if err := json.Unmarshal(raw, wrap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	data, err := base64.StdEncoding.DecodeString(wrap.Message.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode message data: %w", err)
	}

	result := &playStoreDevNotification{}
	if err := json.Unmarshal(data, result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal notification: %w", err)
	}

	return result, nil
}

type playStoreSubscriptionNtf struct {
	Type          int    `json:"notificationType"`
	PurchaseToken string `json:"purchaseToken"`
	SubID         string `json:"subscriptionId"`
}

// shouldProcess determines whether x should be processed.
//
// More details https://developer.android.com/google/play/billing/rtdn-reference#sub.
func (x *playStoreSubscriptionNtf) shouldProcess() bool {
	// Other interesting types.
	//
	// - 4 == purchased:
	//     - can be used to create a new order or prepare to working in multi-device mode;
	// - 5 == on hold;
	// - 6 == in grace period;
	// - 10 == paused;
	// - 20 == pending purchase cancelled.

	return x.shouldRenew() || x.shouldCancel()
}

// shouldRenew reports whether the ntf is about renewal.
func (x *playStoreSubscriptionNtf) shouldRenew() bool {
	switch x.Type {
	// Recovered.
	case 1:
		return true

	// Renewed.
	case 2:
		return true

	// Restarted.
	case 7:
		return true

	default:
		return false
	}
}

// shouldCancel reports whether the ntf is about cancellation.
func (x *playStoreSubscriptionNtf) shouldCancel() bool {
	switch x.Type {
	// Cancelled.
	case 3:
		return true

	// Revoked.
	case 12:
		return true

	// Expired.
	case 13:
		return true

	default:
		return false
	}
}

type playStoreVoidedPurchaseNtf struct {
	ProductType   int    `json:"productType"`
	RefundType    int    `json:"refundType"`
	PurchaseToken string `json:"purchaseToken"`
}

// shouldProcess determines whether x should be processed.
func (x *playStoreVoidedPurchaseNtf) shouldProcess() bool {
	switch x.ProductType {
	// Sub.
	case 1:
		return true

	// One-time.
	case 2:
		return false

	default:
		return false
	}
}
