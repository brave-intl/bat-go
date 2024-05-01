package skus

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/awa/go-iap/appstore"
	"github.com/square/go-jose"

	"github.com/brave-intl/bat-go/services/skus/model"
)

const (
	appleRootCert = `
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
)

const (
	errFailedParseASCert     = model.Error("appstore: failed to parse apple root certificate")
	errInvalidASSNPayload    = model.Error("appstore: invalid assn signed payload")
	errInvalidASSNHeader     = model.Error("appstore: invalid assn header")
	errInvalidASSNPubKeyType = model.Error("appstore: invalid public key type")
	errInvalidASSNCertNum    = model.Error("appstore: invalid number of certificates")
)

type assnHeader struct {
	Alg string   `json:"alg"`
	X5c []string `json:"x5c"`
}

type appStoreSrvNotification struct {
	pubKey *ecdsa.PublicKey
	val    *appstore.SubscriptionNotificationV2DecodedPayload
}

// shouldProcess determines whether x should be processed.
//
// More details https://developer.apple.com/documentation/appstoreservernotifications/notificationtype#4304524.
func (x *appStoreSrvNotification) shouldProcess() bool {
	// Other interesting types.
	//
	// - x.val.NotificationType == appstore.NotificationTypeV2Subscribed && x.val.Subtype == appstore.SubTypeV2InitialBuy:
	//     - the first-time subscription or a family member gained access for the first time;
	//     - can be used to create a new order or prepare to working in multi-device mode;
	// - x.val.NotificationType == appstore.NotificationTypeV2Revoke && x.val.Subtype == "":
	//     - a family member lost access to the subscription.

	return x.shouldRenew() || x.shouldCancel()
}

func (x *appStoreSrvNotification) shouldRenew() bool {
	switch {
	// Auto-renew.
	case x.val.NotificationType == appstore.NotificationTypeV2DidRenew && x.val.Subtype == "":
		return true

	// Billing has recovered.
	case x.val.NotificationType == appstore.NotificationTypeV2DidRenew && x.val.Subtype == appstore.SubTypeV2BillingRecovery:
		return true

	// Resubscribed after cancellation.
	case x.val.NotificationType == appstore.NotificationTypeV2DidChangeRenewalStatus && x.val.Subtype == appstore.SubTypeV2AutoRenewEnabled:
		return true

	// Resubscribed after expiration or a family member gained access after resubscription.
	case x.val.NotificationType == appstore.NotificationTypeV2Subscribed && x.val.Subtype == appstore.SubTypeV2Resubscribe:
		return true

	default:
		return false
	}
}

func (x *appStoreSrvNotification) shouldCancel() bool {
	switch {
	// Cancellation or refund.
	case x.val.NotificationType == appstore.NotificationTypeV2DidChangeRenewalStatus && x.val.Subtype == appstore.SubTypeV2AutoRenewDisabled:
		return true

	// Extra events relating to cancellations or refunds.
	//
	// Need to make sure the order is cancelled.
	// Apple processed a refund.
	case x.val.NotificationType == appstore.NotificationTypeV2Refund && x.val.Subtype == "":
		return true

	// Expiration after user's cancellation.
	case x.val.NotificationType == appstore.NotificationTypeV2Expired && x.val.Subtype == appstore.SubTypeV2Voluntary:
		return true

	// Expiration after billing retry ended without recovery.
	case x.val.NotificationType == appstore.NotificationTypeV2Expired && x.val.Subtype == appstore.SubTypeV2BillingRetry:
		return true

	default:
		return false
	}
}

func parseAppStoreSrvNotification(vrf *assnCertVerifier, spayload string) (*appStoreSrvNotification, error) {
	certs, err := extractASSNCerts(spayload)
	if err != nil {
		return nil, err
	}

	if len(certs) != 3 {
		return nil, errInvalidASSNCertNum
	}

	if err := vrf.verify(certs[2], certs[1]); err != nil {
		return nil, err
	}

	pubKey, err := extractPubKey(certs[0])
	if err != nil {
		return nil, err
	}

	raw, err := jose.ParseSigned(spayload)
	if err != nil {
		return nil, err
	}

	payload, err := raw.Verify(pubKey)
	if err != nil {
		return nil, err
	}

	ntf := &appstore.SubscriptionNotificationV2DecodedPayload{}
	if err := json.Unmarshal(payload, ntf); err != nil {
		return nil, err
	}

	result := &appStoreSrvNotification{
		pubKey: pubKey,
		val:    ntf,
	}

	return result, nil
}

//nolint:unused
func parseRenewalInfo(pubKey *ecdsa.PublicKey, spayload appstore.JWSRenewalInfo) (*appstore.JWSRenewalInfoDecodedPayload, error) {
	raw, err := jose.ParseSigned(string(spayload))
	if err != nil {
		return nil, err
	}

	data, err := raw.Verify(pubKey)
	if err != nil {
		return nil, err
	}

	result := &appstore.JWSRenewalInfoDecodedPayload{}
	if err := json.Unmarshal(data, result); err != nil {
		return nil, err
	}

	return result, nil
}

//nolint:unused
func parseTxnInfo(pubKey *ecdsa.PublicKey, spayload appstore.JWSTransaction) (*appstore.JWSTransactionDecodedPayload, error) {
	raw, err := jose.ParseSigned(string(spayload))
	if err != nil {
		return nil, err
	}

	data, err := raw.Verify(pubKey)
	if err != nil {
		return nil, err
	}

	result := &appstore.JWSTransactionDecodedPayload{}
	if err := json.Unmarshal(data, result); err != nil {
		return nil, err
	}

	return result, nil
}

type assnCertVerifier struct {
	root *x509.CertPool
}

func newASSNCertVerifier() (*assnCertVerifier, error) {
	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM([]byte(appleRootCert)); !ok {
		return nil, errFailedParseASCert
	}

	result := &assnCertVerifier{root: pool}

	return result, nil
}

func (x *assnCertVerifier) verify(rcert, icert *x509.Certificate) error {
	interm := x509.NewCertPool()
	interm.AddCert(icert)

	opts := x509.VerifyOptions{
		Roots:         x.root,
		Intermediates: interm,
	}

	if _, err := rcert.Verify(opts); err != nil {
		return err
	}

	// Consider also verifying the public key.

	return nil
}

func extractASSNCerts(token string) ([]*x509.Certificate, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errInvalidASSNPayload
	}

	hdrRaw, err := base64.RawStdEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, err
	}

	hdr := &assnHeader{}
	if err := json.Unmarshal(hdrRaw, hdr); err != nil {
		return nil, err
	}

	// 0 - public key
	// 1 - intermediate cert
	// 2 - root cert

	if len(hdr.X5c) != 3 {
		return nil, errInvalidASSNHeader
	}

	result := make([]*x509.Certificate, 3)
	for i := range hdr.X5c {
		cert, err := extractASSNCert(hdr.X5c[i])
		if err != nil {
			return nil, err
		}

		result[i] = cert
	}

	return result, nil
}

func extractASSNCert(raw string) (*x509.Certificate, error) {
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}

	return x509.ParseCertificate(data)
}

func extractPubKey(raw *x509.Certificate) (*ecdsa.PublicKey, error) {
	switch result := raw.PublicKey.(type) {
	case *ecdsa.PublicKey:
		return result, nil
	default:
		return nil, errInvalidASSNPubKeyType
	}
}
