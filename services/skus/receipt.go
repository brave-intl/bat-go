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
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	"google.golang.org/api/androidpublisher/v3"
	"google.golang.org/api/option"

	"github.com/brave-intl/bat-go/services/skus/model"
)

const (
	purchasePendingErrCode    = "purchase_pending"
	purchaseExpiredErrCode    = "purchase_expired"
	purchaseValidationErrCode = "validation_failed"
)

const (
	errNoInAppTx model.Error = "no in app info in response"
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

type androidPublisher struct {
	s *androidpublisher.PurchasesSubscriptionsService
}

func newAndroidPublisher(s *androidpublisher.PurchasesSubscriptionsService) *androidPublisher {
	return &androidPublisher{s: s}
}

func (a *androidPublisher) GetSubscriptionPurchase(_ context.Context, pkgName, subID, token string) (*androidpublisher.SubscriptionPurchase, error) {
	call := a.s.Get(pkgName, subID, token)

	sp, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("error retrieving subscription: %w", err)
	}

	return sp, nil
}

type appStoreVerifier interface {
	Verify(ctx context.Context, req appstore.IAPRequest, result interface{}) error
}

type subscriptionPurchaser interface {
	GetSubscriptionPurchase(ctx context.Context, pkgName, subID, token string) (*androidpublisher.SubscriptionPurchase, error)
}

type receiptVerifier struct {
	appStoreCl          appStoreVerifier
	androidSubPurchaser subscriptionPurchaser
}

func newReceiptVerifier(cl *http.Client, playKey []byte) (*receiptVerifier, error) {
	result := &receiptVerifier{
		appStoreCl: appstore.NewWithClient(cl),
	}

	if playKey != nil && len(playKey) != 0 {

		s, err := androidpublisher.NewService(context.TODO(), option.WithCredentialsJSON(playKey))
		if err != nil {
			return nil, err
		}

		aps := androidpublisher.NewPurchasesSubscriptionsService(s)
		result.androidSubPurchaser = newAndroidPublisher(aps)
	}

	return result, nil
}

// validateApple validates Apple App Store receipt.
func (v *receiptVerifier) validateApple(ctx context.Context, req model.ReceiptRequest) (string, error) {
	l := logging.Logger(ctx, "skus").With().Str("func", "validateReceiptApple").Logger()

	sharedKey, sharedKeyOK := ctx.Value(appctx.AppleReceiptSharedKeyCTXKey).(string)

	asreq := appstore.IAPRequest{
		ReceiptData:            req.Blob,
		ExcludeOldTransactions: true,
	}

	if sharedKeyOK && len(sharedKey) > 0 {
		asreq.Password = sharedKey
	}

	resp := &appstore.IAPResponse{}
	if err := v.appStoreCl.Verify(ctx, asreq, resp); err != nil {
		l.Error().Err(err).Msg("failed to verify receipt")

		return "", fmt.Errorf("failed to verify receipt: %w", err)
	}

	l.Debug().Msg(fmt.Sprintf("%+v", resp))

	if len(resp.Receipt.InApp) < 1 {
		l.Error().Msg("failed to verify receipt: no in app info")
		return "", errNoInAppTx
	}

	return resp.Receipt.InApp[0].OriginalTransactionID, nil
}

const (
	errPurchasePending model.Error = "purchase pending"
	errPurchaseExpired model.Error = "purchase expired"
)

// validateGoogle validates Google Store receipt.
func (v *receiptVerifier) validateGoogle(ctx context.Context, req model.ReceiptRequest) (string, error) {
	sp, err := v.androidSubPurchaser.GetSubscriptionPurchase(ctx, req.Package, req.SubscriptionID, req.Blob)
	if err != nil {
		return "", fmt.Errorf("error retrieving subscription purchase: %w", err)
	}

	if isSubPurchaseExpired(sp.ExpiryTimeMillis, time.Now()) {
		return "", errPurchaseExpired
	}

	if isSubPurchasePending(sp) {
		return "", errPurchasePending
	}

	return req.Blob, nil
}

func isSubPurchaseExpired(expTimeMills int64, now time.Time) bool {
	return now.UnixMilli() > expTimeMills
}

const (
	androidPaymentStatePending         int64 = 0
	androidPaymentStatePendingDeferred int64 = 3
)

func isSubPurchasePending(sp *androidpublisher.SubscriptionPurchase) bool {
	// The payment state is not present for canceled or expired subscriptions.
	if sp.PaymentState == nil {
		return false
	}
	p := *sp.PaymentState
	return p == androidPaymentStatePending || p == androidPaymentStatePendingDeferred
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
