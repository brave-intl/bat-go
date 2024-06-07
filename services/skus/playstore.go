package skus

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"google.golang.org/api/androidpublisher/v3"
	"google.golang.org/api/idtoken"

	"github.com/brave-intl/bat-go/services/skus/model"
)

const (
	errGPSSubPurchaseExpired = model.Error("playstore: subscription purchase expired")
	errGPSSubPurchasePending = model.Error("playstore: subscription purchase pending")
	errGPSAuthHeaderEmpty    = model.Error("playsotre: gcp authorization header is empty")
	errGPSAuthHeaderFmt      = model.Error("playstore: gcp authorization header invalid format")
	errGPSInvalidIssuer      = model.Error("playstore: gcp invalid issuer")
	errGPSInvalidEmail       = model.Error("playstore: gcp invalid email")
	errGPSEmailNotVerified   = model.Error("playstore: gcp email not verified")
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

type gcpTokenValidator interface {
	Validate(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error)
}

type gpsNotificationValidator struct {
	cfg   gpsValidatorConfig
	valid gcpTokenValidator
}

func newGPSNotificationValidator(cfg gpsValidatorConfig, valid gcpTokenValidator) *gpsNotificationValidator {
	result := &gpsNotificationValidator{
		cfg:   cfg,
		valid: valid,
	}

	return result
}

func (g *gpsNotificationValidator) validate(ctx context.Context, r *http.Request) error {
	if g.cfg.disabled {
		return nil
	}

	ah := r.Header.Get("Authorization")
	if ah == "" {
		return errGPSAuthHeaderEmpty
	}

	token := strings.Split(ah, " ")
	if len(token) != 2 {
		return errGPSAuthHeaderFmt
	}

	p, err := g.valid.Validate(ctx, token[1], g.cfg.aud)
	if err != nil {
		return fmt.Errorf("invalid authentication token: %w", err)
	}

	if p.Issuer == "" || p.Issuer != g.cfg.iss {
		return errGPSInvalidIssuer
	}

	if p.Claims["email"] != g.cfg.svcAcct {
		return errGPSInvalidEmail
	}

	if p.Claims["email_verified"] != true {
		return errGPSEmailNotVerified
	}

	return nil
}
