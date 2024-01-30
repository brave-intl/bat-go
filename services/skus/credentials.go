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

	"github.com/jmoiron/sqlx"
	"github.com/linkedin/goavro"
	uuid "github.com/satori/go.uuid"
	kafka "github.com/segmentio/kafka-go"

	"github.com/brave-intl/bat-go/libs/backoff/retrypolicy"
	"github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/libs/clients/cbr"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/datastore"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/jsonutils"
	"github.com/brave-intl/bat-go/libs/ptr"

	"github.com/brave-intl/bat-go/services/skus/model"
)

const (
	defaultMaxTokensPerIssuer       = 4000000 // ~1M BAT
	defaultCohort             int16 = 1
)

var (
	ErrOrderUnpaid                   = errors.New("order not paid")
	ErrOrderHasNoItems   model.Error = "order has no items"
	ErrCredsAlreadyExist model.Error = "credentials already exist"

	errInvalidIssuerResp      model.Error = "invalid issuer response"
	errInvalidNCredsSingleUse model.Error = "submitted more blinded creds than quantity of order item"
	errInvalidNCredsTlv2      model.Error = "submitted more blinded creds than allowed for order"
	errUnsupportedCredType    model.Error = "unsupported credential type"
	errItemDoesNotExist       model.Error = "order item does not exist for order"
	errCredsAlreadySubmitted  model.Error = "credentials already submitted"

	errExceededMaxActiveOrderCreds model.Error = "maximum active order credentials exceeded"

	defaultExpiresAt = time.Now().Add(17532 * time.Hour) // 2 years
	retryPolicy      = retrypolicy.DefaultRetry
	dontRetryCodes   = map[int]struct{}{
		http.StatusBadRequest:          struct{}{},
		http.StatusUnauthorized:        struct{}{},
		http.StatusForbidden:           struct{}{},
		http.StatusInternalServerError: struct{}{},
		http.StatusConflict:            struct{}{},
	}
)

// CreateIssuer creates a new v1 issuer if it does not exist.
//
// This only happens in the event of a new sku being created.
func (s *Service) CreateIssuer(ctx context.Context, dbi sqlx.QueryerContext, merchID string, item *OrderItem) error {
	encMerchID, err := encodeIssuerID(merchID, item.SKU)
	if err != nil {
		return errorutils.Wrap(err, "error encoding issuer name")
	}

	_, err = s.issuerRepo.GetByMerchID(ctx, dbi, encMerchID)
	// Found, nothing to do.
	if err == nil {
		return nil
	}

	if !errors.Is(err, model.ErrIssuerNotFound) {
		return fmt.Errorf("error get issuer for issuerID %s: %w", encMerchID, err)
	}

	reqFn := func() (interface{}, error) {
		return nil, s.cbClient.CreateIssuer(ctx, encMerchID, defaultMaxTokensPerIssuer)
	}

	// The create issuer endpoint returns a conflict if the issuer already exists.
	_, err = s.retry(ctx, reqFn, retryPolicy, canRetry(dontRetryCodes))
	if err != nil && !isConflict(err) {
		return fmt.Errorf("error calling cbr create issuer: %w", err)
	}

	reqFn = func() (interface{}, error) {
		return s.cbClient.GetIssuer(ctx, encMerchID)
	}

	resp, err := s.retry(ctx, reqFn, retryPolicy, canRetry(dontRetryCodes))
	if err != nil {
		return fmt.Errorf("error getting issuer %s: %w", encMerchID, err)
	}

	issuerResp, ok := resp.(*cbr.IssuerResponse)
	if !ok {
		return errInvalidIssuerResp
	}

	if _, err := s.issuerRepo.Create(ctx, dbi, model.IssuerNew{
		MerchantID: issuerResp.Name,
		PublicKey:  issuerResp.PublicKey,
	}); err != nil {
		return fmt.Errorf("error creating new issuer: %w", err)
	}

	return nil
}

// CreateIssuerV3 creates a new v3 issuer if it does not exist.
//
// This only happens in the event of a new sku being created.
func (s *Service) CreateIssuerV3(ctx context.Context, dbi sqlx.QueryerContext, merchID string, item *OrderItem, issuerCfg model.IssuerConfig) error {
	encMerchID, err := encodeIssuerID(merchID, item.SKU)
	if err != nil {
		return errorutils.Wrap(err, "error encoding issuer name")
	}

	_, err = s.issuerRepo.GetByMerchID(ctx, dbi, encMerchID)
	// Found, nothing to do.
	if err == nil {
		return nil
	}

	if !errors.Is(err, model.ErrIssuerNotFound) {
		return fmt.Errorf("error get issuer for issuerID %s: %w", encMerchID, err)
	}

	if item.EachCredentialValidForISO == nil {
		return fmt.Errorf("error each credential valid iso is empty for order item sku %s", item.SKU)
	}

	req := cbr.IssuerRequest{
		Name:      encMerchID,
		Cohort:    defaultCohort,
		MaxTokens: defaultMaxTokensPerIssuer,
		ValidFrom: ptr.FromTime(time.Now()),
		ExpiresAt: ptr.FromTime(defaultExpiresAt),
		Duration:  *item.EachCredentialValidForISO,
		Buffer:    issuerCfg.Buffer,
		Overlap:   issuerCfg.Overlap,
	}

	reqFn := func() (interface{}, error) {
		return nil, s.cbClient.CreateIssuerV3(ctx, req)
	}

	// The create issuer v3 endpoints returns a conflict if the issuer already exists.
	_, err = s.retry(ctx, reqFn, retryPolicy, canRetry(dontRetryCodes))
	if err != nil && !isConflict(err) {
		return fmt.Errorf("error calling cbr create issuer v3: %w", err)
	}

	reqFn = func() (interface{}, error) {
		return s.cbClient.GetIssuerV3(ctx, req.Name)
	}

	resp, err := s.retry(ctx, reqFn, retryPolicy, canRetry(dontRetryCodes))
	if err != nil {
		return fmt.Errorf("error getting issuer %s: %w", req.Name, err)
	}

	issuerResp, ok := resp.(*cbr.IssuerResponse)
	if !ok {
		return errInvalidIssuerResp
	}

	if _, err := s.issuerRepo.Create(ctx, dbi, model.IssuerNew{
		MerchantID: issuerResp.Name,
		PublicKey:  issuerResp.PublicKey,
	}); err != nil {
		return fmt.Errorf("error creating new issuer: %w", err)
	}

	return nil
}

func canRetry(nonRetrySet map[int]struct{}) func(error) bool {
	return func(err error) bool {
		var eb *errorutils.ErrorBundle
		switch {
		case errors.As(err, &eb):
			if state, ok := eb.Data().(clients.HTTPState); ok {
				if _, ok := nonRetrySet[state.Status]; ok {
					return false
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

// CreateOrderItemCredentials creates credentials for the given order id and item with the supplied blinded credentials.
//
// It handles only paid orders.
func (s *Service) CreateOrderItemCredentials(ctx context.Context, orderID, itemID, requestID uuid.UUID, blindedCreds []string) error {
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

	var orderItem *OrderItem
	for _, item := range order.Items {
		if item.ID == itemID {
			orderItem = &item
			break
		}
	}

	if orderItem == nil {
		return errItemDoesNotExist
	}

	if err := s.doCredentialsExist(ctx, orderItem, blindedCreds); err != nil {
		if errors.Is(err, errCredsAlreadySubmitted) {
			return nil
		}

		return err
	}

	if err := checkNumBlindedCreds(order, orderItem, len(blindedCreds)); err != nil {
		return err
	}

	issuerID, err := encodeIssuerID(order.MerchantID, orderItem.SKU)
	if err != nil {
		return errorutils.Wrap(err, "error encoding issuer name")
	}

	issuer, err := s.issuerRepo.GetByMerchID(ctx, s.Datastore.RawDB(), issuerID)
	if err != nil {
		return fmt.Errorf("error getting issuer for issuerID %s: %w", issuerID, err)
	}

	metadata := &Metadata{
		ItemID:         orderItem.ID,
		OrderID:        order.ID,
		IssuerID:       issuer.ID,
		CredentialType: orderItem.CredentialType,
	}

	associatedData, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("error serializing associated data: %w", err)
	}

	signReq := SigningOrderRequest{
		RequestID: requestID.String(),
		Data: []SigningOrder{
			{
				IssuerType:     issuerID,
				IssuerCohort:   defaultCohort,
				BlindedTokens:  blindedCreds,
				AssociatedData: associatedData,
			},
		},
	}

	if err := s.Datastore.InsertSigningOrderRequestOutbox(ctx, requestID, order.ID, orderItem.ID, signReq); err != nil {
		return fmt.Errorf("error inserting signing order request outbox orderID %s: %w", order.ID, err)
	}

	return nil
}

func (s *Service) doCredentialsExist(ctx context.Context, item *model.OrderItem, blindedCreds []string) error {
	switch item.CredentialType {
	case timeLimitedV2:
		// NOTE: This creates a possible race to submit between clients.
		// Multiple signing request outboxes can be created since their
		// uniqueness constraint is on the request id.
		// Despite this, the uniqueness constraint of time_limited_v2_order_creds ensures that
		// only one set of credentials is written for each order / item & time interval.
		// As a result, one client will successfully unblind the credentials and
		// the others will fail.

		return s.doTLV2Exist(ctx, item, blindedCreds)
	default:
		return s.doCredsExist(ctx, item)
	}
}

func (s *Service) doTLV2Exist(ctx context.Context, item *model.OrderItem, blindedCreds []string) error {
	if item.CredentialType != timeLimitedV2 {
		return errUnsupportedCredType
	}

	// Check TLV2 to see if we have credentials signed that match incoming blinded tokens.
	alreadySubmitted, err := s.Datastore.AreTimeLimitedV2CredsSubmitted(ctx, blindedCreds...)
	if err != nil {
		return fmt.Errorf("error validating credentials exist for order item: %w", err)
	}

	if alreadySubmitted {
		// No need to create order credentials, since these are already submitted.
		return errCredsAlreadySubmitted
	}

	// Check if we have signed credentials for this order item.
	// If there is no order and no creds, we can submit again.
	// Similar to the outbox check case, delete order creds will wipe out any already signed order creds.
	creds, err := s.Datastore.GetTimeLimitedV2OrderCredsByOrderItem(item.ID)
	if err != nil {
		return fmt.Errorf("error validating no credentials exist for order item: %w", err)
	}

	if creds != nil {
		return ErrCredsAlreadyExist
	}

	return nil
}

func (s *Service) doCredsExist(ctx context.Context, item *model.OrderItem) error {
	if item.CredentialType == timeLimitedV2 {
		return errUnsupportedCredType
	}

	// Check if we already have a signing request for this order, delete order creds will
	// delete the prior signing request.
	// This allows subscriptions to manage how many order creds are handed out.
	signingOrderRequests, err := s.Datastore.GetSigningOrderRequestOutboxByOrderItem(ctx, item.ID)
	if err != nil {
		return fmt.Errorf("error validating no credentials exist for order item: %w", err)
	}

	if len(signingOrderRequests) > 0 {
		return ErrCredsAlreadyExist
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
			Key:   messages[i].RequestID.Bytes(),
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

// Decode decodes the kafka message using from the avro schema.
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

// SignedOrderCredentialsHandler - this is the handler for getting the signed order credentials
type SignedOrderCredentialsHandler struct {
	decoder   Decoder
	datastore Datastore
}

// Handle processes Kafka message of type SigningOrderResult.
func (s *SignedOrderCredentialsHandler) Handle(ctx context.Context, message kafka.Message) (err error) {
	signedOrderResult, err := s.decoder.Decode(message)
	if err != nil {
		return fmt.Errorf("error decoding message key %s partition %d offset %d: %w",
			string(message.Key), message.Partition, message.Offset, err)
	}

	requestID, err := uuid.FromString(signedOrderResult.RequestID)
	if err != nil {
		return fmt.Errorf("error getting uuid from signed order request %w", err)
	}

	ctx, tx, rollback, commit, err := datastore.GetTx(ctx, s.datastore)
	defer rollback()

	// Check to see if the signing request has not been deleted whilst signing the request.
	sor, err := s.datastore.GetSigningOrderRequestOutboxByRequestID(ctx, tx, requestID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("error get signing order credentials tx: %w", err)
	}

	if sor == nil || sor.CompletedAt != nil {
		return nil
	}

	err = s.datastore.InsertSignedOrderCredentialsTx(ctx, tx, signedOrderResult)
	if err != nil {
		return fmt.Errorf("error inserting signed order credentials: %w", err)
	}

	err = s.datastore.UpdateSigningOrderRequestOutboxTx(ctx, tx, requestID, time.Now())
	if err != nil {
		return fmt.Errorf("error updating signing order request outbox: %w", err)
	}

	err = commit()
	if err != nil {
		return fmt.Errorf("error commiting signing order request outbox: %w", err)
	}

	return nil
}

// Decoder - kafka message decoder interface
type Decoder interface {
	Decode(message kafka.Message) (*SigningOrderResult, error)
}

// SigningOrderResultDecoder - signed order result kafka message decoder interface
type SigningOrderResultDecoder struct {
	codec *goavro.Codec
}

// Decode decodes the kafka message using from the avro schema.
func (s *SigningOrderResultDecoder) Decode(message kafka.Message) (*SigningOrderResult, error) {
	native, _, err := s.codec.NativeFromBinary(message.Value)
	if err != nil {
		return nil, fmt.Errorf("read message: error could not decode naitve from binary %w", err)
	}

	textual, err := s.codec.TextualFromNative(nil, native)
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

// SigningOrderResultErrorHandler - error handler for signing results
type SigningOrderResultErrorHandler struct {
	kafkaWriter *kafka.Writer
}

// Handle writes messages the SigningResultReader's dead letter queue.
func (s *SigningOrderResultErrorHandler) Handle(ctx context.Context, message kafka.Message, errorMessage error) error {
	message.Headers = append(message.Headers, kafka.Header{
		Key:   "error-message",
		Value: []byte(errorMessage.Error()),
	})
	message.Topic = kafkaSignedOrderCredsDLQTopic
	err := s.kafkaWriter.WriteMessages(ctx, message)
	if err != nil {
		return fmt.Errorf("error writting message to signing result dlq: %w", err)
	}
	return nil
}

// DeleteOrderCreds hard-deletes all the order credentials associated with the given orderID.
//
// This includes both time-limited-v2 and single-use credentials.
// The isSigned param only applies to single use and will always be false for time-limited-v2.
// Credentials cannot be deleted when an order is in the process of being signed.
//
// TODO(pavelb):
// - create repos for credentials;
// - move the corresponding methods there;
// - make those methods work on per-item basis.
func (s *Service) DeleteOrderCreds(ctx context.Context, orderID uuid.UUID, reqID uuid.UUID, isSigned bool) error {
	order, err := s.Datastore.GetOrder(orderID)
	if err != nil {
		return err
	}

	if len(order.Items) == 0 {
		return ErrOrderHasNoItems
	}

	doSingleUse, doTlv2 := doItemsHaveSUOrTlv2(order.Items)

	// Handle special cases:
	// - 1 item with time-limited credential type;
	// - multiple items with time-limited credential type.
	if !doSingleUse && !doTlv2 {
		return nil
	}

	tx, err := s.Datastore.RawDB().BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer s.Datastore.RollbackTx(tx)

	if doSingleUse {
		if err := s.Datastore.DeleteSingleUseOrderCredsByOrderTx(ctx, tx, orderID, isSigned); err != nil {
			return fmt.Errorf("error deleting single use order creds: %w", err)
		}
	}

	if doTlv2 {
		if err := s.deleteTLV2(ctx, tx, order, reqID, time.Now()); err != nil {
			return fmt.Errorf("error deleting time limited v2 order creds: %w", err)
		}
	}

	if err := s.Datastore.DeleteSigningOrderRequestOutboxByOrderTx(ctx, tx, orderID); err != nil {
		return fmt.Errorf("error deleting order creds signing in progress")
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error commiting delete order creds: %w", err)
	}

	return nil
}

// maxTLV2ActiveItemCreds is the max number of credentials an item is allowed to have in the given day
const maxTLV2ActiveOrderCreds = 10

func (s *Service) deleteTLV2(ctx context.Context, dbi sqlx.ExtContext, order *model.Order, reqID uuid.UUID, now time.Time) error {

	// Pass the request id as an "item id", which will allow for legacy credentials to be deleted.
	// Otherwise, do not delete said credentials for multiple device support.
	if !uuid.Equal(reqID, uuid.Nil) {
		// check if we already have N active credentials on this item for the current day
		activeCreds, err := s.Datastore.GetCountActiveOrderCreds(ctx, dbi, order.ID, now)
		if err != nil {
			return fmt.Errorf("failed to get count of active order credentials: %w", err)
		}
		if activeCreds > maxTLV2ActiveOrderCreds {
			return errExceededMaxActiveOrderCreds
		}
		return s.Datastore.DeleteTimeLimitedV2OrderCredsByOrderTx(ctx, dbi, order.ID, reqID)
	}

	itemIDs := make([]uuid.UUID, 0, len(order.Items))
	// Legacy, delete all items.
	for i := range order.Items {
		itemIDs = append(itemIDs, order.Items[i].ID)
	}

	return s.Datastore.DeleteTimeLimitedV2OrderCredsByOrderTx(ctx, dbi, order.ID, itemIDs...)
}

// checkNumBlindedCreds checks the number of submitted blinded credentials.
//
// The number of submitted credentials must not exceed:
// - for single-use the quantity of the item;
// - for time-limited-v2 the product of numPerInterval and numIntervals.
func checkNumBlindedCreds(ord *model.Order, item *model.OrderItem, ncreds int) error {
	switch item.CredentialType {
	case singleUse:
		if ncreds > item.Quantity {
			return errInvalidNCredsSingleUse
		}

		return nil
	case timeLimitedV2:
		nperInterval, err := ord.NumPerInterval()
		if err != nil {
			return err
		}

		nintervals, err := ord.NumIntervals()
		if err != nil {
			return err
		}

		if ncreds > nperInterval*nintervals {
			return errInvalidNCredsTlv2
		}

		return nil
	default:
		return nil
	}
}

func doItemsHaveSUOrTlv2(items []model.OrderItem) (bool, bool) {
	var hasSingleUse, hasTlv2 bool

	for i := range items {
		switch items[i].CredentialType {
		case singleUse:
			hasSingleUse = true
		case timeLimitedV2:
			hasTlv2 = true
		}
	}

	return hasSingleUse, hasTlv2
}
