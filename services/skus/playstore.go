package skus

import (
	"context"
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
		return nil
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
