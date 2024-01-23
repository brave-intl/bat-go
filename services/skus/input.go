package skus

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/asaskevich/govalidator"
	"github.com/awa/go-iap/appstore"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/square/go-jose"
)

// VerifyCredentialRequestV1 includes an opaque subscription credential blob
type VerifyCredentialRequestV1 struct {
	Version      float64 `json:"version" valid:"-"`
	Type         string  `json:"type" valid:"in(single-use|time-limited|time-limited-v2)"`
	SKU          string  `json:"sku" valid:"-"`
	MerchantID   string  `json:"merchantId" valid:"-"`
	Presentation string  `json:"presentation" valid:"base64"`
}

// GetSku - implement credential interface
func (vcr *VerifyCredentialRequestV1) GetSku(ctx context.Context) string {
	return vcr.SKU
}

// GetType - implement credential interface
func (vcr *VerifyCredentialRequestV1) GetType(ctx context.Context) string {
	return vcr.Type
}

// GetMerchantID - implement credential interface
func (vcr *VerifyCredentialRequestV1) GetMerchantID(ctx context.Context) string {
	return vcr.MerchantID
}

// GetPresentation - implement credential interface
func (vcr *VerifyCredentialRequestV1) GetPresentation(ctx context.Context) string {
	return vcr.Presentation
}

// VerifyCredentialRequestV2 includes an opaque subscription credential blob
type VerifyCredentialRequestV2 struct {
	SKU              string                  `json:"sku" valid:"-"`
	MerchantID       string                  `json:"merchantId" valid:"-"`
	Credential       string                  `json:"credential" valid:"base64"`
	CredentialOpaque *VerifyCredentialOpaque `json:"-" valid:"-"`
}

// GetSku - implement credential interface
func (vcr *VerifyCredentialRequestV2) GetSku(ctx context.Context) string {
	return vcr.SKU
}

// GetType - implement credential interface
func (vcr *VerifyCredentialRequestV2) GetType(ctx context.Context) string {
	if vcr.CredentialOpaque == nil {
		return ""
	}
	return vcr.CredentialOpaque.Type
}

// GetMerchantID - implement credential interface
func (vcr *VerifyCredentialRequestV2) GetMerchantID(ctx context.Context) string {
	return vcr.MerchantID
}

// GetPresentation - implement credential interface
func (vcr *VerifyCredentialRequestV2) GetPresentation(ctx context.Context) string {
	if vcr.CredentialOpaque == nil {
		return ""
	}
	return vcr.CredentialOpaque.Presentation
}

// Decode - implement Decodable interface
func (vcr *VerifyCredentialRequestV2) Decode(ctx context.Context, data []byte) error {
	logger := logging.Logger(ctx, "VerifyCredentialRequestV2.Decode")
	logger.Debug().Msg("starting VerifyCredentialRequestV2.Decode")
	var err error

	if err := json.Unmarshal(data, vcr); err != nil {
		return fmt.Errorf("failed to json decode credential request payload: %w", err)
	}
	// decode the opaque credential
	if vcr.CredentialOpaque, err = credentialOpaqueFromString(vcr.Credential); err != nil {
		return fmt.Errorf("failed to decode opaque credential payload: %w", err)
	}
	return nil
}

// Validate - implement Validable interface
func (vcr *VerifyCredentialRequestV2) Validate(ctx context.Context) error {
	logger := logging.Logger(ctx, "VerifyCredentialRequestV2.Validate")
	var err error
	for _, v := range []interface{}{vcr, vcr.CredentialOpaque} {
		_, err = govalidator.ValidateStruct(v)
		if err != nil {
			logger.Error().Err(err).Msg("failed to validate request")
			return fmt.Errorf("failed to validate verify credential request: %w", err)
		}
	}
	return nil
}

// VerifyCredentialOpaque includes an opaque presentation blob
type VerifyCredentialOpaque struct {
	Type         string  `json:"type" valid:"in(single-use|time-limited|time-limited-v2)"`
	Version      float64 `json:"version" valid:"-"`
	Presentation string  `json:"presentation" valid:"base64"`
}

// credentialOpaqueFromString - given a base64 encoded "credential" unmarshal into a VerifyCredentialOpaque
func credentialOpaqueFromString(s string) (*VerifyCredentialOpaque, error) {
	d, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("failed to base64 decode credential payload: %w", err)
	}
	var vcp = new(VerifyCredentialOpaque)
	if err = json.Unmarshal(d, vcp); err != nil {
		return nil, fmt.Errorf("failed to json decode credential payload: %w", err)
	}
	return vcp, nil
}

const (
	androidSubscriptionUnknown = iota
	androidSubscriptionRecovered
	androidSubscriptionRenewed
	androidSubscriptionCanceled
	androidSubscriptionPurchased
	androidSubscriptionOnHold
	androidSubscriptionInGracePeriod
	androidSubscriptionRestarted
	androidSubscriptionPriceChangeConfirmed
	androidSubscriptionDeferred
	androidSubscriptionPaused
	androidSubscriptionPausedScheduleChanged
	androidSubscriptionRevoked
	androidSubscriptionExpired
)

// SubscriptionNotification - an android subscription notification
type SubscriptionNotification struct {
	Version          string `json:"version"`
	NotificationType int    `json:"notificationType"`
	PurchaseToken    string `json:"purchaseToken"`
	SubscriptionID   string `json:"subscriptionId"`
}

// DeveloperNotification - developer notification details from AndroidNotificationMessage.Data
type DeveloperNotification struct {
	Version                  string                   `json:"version"`
	PackageName              string                   `json:"packageName"`
	SubscriptionNotification SubscriptionNotification `json:"subscriptionNotification"`
}

// AndroidNotificationMessageAttrs - attributes of a notification message
type AndroidNotificationMessageAttrs map[string]string

// AndroidNotificationMessage - wrapping structure of an android notification
type AndroidNotificationMessage struct {
	Attributes AndroidNotificationMessageAttrs `json:"attributes" valid:"-"`
	Data       string                          `json:"data" valid:"base64"`
	MessageID  string                          `json:"messageId" valid:"-"`
}

// Decode - implement Decodable interface
func (anm *AndroidNotificationMessage) Decode(ctx context.Context, data []byte) error {
	logger := logging.Logger(ctx, "AndroidNotificationMessage.Decode")
	logger.Debug().Msg("starting AndroidNotificationMessage.Decode")

	if err := json.Unmarshal(data, anm); err != nil {
		return fmt.Errorf("failed to json decode android notification message: %w", err)
	}
	return nil
}

// Validate - implement Validatable interface
func (anm *AndroidNotificationMessage) Validate(ctx context.Context) error {
	logger := logging.Logger(ctx, "AndroidNotificationMessage.Validate")
	if _, err := govalidator.ValidateStruct(anm); err != nil {
		logger.Error().Err(err).Msg("failed to validate request")
		return fmt.Errorf("failed to validate android notification message: %w", err)
	}
	return nil
}

// GetDeveloperNotification - Extract the developer notification from the android notification message
func (anm *AndroidNotificationMessage) GetDeveloperNotification() (*DeveloperNotification, error) {

	var devNotification = new(DeveloperNotification)
	buf := make([]byte, base64.StdEncoding.DecodedLen(len([]byte(anm.Data))))

	// base64 decode the bytes
	n, err := base64.StdEncoding.Decode(buf, []byte(anm.Data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode input base64: %w", err)
	}
	// read the json values
	if err := json.Unmarshal(buf[:n], devNotification); err != nil {
		return nil, fmt.Errorf("failed to decode input json: %w", err)
	}
	return devNotification, nil
}

// AndroidNotification - wrapping structure of an android notification
type AndroidNotification struct {
	Message      AndroidNotificationMessage `json:"message" valid:"-"`
	Subscription string                     `json:"subscription" valid:"-"`
}

// Decode - implement Decodable interface
func (an *AndroidNotification) Decode(ctx context.Context, data []byte) error {
	logger := logging.Logger(ctx, "AndroidNotification.Decode")
	logger.Debug().Msg("starting AndroidNotification.Decode")

	if err := json.Unmarshal(data, an); err != nil {
		return fmt.Errorf("failed to json decode android notification: %w", err)
	}
	return nil
}

// Validate - implement Validable interface
func (an *AndroidNotification) Validate(ctx context.Context) error {
	logger := logging.Logger(ctx, "AndroidNotification.Validate")
	if _, err := govalidator.ValidateStruct(an); err != nil {
		logger.Error().Err(err).Msg("failed to validate request")
		return fmt.Errorf("failed to validate android notification: %w", err)
	}
	return nil
}

// IOSNotification - wrapping structure of an android notification
type IOSNotification struct {
	payload       []byte                 `json:"-" valid:"-"`
	payloadJWS    *jose.JSONWebSignature `json:"-" valid:"-"`
	SignedPayload string                 `json:"signedPayload" valid:"-"`
	// signed payload is a JWS the payload of which is a base64 encoded
	// responseBodyV2DecodedPayload.  The data attribute of this payload is the JWSTransaction
}

// Decode - implement Decodable interface
func (iosn *IOSNotification) Decode(ctx context.Context, data []byte) error {
	logger := logging.Logger(ctx, "IOSNotification.Decode")
	logger.Debug().Msg("starting IOSNotification.Decode")

	// json unmarshal the notification
	if err := json.Unmarshal(data, iosn); err != nil {
		logger.Error().Msg("failed to json unmarshal body")
		return errorutils.Wrap(err, "error unmarshalling body")
	}

	// parse the jws into payloadJWS from the signed payload
	payload, err := jose.ParseSigned(iosn.SignedPayload)
	if err != nil {
		return fmt.Errorf("failed to parse ios notification: %w", err)
	}

	iosn.payloadJWS = payload

	return nil
}

// Validate - implement Validable interface
func (iosn *IOSNotification) Validate(ctx context.Context) error {
	logger := logging.Logger(ctx, "IOSNotification.Validate")

	// extract the public key from the jws
	pk, err := extractPublicKey(iosn.SignedPayload)
	if err != nil {
		return fmt.Errorf("failed to extract public key in request: %w", err)
	}
	// validate the payloadJWS
	payload, err := iosn.payloadJWS.Verify(pk)
	if err != nil {
		return fmt.Errorf("failed to verify jws payload in request: %w", err)
	}
	logger.Debug().Msg("validated ios notification")

	iosn.payload = payload

	return nil
}

// GetRenewalInfo - from request get renewal information
func (iosn *IOSNotification) GetRenewalInfo(ctx context.Context) (*appstore.JWSRenewalInfoDecodedPayload, error) {
	var (
		resp   = new(appstore.JWSRenewalInfoDecodedPayload)
		logger = logging.Logger(ctx, "IOSNotification.GetRenewalInfo")
	)
	// get the cert from jws header
	rootCertStr, err := extractHeaderByIndex(iosn.SignedPayload, 2)
	if err != nil {
		return nil, err
	}

	intermediaCertStr, err := extractHeaderByIndex(iosn.SignedPayload, 1)
	if err != nil {
		return nil, err
	}

	// verify the cert and intermediates with known root
	if err = verifyCert(rootCertStr, intermediaCertStr); err != nil {
		return nil, err
	}

	// cert is good, extract the public key
	pk, err := extractPublicKey(iosn.SignedPayload)
	if err != nil {
		return nil, err
	}

	// extract the payload from
	payload, err := iosn.payloadJWS.Verify(pk)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to verify the notification jws")
		return nil, fmt.Errorf("failed to verify the notification JWS: %w", err)
	}
	logger.Debug().Msgf("raw payload: %s", string(payload))

	// first get the subscription notification payload decoded
	// req.payload is json serialized appstore.SubscriptionNotificationV2DecodedPayload
	var snv2dp = new(appstore.SubscriptionNotificationV2DecodedPayload)
	if err := json.Unmarshal(payload, snv2dp); err != nil {
		logger.Warn().Err(err).Msg("failed to unmarshal notification")
		return nil, fmt.Errorf("failed to unmarshal subscription notification v2 decoded: %w", err)
	}

	signedRenewalInfo, err := jose.ParseSigned(string(snv2dp.Data.SignedRenewalInfo))
	if err != nil {
		logger.Warn().Err(err).Msg("failed to parse jws")
		return nil, fmt.Errorf("failed to parse the Signed Renewal Info JWS: %w", err)
	}

	// verify
	signedRenewalBytes, err := signedRenewalInfo.Verify(pk)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to verify renewal info")
		return nil, fmt.Errorf("failed to verify the Signed Renewal Info JWS: %w", err)
	}

	// third json unmarshal the resulting output of the jws into a JWSRenewalInfoDecodedPayload (resp)
	if err := json.Unmarshal(signedRenewalBytes, resp); err != nil {
		logger.Warn().Err(err).Msg("failed to json parse renewal info")
		return nil, fmt.Errorf("failed to json parse the Signed Renewal Info JWS: %w", err)
	}

	return resp, nil
}

// GetTransactionInfo - from request get renewal information
func (iosn *IOSNotification) GetTransactionInfo(ctx context.Context) (*appstore.JWSTransactionDecodedPayload, error) {
	var (
		resp   = new(appstore.JWSTransactionDecodedPayload)
		logger = logging.Logger(ctx, "IOSNotification.GetTransactionInfo")
	)

	// get the cert from jws header
	rootCertStr, err := extractHeaderByIndex(iosn.SignedPayload, 2)
	if err != nil {
		return nil, err
	}

	intermediaCertStr, err := extractHeaderByIndex(iosn.SignedPayload, 1)
	if err != nil {
		return nil, err
	}

	// verify the cert and intermediates with known root
	if err = verifyCert(rootCertStr, intermediaCertStr); err != nil {
		return nil, err
	}

	// cert is good, extract the public key
	pk, err := extractPublicKey(iosn.SignedPayload)
	if err != nil {
		return nil, err
	}

	// extract the payload from
	payload, err := iosn.payloadJWS.Verify(pk)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to verify the notification jws")
		return nil, fmt.Errorf("failed to verify the notification JWS: %w", err)
	}
	logger.Debug().Msgf("raw payload: %s", string(payload))

	// first get the subscription notification payload decoded
	// req.payload is json serialized appstore.SubscriptionNotificationV2DecodedPayload
	var snv2dp = new(appstore.SubscriptionNotificationV2DecodedPayload)
	if err := json.Unmarshal(iosn.payload, snv2dp); err != nil {
		logger.Warn().Err(err).Msg("failed to unmarshal notification")
		return nil, fmt.Errorf("failed to unmarshal subscription notification v2 decoded: %w", err)
	}

	// verify the signed transaction jws
	signedTransactionInfo, err := jose.ParseSigned(string(snv2dp.Data.SignedTransactionInfo))
	if err != nil {
		logger.Warn().Err(err).Msg("failed to parse transaction jws")
		return nil, fmt.Errorf("failed to parse the Signed Transaction Info JWS: %w", err)
	}

	// verify
	signedTransactionBytes, err := signedTransactionInfo.Verify(pk)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to verify transaction jws")
		return nil, fmt.Errorf("failed to verify the Signed Transaction Info JWS: %w", err)
	}

	// third json unmarshal the resulting output of the jws into a JWSTransactionDecodedPayload (resp)
	if err := json.Unmarshal(signedTransactionBytes, resp); err != nil {
		logger.Warn().Err(err).Msg("failed to json parse the transaction")
		return nil, fmt.Errorf("failed to json parse the Signed Transaction Info JWS: %w", err)
	}

	return resp, nil
}
