package skus

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"strings"
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
		ReceiptData:            req.Blob,
		ExcludeOldTransactions: true,
	}

	if v.asKey != "" {
		asreq.Password = v.asKey
	}

	resp := &appstore.IAPResponse{}
	if err := v.appStoreCl.Verify(ctx, asreq, resp); err != nil {
		return "", fmt.Errorf("failed to verify receipt: %w", err)
	}

	if len(resp.Receipt.InApp) == 0 {
		return "", errNoInAppTx
	}

	// ProductID on an InApp object must match the SubscriptionID.
	//
	// By doing so we:
	// - find the purchase that is being verified (i.e. to disambiguate VPN from Leo);
	// - utilise Apple verification to make sure the client supplied data (SubscriptionID) is valid and to be trusted.
	item, ok := findInAppBySubID(resp.Receipt.InApp, req.SubscriptionID)
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

// get the public key from the jws header
func extractPublicKey(jwsToken string) (*ecdsa.PublicKey, error) {
	certStr, err := extractHeaderByIndex(jwsToken, 0)
	if err != nil {
		return nil, err
	}

	cert, err := x509.ParseCertificate(certStr)
	if err != nil {
		return nil, err
	}

	switch pk := cert.PublicKey.(type) {
	case *ecdsa.PublicKey:
		return pk, nil
	default:
		return nil, errors.New("appstore public key must be of type ecdsa.PublicKey")
	}
}

func extractHeaderByIndex(tokenStr string, index int) ([]byte, error) {
	if index > 2 {
		return nil, errors.New("invalid index")
	}

	tokenArr := strings.Split(tokenStr, ".")
	headerByte, err := base64.RawStdEncoding.DecodeString(tokenArr[0])
	if err != nil {
		return nil, err
	}

	type Header struct {
		Alg string   `json:"alg"`
		X5c []string `json:"x5c"`
	}
	var header Header
	err = json.Unmarshal(headerByte, &header)
	if err != nil {
		return nil, err
	}

	certByte, err := base64.StdEncoding.DecodeString(header.X5c[index])
	if err != nil {
		return nil, err
	}

	return certByte, nil
}

func verifyCert(certByte, intermediaCertStr []byte) error {
	roots := x509.NewCertPool()
	ok := roots.AppendCertsFromPEM([]byte(appleRootPEM))
	if !ok {
		return errors.New("failed to parse root certificate")
	}

	interCert, err := x509.ParseCertificate(intermediaCertStr)
	if err != nil {
		return errors.New("failed to parse intermedia certificate")
	}
	intermedia := x509.NewCertPool()
	intermedia.AddCert(interCert)

	cert, err := x509.ParseCertificate(certByte)
	if err != nil {
		return err
	}

	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermedia,
	}

	_, err = cert.Verify(opts)
	if err != nil {
		return err
	}

	return nil
}

func findInAppBySubID(iap []appstore.InApp, subID string) (*appstore.InApp, bool) {
	for i := range iap {
		if iap[i].ProductID == subID {
			return &iap[i], true
		}
	}

	return nil, false
}

// rootPEM is from `openssl x509 -inform der -in AppleRootCA-G3.cer -out apple_root.pem`
const appleRootPEM = `
-----BEGIN CERTIFICATE-----
MIICQzCCAcmgAwIBAgIILcX8iNLFS5UwCgYIKoZIzj0EAwMwZzEbMBkGA1UEAwwS
QXBwbGUgUm9vdCBDQSAtIEczMSYwJAYDVQQLDB1BcHBsZSBDZXJ0aWZpY2F0aW9u
IEF1dGhvcml0eTETMBEGA1UECgwKQXBwbGUgSW5jLjELMAkGA1UEBhMCVVMwHhcN
MTQwNDMwMTgxOTA2WhcNMzkwNDMwMTgxOTA2WjBnMRswGQYDVQQDDBJBcHBsZSBS
b290IENBIC0gRzMxJjAkBgNVBAsMHUFwcGxlIENlcnRpZmljYXRpb24gQXV0aG9y
aXR5MRMwEQYDVQQKDApBcHBsZSBJbmMuMQswCQYDVQQGEwJVUzB2MBAGByqGSM49
AgEGBSuBBAAiA2IABJjpLz1AcqTtkyJygRMc3RCV8cWjTnHcFBbZDuWmBSp3ZHtf
TjjTuxxEtX/1H7YyYl3J6YRbTzBPEVoA/VhYDKX1DyxNB0cTddqXl5dvMVztK517
IDvYuVTZXpmkOlEKMaNCMEAwHQYDVR0OBBYEFLuw3qFYM4iapIqZ3r6966/ayySr
MA8GA1UdEwEB/wQFMAMBAf8wDgYDVR0PAQH/BAQDAgEGMAoGCCqGSM49BAMDA2gA
MGUCMQCD6cHEFl4aXTQY2e3v9GwOAEZLuN+yRhHFD/3meoyhpmvOwgPUnPWTxnS4
at+qIxUCMG1mihDK1A3UT82NQz60imOlM27jbdoXt2QfyFMm+YhidDkLF1vLUagM
6BgD56KyKA==
-----END CERTIFICATE-----
`
