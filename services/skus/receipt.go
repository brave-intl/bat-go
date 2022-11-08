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
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
)

const (
	androidPaymentStatePending int64 = iota
	androidPaymentStatePaid
	androidPaymentStateTrial
	androidPaymentStatePendingDeferred

	androidCancelReasonUser      int64 = 0
	androidCancelReasonSystem    int64 = 1
	androidCancelReasonReplaced  int64 = 2
	androidCancelReasonDeveloper int64 = 3
)

var (
	receiptValidationFns = map[Vendor]func(context.Context, interface{}) (string, error){
		appleVendor:  validateIOSReceipt,
		googleVendor: validateAndroidReceipt,
	}
	iosClient              *appstore.Client
	androidClient          *playstore.Client
	errClientMisconfigured = errors.New("misconfigured client")

	errPurchaseUserCanceled      = errors.New("purchase is canceled by user")
	errPurchaseSystemCanceled    = errors.New("purchase is canceled by google playstore")
	errPurchaseReplacedCanceled  = errors.New("purchase is canceled and replaced")
	errPurchaseDeveloperCanceled = errors.New("purchase is canceled by developer")

	errPurchasePending       = errors.New("purchase is pending")
	errPurchaseDeferred      = errors.New("purchase is deferred")
	errPurchaseStatusUnknown = errors.New("purchase status is unknown")
	errPurchaseFailed        = errors.New("purchase failed")

	errPurchaseExpired = errors.New("purchase expired")

	purchasePendingErrCode       = "purchase_pending"
	purchaseDeferredErrCode      = "purchase_deferred"
	purchaseStatusUnknownErrCode = "purchase_status_unknown"
	purchaseFailedErrCode        = "purchase_failed"
	purchaseValidationErrCode    = "validation_failed"
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

func initClients(ctx context.Context) {

	var logClient = &http.Client{
		Transport: &dumpTransport{},
	}

	logger := logging.Logger(ctx, "skus").With().Str("func", "initClients").Logger()
	iosClient = appstore.New()

	if jsonKey, ok := ctx.Value(appctx.PlaystoreJSONKeyCTXKey).([]byte); ok {
		var err error
		androidClient, err = playstore.NewWithClient(jsonKey, logClient)
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
		if v, ok := receipt.(SubmitReceiptRequestV1); ok {
			logger.Debug().Str("receipt", fmt.Sprintf("%+v", v)).Msg("about to verify subscription")
			// handle v1 receipt type
			resp, err := androidClient.VerifySubscription(ctx, v.Package, v.SubscriptionID, v.Blob)
			if err != nil {
				logger.Error().Err(err).Msg("failed to verify subscription")
				return "", errPurchaseFailed
			}

			// is order expired?
			if time.Unix(0, resp.ExpiryTimeMillis*int64(time.Millisecond)).Before(time.Now()) {
				return "", errPurchaseExpired
			}

			logger.Debug().Msgf("resp: %+v", resp)
			if resp.PaymentState != nil {
				// check that the order was paid
				switch *resp.PaymentState {
				case androidPaymentStatePaid, androidPaymentStateTrial:
					break
				case androidPaymentStatePending:
					// is there a cancel reason?
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
				return v.Blob, nil
			}
			logger.Error().Err(err).Msg("failed to verify subscription: no payment state")
			return "", errPurchaseFailed
		}
	}
	logger.Error().Msg("client is not configured")
	return "", errClientMisconfigured
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
