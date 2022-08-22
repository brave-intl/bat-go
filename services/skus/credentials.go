package skus

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/brave-intl/bat-go/utils/backoff/retrypolicy"
	"github.com/brave-intl/bat-go/utils/clients"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/jsonutils"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/ptr"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/ptr"
	"github.com/segmentio/kafka-go"

	"github.com/brave-intl/bat-go/libs/backoff/retrypolicy"
	"github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/libs/clients/cbr"
	appctx "github.com/brave-intl/bat-go/libs/context"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/jsonutils"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/ptr"
	"github.com/brave-intl/bat-go/libs/requestutils"
	"github.com/brave-intl/bat-go/libs/requestutils"
	uuid "github.com/satori/go.uuid"
	"github.com/segmentio/kafka-go"
)

const (
	defaultMaxTokensPerIssuer       = 4000000 // ~1M BAT
	defaultCohort             int16 = 1
)

var (
	retryPolicy        = retrypolicy.DefaultRetry
	nonRetriableErrors = []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError, http.StatusConflict}
)

// ErrOrderUnpaid - unpaid order variable
var ErrOrderUnpaid = errors.New("order not paid")

// Issuer includes information about a particular credential issuer
type Issuer struct {
	ID         uuid.UUID `json:"id" db:"id"`
	CreatedAt  time.Time `json:"createdAt" db:"created_at"`
	MerchantID string    `json:"merchantId" db:"merchant_id"`
	PublicKey  string    `json:"publicKey" db:"public_key"`
}

// Name returns the name of the issuer as known by the challenge bypass server
func (issuer *Issuer) Name() string {
	return issuer.MerchantID
}

// CreateIssuer creates a new v1 issuer if it does not exist. This only happens in the event of a new sku being created.
func (s *Service) CreateIssuer(ctx context.Context, merchantID string, orderItem OrderItem) error {
	issuerID, err := encodeIssuerID(merchantID, orderItem.SKU)
	if err != nil {
		return errorutils.Wrap(err, "error encoding issuer name")
	}

	issuer, err := s.Datastore.GetIssuer(issuerID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("error get issuer for issuerID %s: %w", issuerID, err)
	}

	if issuer == nil {
		logging.FromContext(ctx).Info().
			Msgf("creating new issuer %s", issuerID)

		requestOperation := func() (interface{}, error) {
			return nil, s.cbClient.CreateIssuer(ctx, issuerID, defaultMaxTokensPerIssuer)
		}

		// The create issuer endpoint returns a conflict if the issuer already exists
		_, err := s.retry(ctx, requestOperation, retryPolicy, canRetry(nonRetriableErrors))
		if err != nil && !isConflict(err) {
			return fmt.Errorf("error calling cbr create issuer: %w", err)
		}

		requestOperation = func() (interface{}, error) {
			return s.cbClient.GetIssuer(ctx, issuerID)
		}

		response, err := s.retry(ctx, requestOperation, retryPolicy, canRetry(nonRetriableErrors))
		if err != nil {
			return fmt.Errorf("error getting issuer %s: %w", issuerID, err)
		}

		issuerResponse, ok := response.(*cbr.IssuerResponse)
		if !ok {
			return fmt.Errorf("error converting issuer response: %w", err)
		}

		_, err = s.Datastore.InsertIssuer(&Issuer{
			MerchantID: issuerResponse.Name,
			PublicKey:  issuerResponse.PublicKey,
		})
		if err != nil {
			return fmt.Errorf("error creating new issuer: %w", err)
		}
	}

	return nil
}

// CreateIssuerV3 creates a new v3 issuer if it does not exist. This only happens in the event of a new sku being created.
func (s *Service) CreateIssuerV3(ctx context.Context, merchantID string, orderItem OrderItem, issuerConfig IssuerConfig) error {
	issuerID, err := encodeIssuerID(merchantID, orderItem.SKU)
	if err != nil {
		return errorutils.Wrap(err, "error encoding issuer name")
	}

	issuer, err := s.Datastore.GetIssuer(issuerID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("error get issuer for issuerID %s: %w", issuerID, err)
	}

	// Create a new issuer if one is not present.
	if issuer == nil {
		logging.FromContext(ctx).Info().
			Msgf("creating new v3 issuer %s", issuerID)

		if orderItem.ValidForISO == nil {
			return fmt.Errorf("error valid iso is empty for order item sku %s", orderItem.SKU)
		}

		createIssuerV3 := cbr.IssuerRequest{
			Name:      issuerID,
			Cohort:    defaultCohort,
			MaxTokens: defaultMaxTokensPerIssuer,
			ValidFrom: ptr.FromTime(time.Now()),
			Duration:  *orderItem.ValidForISO,
			Buffer:    issuerConfig.buffer,
			Overlap:   issuerConfig.overlap,
		}

		requestOperation := func() (interface{}, error) {
			return nil, s.cbClient.CreateIssuerV3(ctx, createIssuerV3)
		}

		// The create issuer v3 endpoints returns a conflict if the issuer already exists.
		_, err := s.retry(ctx, requestOperation, retryPolicy, canRetry(nonRetriableErrors))
		if err != nil && !isConflict(err) {
			return fmt.Errorf("error calling cbr create issuer v3: %w", err)
		}

		requestOperation = func() (interface{}, error) {
			return s.cbClient.GetIssuerV2(ctx, createIssuerV3.Name, createIssuerV3.Cohort)
		}

		response, err := s.retry(ctx, requestOperation, retryPolicy, canRetry(nonRetriableErrors))
		if err != nil {
			return fmt.Errorf("error getting issuer %s: %w", createIssuerV3.Name, err)
		}

		issuerResponse, ok := response.(*cbr.IssuerResponse)
		if !ok {
			return fmt.Errorf("error converting issuer response: %w", err)
		}

		_, err = s.Datastore.InsertIssuer(&Issuer{
			MerchantID: issuerResponse.Name,
			PublicKey:  issuerResponse.PublicKey,
		})
		if err != nil {
			return fmt.Errorf("error creating new issuer: %w", err)
		}
	}

	return nil
}

func canRetry(nonRetriableErrors []int) func(error) bool {
	return func(err error) bool {
		var eb *errorutils.ErrorBundle
		switch {
		case errors.As(err, &eb):
			if hs, ok := eb.Data().(clients.HTTPState); ok {
				for _, httpStatusCode := range nonRetriableErrors {
					if hs.Status == httpStatusCode {
						return false
					}
				}
				return true
			}
		}
		return false
	}
}

func isConflict(err error) bool {
	var eb *errorutils.ErrorBundle
	if errors.As(err, &eb) {
		if httpState, ok := eb.Data().(clients.HTTPState); ok {
			return httpState.Status == http.StatusConflict
		}
	}
	return false
}

// OrderCreds encapsulates the credentials to be signed in response to a completed order
type OrderCreds struct {
	ID           uuid.UUID                  `json:"id" db:"item_id"`
	OrderID      uuid.UUID                  `json:"orderId" db:"order_id"`
	IssuerID     uuid.UUID                  `json:"issuerId" db:"issuer_id"`
	BlindedCreds jsonutils.JSONStringArray  `json:"blindedCreds" db:"blinded_creds"`
	SignedCreds  *jsonutils.JSONStringArray `json:"signedCreds" db:"signed_creds"`
	BatchProof   *string                    `json:"batchProof" db:"batch_proof"`
	PublicKey    *string                    `json:"publicKey" db:"public_key"`
}

// TimeLimitedCreds encapsulates time-limited credentials
type TimeLimitedCreds struct {
	ID        uuid.UUID `json:"id"`
	OrderID   uuid.UUID `json:"orderId"`
	IssuedAt  string    `json:"issuedAt"`
	ExpiresAt string    `json:"expiresAt"`
	Token     string    `json:"token"`
}

// CreateOrderCredentials - create the order credentials if order is paid
func (s *Service) CreateOrderCredentials(ctx context.Context, orderID uuid.UUID, blindedCreds []string) error {
	order, err := s.Datastore.GetOrder(orderID)
	if err != nil {
		return fmt.Errorf("error retrieving order: %w", err)
	}

	if order == nil {
		return fmt.Errorf("error retrieving orderID %s: %w", orderID, errorutils.ErrNotFound)
	}

	if !order.IsPaid() {
		return ErrOrderUnpaid
	}

	var signingOrderRequests []SigningOrderRequest

	for _, orderItem := range order.Items {
		// generalized issuer based on sku and merchant id
		issuerID, err := encodeIssuerID(order.MerchantID, orderItem.SKU)
		if err != nil {
			return errorutils.Wrap(err, "error encoding issuer name")
		}

		issuer, err := s.Datastore.GetIssuer(issuerID)
		if err != nil {
			return fmt.Errorf("error getting issuer for issuerID %s: %w", issuerID, err)
		}

		requestID, ok := ctx.Value(requestutils.RequestID).(string)
		if !ok {
			return errors.New("error retrieving requestID from context for create order credentials")
		}

		metadata := Metadata{
			ItemID:         orderItem.ID,
			OrderID:        order.ID,
			IssuerID:       issuer.ID,
			CredentialType: orderItem.CredentialType,
		}

		associatedData, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("error serializing associated data: %w", err)
		}

		signingOrderRequest := SigningOrderRequest{
			RequestID: requestID,
			Data: []SigningOrder{
				{
					IssuerType:     issuerID,
					IssuerCohort:   defaultCohort,
					BlindedTokens:  blindedCreds,
					AssociatedData: associatedData,
				},
			},
		}

		signingOrderRequests = append(signingOrderRequests, signingOrderRequest)
	}

	err = s.Datastore.InsertSigningOrderRequestOutbox(ctx, order.ID, signingOrderRequests)
	if err != nil {
		return fmt.Errorf("error inserting signing order request outbox orderID %s: %w", order.ID, err)
	}

	return nil
}

// OrderWorker attempts to work on an order job by signing the blinded credentials of the client
type OrderWorker interface {
	SignOrderCreds(ctx context.Context, orderID uuid.UUID, issuer Issuer, blindedCreds []string) (*OrderCreds, error)
}

// SignOrderCreds signs the blinded credentials
func (s *Service) SignOrderCreds(ctx context.Context, orderID uuid.UUID, issuer Issuer, blindedCreds []string) (*OrderCreds, error) {
	resp, err := s.cbClient.SignCredentials(ctx, issuer.Name(), blindedCreds)
	if err != nil {
		return nil, err
	}

	signedTokens := jsonutils.JSONStringArray(resp.SignedTokens)

	creds := &OrderCreds{
		ID:           orderID,
		BlindedCreds: blindedCreds,
		SignedCreds:  &signedTokens,
		BatchProof:   &resp.BatchProof,
		PublicKey:    &issuer.PublicKey,
	}

	return creds, nil
}

// SigningRequestWriter is the interface implemented by types that can write signing request messages.
type SigningRequestWriter interface {
	WriteMessage(ctx context.Context, message []byte) error
	WriteMessages(ctx context.Context, messages []SigningOrderRequestOutbox) error
}

// WriteMessage writes a single message to the kafka topic configured on this writer.
func (s *Service) WriteMessage(ctx context.Context, message []byte) error {
	native, _, err := s.codecs[kafkaUnsignedOrderCredsTopic].NativeFromTextual(message)
	if err != nil {
		return fmt.Errorf("error converting native from textual: %w", err)
	}

	binary, err := s.codecs[kafkaUnsignedOrderCredsTopic].BinaryFromNative(nil, native)
	if err != nil {
		return fmt.Errorf("error converting binary from native: %w", err)
	}

	err = s.kafkaWriter.WriteMessages(ctx, kafka.Message{
		Topic: kafkaUnsignedOrderCredsTopic,
		Value: binary,
	})
	if err != nil {
		return fmt.Errorf("error writting kafka message: %w", err)
	}

	return nil
}

// WriteMessages writes a batch of SigningOrderRequestOutbox messages to the kafka topic configured on this writer.
func (s *Service) WriteMessages(ctx context.Context, messages []SigningOrderRequestOutbox) error {
	msgs := make([]kafka.Message, len(messages))

	for i := 0; i < len(messages); i++ {

		native, _, err := s.codecs[kafkaUnsignedOrderCredsTopic].NativeFromTextual(messages[i].Message)
		if err != nil {
			return fmt.Errorf("error converting native from textual: %w", err)
		}

		binary, err := s.codecs[kafkaUnsignedOrderCredsTopic].BinaryFromNative(nil, native)
		if err != nil {
			return fmt.Errorf("error converting binary from native: %w", err)
		}

		km := kafka.Message{
			Topic: kafkaUnsignedOrderCredsTopic,
			Key:   messages[i].ID.Bytes(),
			Value: binary,
		}

		msgs[i] = km
	}

	err := s.kafkaWriter.WriteMessages(ctx, msgs...)
	if err != nil {
		return fmt.Errorf("error writting kafka message: %w", err)
	}

	return nil
}

// SigningResultReader is the interface implemented by types that can read SigningOrderResult's.
type SigningResultReader interface {
	FetchMessage(ctx context.Context) (kafka.Message, error)
	CommitMessages(ctx context.Context, messages ...kafka.Message) error
	Decode(message kafka.Message) (*SigningOrderResult, error)
}

// FetchMessage reads and return the next message.
// FetchMessage does not commit offsets automatically when using consumer groups.
func (s *Service) FetchMessage(ctx context.Context) (kafka.Message, error) {
	return s.kafkaSignedRequestReader.FetchMessage(ctx)
}

// CommitMessages commits the list of messages passed as argument.
func (s *Service) CommitMessages(ctx context.Context, messages ...kafka.Message) error {
	return s.kafkaSignedRequestReader.CommitMessages(ctx, messages...)
}

// Decode - perform a kafka message decode
func (s *Service) Decode(message kafka.Message) (*SigningOrderResult, error) {
	codec, ok := s.codecs[kafkaSignedOrderCredsTopic]
	if !ok {
		return nil, fmt.Errorf("read message: could not find codec %s", kafkaSignedOrderCredsTopic)
	}

	native, _, err := codec.NativeFromBinary(message.Value)
	if err != nil {
		return nil, fmt.Errorf("read message: error could not decode naitve from binary %w", err)
	}

	textual, err := codec.TextualFromNative(nil, native)
	if err != nil {
		return nil, fmt.Errorf("read message: error could not decode textual from native %w", err)
	}

	var signedOrderResult SigningOrderResult
	err = json.Unmarshal(textual, &signedOrderResult)
	if err != nil {
		return nil, fmt.Errorf("read message: error could not decode json from textual %w", err)
	}

	return &signedOrderResult, nil
}

// generateCredentialRedemptions - helper to create credential redemptions from cred bindings
var generateCredentialRedemptions = func(ctx context.Context, cb []CredentialBinding) ([]cbr.CredentialRedemption, error) {
	// deduplicate credential bindings
	cb = deduplicateCredentialBindings(cb...)

	var (
		requestCredentials = make([]cbr.CredentialRedemption, len(cb))
		issuers            = make(map[string]*Issuer)
	)

	db, ok := ctx.Value(appctx.DatastoreCTXKey).(Datastore)
	if !ok {
		return nil, errors.New("failed to get datastore from context")
	}

	for i := 0; i < len(cb); i++ {

		var (
			ok     bool
			issuer *Issuer
			err    error
		)

		publicKey := cb[i].PublicKey

		if issuer, ok = issuers[publicKey]; !ok {
			issuer, err = db.GetIssuerByPublicKey(publicKey)
			if err != nil {
				return nil, fmt.Errorf("error finding issuer: %w", err)
			}
		}

		requestCredentials[i].Issuer = issuer.Name()
		requestCredentials[i].TokenPreimage = cb[i].TokenPreimage
		requestCredentials[i].Signature = cb[i].Signature
	}
	return requestCredentials, nil
}

// CredentialBinding includes info needed to redeem a single credential
type CredentialBinding struct {
	PublicKey     string `json:"publicKey" valid:"base64"`
	TokenPreimage string `json:"t" valid:"base64"`
	Signature     string `json:"signature" valid:"base64"`
}

// deduplicateCredentialBindings - given a list of tokens return a deduplicated list
func deduplicateCredentialBindings(tokens ...CredentialBinding) []CredentialBinding {
	var (
		seen   = map[string]bool{}
		result []CredentialBinding
	)
	for _, t := range tokens {
		if !seen[t.TokenPreimage] {
			seen[t.TokenPreimage] = true
			result = append(result, t)
		}
	}
	return result
}

func decodeIssuerID(issuerID string) (string, string, error) {
	var (
		merchantID string
		sku        string
	)

	u, err := url.Parse(issuerID)
	if err != nil {
		return "", "", fmt.Errorf("parse issuer name: %w", err)
	}

	sku = u.Query().Get("sku")
	u.RawQuery = ""
	merchantID = u.String()

	return merchantID, sku, nil
}

func encodeIssuerID(merchantID, sku string) (string, error) {
	v := url.Values{}
	v.Add("sku", sku)

	u, err := url.Parse(merchantID + "?" + v.Encode())
	if err != nil {
		return "", fmt.Errorf("parse merchant id: %w", err)
	}

	return u.String(), nil
}
