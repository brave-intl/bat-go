package skus

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/brave-intl/bat-go/libs/backoff"
	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/brave-intl/bat-go/libs/datastore"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/logging"
	srv "github.com/brave-intl/bat-go/libs/service"
	timeutils "github.com/brave-intl/bat-go/libs/time"
	"github.com/brave-intl/bat-go/libs/wallet/provider"
	"github.com/brave-intl/bat-go/libs/wallet/provider/uphold"
	"github.com/brave-intl/bat-go/services/wallet"
	"github.com/linkedin/goavro"

	"github.com/brave-intl/bat-go/libs/clients/cbr"
	"github.com/brave-intl/bat-go/libs/clients/gemini"
	appctx "github.com/brave-intl/bat-go/libs/context"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	kafkautils "github.com/brave-intl/bat-go/libs/kafka"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
	uuid "github.com/satori/go.uuid"
	"github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/checkout/session"
	"github.com/stripe/stripe-go/v72/client"
	"github.com/stripe/stripe-go/v72/sub"
)

var (
	voteTopic = os.Getenv("ENV") + ".payment.vote"

	// TODO address in kafka refactor. Check topics are correct
	// kafka topic for requesting order credentials are signed, write to by sku service
	kafkaUnsignedOrderCredsTopic = os.Getenv("GRANT_CBP_SIGN_PRODUCER_TOPIC")

	// kafka topic which receives order creds once they have been signed, read by sku service
	kafkaSignedOrderCredsTopic      = os.Getenv("GRANT_CBP_SIGN_CONSUMER_TOPIC")
	kafkaSignedRequestReaderGroupID = os.Getenv("KAFKA_CONSUMER_GROUP_SIGNED_ORDER_CREDENTIALS")
)

const (
	// OrderStatusCanceled - string literal used in db for canceled status
	OrderStatusCanceled = "canceled"
	// OrderStatusPaid - string literal used in db for canceled status
	OrderStatusPaid = "paid"
	// OrderStatusPending - string literal used in db for pending status
	OrderStatusPending = "pending"
)

// Default issuer V3 config default values
const (
	defaultBuffer  = 30
	defaultOverlap = 5
)

// Service contains datastore
type Service struct {
	wallet                   *wallet.Service
	cbClient                 cbr.Client
	geminiClient             gemini.Client
	geminiConf               *gemini.Conf
	scClient                 *client.API
	Datastore                Datastore
	codecs                   map[string]*goavro.Codec
	kafkaWriter              *kafka.Writer
	kafkaDialer              *kafka.Dialer
	jobs                     []srv.Job
	pauseVoteUntil           time.Time
	pauseVoteUntilMu         sync.RWMutex
	kafkaSignedRequestReader kafkautils.Consumer
	retry                    backoff.RetryFunc
}

// PauseWorker - pause worker until time specified
func (s *Service) PauseWorker(until time.Time) {
	s.pauseVoteUntilMu.Lock()
	defer s.pauseVoteUntilMu.Unlock()
	s.pauseVoteUntil = until
}

// IsPaused - is the worker paused?
func (s *Service) IsPaused() bool {
	s.pauseVoteUntilMu.RLock()
	defer s.pauseVoteUntilMu.RUnlock()
	return time.Now().Before(s.pauseVoteUntil)
}

// Jobs - Implement srv.JobService interface
func (s *Service) Jobs() []srv.Job {
	return s.jobs
}

// InitKafka by creating a kafka writer and creating local copies of codecs
func (s *Service) InitKafka(ctx context.Context) error {

	// TODO: eventually as cobra/viper
	ctx = context.WithValue(ctx, appctx.KafkaBrokersCTXKey, os.Getenv("KAFKA_BROKERS"))

	// TODO address in kafka refactor
	// passing an empty string will not set topic on writer, so it can be defined at message write time
	var err error
	s.kafkaWriter, s.kafkaDialer, err = kafkautils.InitKafkaWriter(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to initialize kafka: %w", err)
	}

	s.kafkaSignedRequestReader, err = kafkautils.NewKafkaReader(ctx, kafkaSignedRequestReaderGroupID,
		kafkaSignedOrderCredsTopic)
	if err != nil {
		return fmt.Errorf("failed to initialize kafka sigend order credentials reader: %w", err)
	}

	s.codecs, err = kafkautils.GenerateCodecs(map[string]string{
		"vote":                       voteSchema,
		kafkaUnsignedOrderCredsTopic: signingOrderRequestSchema,
		kafkaSignedOrderCredsTopic:   signingOrderResultSchema,
	})

	if err != nil {
		return fmt.Errorf("failed to generate codecs kafka: %w", err)
	}
	return nil
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(ctx context.Context, datastore Datastore, walletService *wallet.Service) (service *Service, err error) {
	sublogger := logging.Logger(ctx, "payments").With().Str("func", "InitService").Logger()
	// setup the in app purchase clients
	initClients(ctx)

	// setup stripe if exists in context and enabled
	var scClient = &client.API{}
	if enabled, ok := ctx.Value(appctx.StripeEnabledCTXKey).(bool); ok && enabled {
		sublogger.Debug().Msg("stripe enabled")
		stripe.Key, err = appctx.GetStringFromContext(ctx, appctx.StripeSecretCTXKey)
		if err != nil {
			sublogger.Panic().Err(err).Msg("failed to get Stripe secret from context, and Stripe enabled")
		}
		// initialize stripe client
		scClient.Init(stripe.Key, nil)
	}

	cbClient, err := cbr.New()
	if err != nil {
		return nil, err
	}

	var (
		geminiClient gemini.Client
		geminiConf   *gemini.Conf
	)
	if os.Getenv("GEMINI_ENABLED") == "true" {
		apiKey, clientID, settlementAddress, apiSecret, err := getGeminiInfoFromCtx(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get gemini info: %w", err)
		}
		// get the correct env variables for bulk pay API call
		geminiConf = &gemini.Conf{
			ClientID:          clientID,
			APIKey:            apiKey,
			Secret:            apiSecret,
			SettlementAddress: settlementAddress,
		}

		geminiClient, err = gemini.New()
		if err != nil {
			return nil, fmt.Errorf("failed to create gemini client: %w", err)
		}
	}

	service = &Service{
		wallet:           walletService,
		geminiClient:     geminiClient,
		geminiConf:       geminiConf,
		cbClient:         cbClient,
		scClient:         scClient,
		Datastore:        datastore,
		pauseVoteUntilMu: sync.RWMutex{},
		retry:            backoff.Retry,
	}

	// setup runnable jobs
	service.jobs = []srv.Job{
		{
			Func:    service.RunNextVoteDrainJob,
			Cadence: 2 * time.Second,
			Workers: 1,
		},
		{
			Func:    service.RunNextOrderJob,
			Cadence: 500 * time.Millisecond,
			Workers: 3,
		},
		{
			Func:    service.RunSendSigningRequestJob,
			Cadence: 100 * time.Millisecond,
			Workers: 1,
		},
		{
			Func:    service.RunStoreSignedOrderCredentialsJob,
			Cadence: 200 * time.Millisecond,
			Workers: 1,
		},
	}

	err = service.InitKafka(ctx)
	if err != nil {
		return nil, err
	}

	return service, nil
}

// CreateOrderFromRequest creates an order from the request
func (s *Service) CreateOrderFromRequest(ctx context.Context, req CreateOrderRequest) (*Order, error) {
	totalPrice := decimal.New(0, 0)
	var (
		currency              string
		orderItems            []OrderItem
		location              string
		validFor              *time.Duration
		stripeSuccessURI      string
		stripeCancelURI       string
		status                string
		allowedPaymentMethods = new(Methods)
		merchantID            = "brave.com"
	)

	for i := 0; i < len(req.Items); i++ {
		orderItem, pm, issuerConfig, err := s.CreateOrderItemFromMacaroon(ctx, req.Items[i].SKU, req.Items[i].Quantity)
		if err != nil {
			return nil, err
		}

		// Create issuer for sku. This only happens when a new sku is created.
		switch orderItem.CredentialType {
		case singleUse:
			err = s.CreateIssuer(ctx, merchantID, *orderItem)
			if err != nil {
				return nil, errorutils.Wrap(err, "error finding issuer")
			}
		case timeLimitedV2:
			err = s.CreateIssuerV3(ctx, merchantID, *orderItem, *issuerConfig)
			if err != nil {
				return nil, fmt.Errorf("error creating issuer for merchantID %s and sku %s: %w",
					merchantID, orderItem.SKU, err)
			}
		}

		// make sure all the order item skus have the same allowed Payment Methods
		if i >= 1 {
			if !allowedPaymentMethods.Equal(pm) {
				return nil, errors.New("all order items must have the same allowed payment methods")
			}
		} else {
			// first order item
			*allowedPaymentMethods = *pm
		}

		totalPrice = totalPrice.Add(orderItem.Subtotal)

		if location == "" {
			location = orderItem.Location.String
		}

		if orderItem.ValidFor != nil {
			validFor = new(time.Duration)
			*validFor = *orderItem.ValidFor
		}

		if location != orderItem.Location.String {
			return nil, errors.New("all order items must be from the same location")
		}
		if currency == "" {
			currency = orderItem.Currency
		}
		if currency != orderItem.Currency {
			return nil, errors.New("all order items must be the same currency")
		}
		// stripe related
		if stripeSuccessURI == "" {
			stripeSuccessURI = orderItem.Metadata["stripe_success_uri"]
		} else if stripeSuccessURI != orderItem.Metadata["stripe_success_uri"] {
			return nil, errors.New("all order items must have same stripe success uri")
		}
		if stripeCancelURI == "" {
			stripeCancelURI = orderItem.Metadata["stripe_cancel_uri"]
		} else if stripeCancelURI != orderItem.Metadata["stripe_cancel_uri"] {
			return nil, errors.New("all order items must have same stripe cancel uri")
		}

		orderItems = append(orderItems, *orderItem)
	}

	// If order consists entirely of zero cost items ( e.g. trials ), we can consider it paid
	if totalPrice.IsZero() {
		status = OrderStatusPaid
	} else {
		status = OrderStatusPending
	}

	order, err := s.Datastore.CreateOrder(totalPrice, merchantID, status, currency,
		location, validFor, orderItems, allowedPaymentMethods)

	if err != nil {
		return nil, fmt.Errorf("failed to create order: %w", err)
	}

	if !order.IsPaid() && order.IsStripePayable() {
		// brand-new order, contains an email in the request
		checkoutSession, err := order.CreateStripeCheckoutSession(
			req.Email,
			parseURLAddOrderIDParam(stripeSuccessURI, order.ID),
			parseURLAddOrderIDParam(stripeCancelURI, order.ID),
			order.getTrialDays(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create checkout session: %w", err)
		}

		err = s.Datastore.UpdateOrderMetadata(order.ID, "stripeCheckoutSessionId", checkoutSession.SessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to update order metadata: %w", err)
		}
	}

	return order, err
}

// GetOrder - business logic for getting an order, needs to validate the checkout session is not expired
func (s *Service) GetOrder(orderID uuid.UUID) (*Order, error) {
	// get the order
	order, err := s.Datastore.GetOrder(orderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get order (%s): %w", orderID.String(), err)
	}

	if order != nil {
		if !order.IsPaid() && order.IsStripePayable() {
			order, err = s.TransformStripeOrder(order)
			if err != nil {
				return nil, fmt.Errorf("failed to transform stripe order (%s): %w", orderID.String(), err)
			}
		}
	}

	return order, nil

}

// TransformStripeOrder - update checkout session if expired, check the status of the checkout session
func (s *Service) TransformStripeOrder(order *Order) (*Order, error) {

	// check if this order has an expired checkout session
	expired, cs, err := s.Datastore.CheckExpiredCheckoutSession(order.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to check for expired stripe checkout session: %w", err)
	}

	if expired {
		// get old checkout session from stripe by id
		stripeSession, err := session.Get(cs, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get stripe checkout session: %w", err)
		}

		checkoutSession, err := order.CreateStripeCheckoutSession(
			getEmailFromCheckoutSession(stripeSession),
			stripeSession.SuccessURL, stripeSession.CancelURL,
			order.getTrialDays(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create checkout session: %w", err)
		}

		err = s.Datastore.UpdateOrderMetadata(order.ID, "stripeCheckoutSessionId", checkoutSession.SessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to update order metadata: %w", err)
		}

	}

	// if this is a stripe order, and there is a checkout session, we actually need to check it with
	// stripe, as the redirect flow sometimes is too fast for the webhook to be delivered.
	if cs, ok := order.Metadata["stripeCheckoutSessionId"]; ok && cs != "" {
		// get old checkout session from stripe by id
		stripeSession, err := session.Get(cs, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get stripe checkout session: %w", err)
		}

		if stripeSession.PaymentStatus == "paid" {
			// if the session is actually paid, then set the subscription id and order to paid
			if err = s.Datastore.UpdateOrder(order.ID, "paid"); err != nil {
				return nil, fmt.Errorf("failed to update order to paid status: %w", err)
			}
			err = s.Datastore.UpdateOrderMetadata(order.ID, "stripeSubscriptionId", stripeSession.Subscription.ID)
			if err != nil {
				return nil, fmt.Errorf("failed to update order to add the subscription id")
			}
		}
	}

	// get the order latest state
	order, err = s.Datastore.GetOrder(order.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	return order, nil
}

// CancelOrder - cancels an order, propagates to stripe if needed
func (s *Service) CancelOrder(orderID uuid.UUID) error {
	// check the order, do we have a stripe subscription?
	ok, subID, err := s.Datastore.IsStripeSub(orderID)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check stripe subscription: %w", err)
	}
	if ok && subID != "" {
		// cancel the stripe subscription
		if _, err := sub.Cancel(subID, nil); err != nil {
			return fmt.Errorf("failed to cancel stripe subscription: %w", err)
		}
	}
	return s.Datastore.UpdateOrder(orderID, OrderStatusCanceled)
}

// SetOrderTrialDays set the order's free trial days
func (s *Service) SetOrderTrialDays(ctx context.Context, orderID *uuid.UUID, days int64) error {
	// get the order
	order, err := s.Datastore.SetOrderTrialDays(ctx, orderID, days)
	if err != nil {
		return fmt.Errorf("failed to set the order's trial days: %w", err)
	}

	// recreate the stripe checkout session now that we have set the trial days on this order
	if !order.IsPaid() && order.IsStripePayable() {
		// get old checkout session from stripe by id
		stripeSession, err := session.Get(order.Metadata["stripeCheckoutSessionId"], nil)
		if err != nil {
			return fmt.Errorf("failed to get stripe checkout session: %w", err)
		}

		checkoutSession, err := order.CreateStripeCheckoutSession(
			getEmailFromCheckoutSession(stripeSession),
			stripeSession.SuccessURL, stripeSession.CancelURL,
			order.getTrialDays(),
		)
		if err != nil {
			return fmt.Errorf("failed to create checkout session: %w", err)
		}

		// overwrite the old checkout session
		err = s.Datastore.UpdateOrderMetadata(order.ID, "stripeCheckoutSessionId", checkoutSession.SessionID)
		if err != nil {
			return fmt.Errorf("failed to update order metadata: %w", err)
		}
	}

	return nil
}

// UpdateOrderStatus checks to see if an order has been paid and updates it if so
func (s *Service) UpdateOrderStatus(orderID uuid.UUID) error {
	// get the order
	order, err := s.Datastore.GetOrder(orderID)
	if err != nil {
		return err
	}

	sum, err := s.Datastore.GetSumForTransactions(orderID)
	if err != nil {
		return err
	}

	if sum.GreaterThanOrEqual(order.TotalPrice) {
		err = s.Datastore.UpdateOrder(orderID, "paid")
		if err != nil {
			return err
		}
	}

	return nil
}

// UpdateOrderMetadata updates the metadata on an order
func (s *Service) UpdateOrderMetadata(orderID uuid.UUID, key string, value string) error {
	err := s.Datastore.UpdateOrderMetadata(orderID, key, value)
	if err != nil {
		return err
	}
	return nil
}

// getCustodialTxFn - type definition of a get custodial tx function
// return amount, status, currency, kind, err
type getCustodialTxFn func(context.Context, string) (*decimal.Decimal, string, string, string, error)

// get the uphold tx based on txRef
func getUpholdCustodialTx(ctx context.Context, txRef string) (*decimal.Decimal, string, string, string, error) {
	var wallet uphold.Wallet
	upholdTransaction, err := wallet.GetTransaction(ctx, txRef)

	if err != nil {
		return nil, "", "", "", err
	}

	amount := upholdTransaction.AltCurrency.FromProbi(upholdTransaction.Probi)
	status := upholdTransaction.Status
	currency := upholdTransaction.AltCurrency.String()
	custodian := "uphold"

	// check if destination is the right address
	if upholdTransaction.Destination != uphold.UpholdSettlementAddress {
		return nil, "", "", custodian, errors.New("error recording transaction: invalid settlement address")
	}

	return &amount, status, currency, custodian, nil
}

// getUpholdCustodialTxWithRetries - the the custodial tx information from uphold with retries
func getUpholdCustodialTxWithRetries(ctx context.Context, txRef string) (*decimal.Decimal, string, string, string, error) {

	var (
		amount    *decimal.Decimal
		status    string
		currency  string
		custodian string
		err       error
	)

	// best effort to check that the tx is done processing
OUTER:
	for i := 0; i < 5; i++ {
		select {
		case <-ctx.Done():
			break OUTER
		case <-time.After(500 * time.Millisecond):
			amount, status, currency, custodian, err = getUpholdCustodialTx(ctx, txRef)
			if err != nil {
				return nil, "", "", "", fmt.Errorf("failed to get uphold tx by txRef %s: %w", txRef, err)
			}
			if status != "processing" && status != "pending" {
				break OUTER
			}
		}
	}

	return amount, status, currency, custodian, nil
}

// returns gemini client, api key, client id, settlement address, apiSecret, error
func getGeminiInfoFromCtx(ctx context.Context) (string, string, string, string, error) {
	// get gemini client from context
	apiKey, ok := ctx.Value(appctx.GeminiAPIKeyCTXKey).(string)
	if !ok {
		return "", "", "", "", fmt.Errorf("no gemini api key in ctx: %w", appctx.ErrNotInContext)
	}

	// get gemini client id from context
	clientID, ok := ctx.Value(appctx.GeminiBrowserClientIDCTXKey).(string)
	if !ok {
		return "", "", "", "", fmt.Errorf("no gemini browser client id in ctx: %w", appctx.ErrNotInContext)
	}

	// get gemini settlement address from context
	settlementAddress, ok := ctx.Value(appctx.GeminiSettlementAddressCTXKey).(string)
	if !ok {
		return "", "", "", "", fmt.Errorf("no gemini settlement address in ctx: %w", appctx.ErrNotInContext)
	}

	// get gemini api secret from context
	apiSecret, ok := ctx.Value(appctx.GeminiAPISecretCTXKey).(string)
	if !ok {
		return "", "", "", "", fmt.Errorf("no gemini api secret in ctx: %w", appctx.ErrNotInContext)
	}

	return apiKey, clientID, settlementAddress, apiSecret, nil
}

// getGeminiCustodialTx - the the custodial tx information from gemini
func (s *Service) getGeminiCustodialTx(ctx context.Context, txRef string) (*decimal.Decimal, string, string, string, error) {
	sublogger := logging.Logger(ctx, "payments").With().
		Str("func", "getGeminiCustodialTx").
		Logger()

	custodian := "gemini"

	// call client.CheckTxStatus
	ctx = context.WithValue(ctx, appctx.GeminiAPISecretCTXKey, s.geminiConf.Secret)
	resp, err := s.geminiClient.CheckTxStatus(ctx, s.geminiConf.APIKey, s.geminiConf.ClientID, txRef)
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to check tx status")
		return nil, "", "", custodian, fmt.Errorf("error getting tx status: %w", err)
	}

	// check if destination is the right address
	if *resp.Destination != s.geminiConf.SettlementAddress {
		sublogger.Error().Err(err).Msg("settlement address does not match tx destination")
		return nil, "", "", custodian, errors.New("error recording transaction: invalid settlement address")
	}

	var (
		amount   decimal.Decimal
		status   string
		currency string
	)
	// return back the amount
	if resp.Amount != nil {
		amount = *resp.Amount
	}
	if resp.Status != nil {
		// response values are Titled from Gemini
		status = strings.ToLower(*resp.Status)
	}
	if resp.Currency != nil {
		currency = *resp.Currency
	}

	return &amount, status, currency, custodian, nil
}

// CreateTransactionFromRequest queries the endpoints and creates a transaciton
func (s *Service) CreateTransactionFromRequest(ctx context.Context, req CreateTransactionRequest, orderID uuid.UUID, getCustodialTx getCustodialTxFn) (*Transaction, error) {

	sublogger := logging.Logger(ctx, "payments").With().
		Str("func", "CreateAnonCardTransaction").
		Logger()

	// get the information from the custodian
	amount, status, currency, kind, err := getCustodialTx(ctx, req.ExternalTransactionID.String())
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to get and validate custodian transaction")
		return nil, errorutils.Wrap(err, fmt.Sprintf("failed to get get and validate custodialtx: %s", err.Error()))
	}

	transaction, err := s.Datastore.CreateTransaction(orderID, req.ExternalTransactionID.String(), status, currency, kind, *amount)
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to create the transaction for the order")
		return nil, errorutils.Wrap(err, "error recording transaction")
	}

	isPaid, err := s.IsOrderPaid(transaction.OrderID)
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to validate the order is paid based on transactions")
		return nil, errorutils.Wrap(err, "error validating order is paid")
	}

	// If the transaction that was satisifies the order then let's update the status
	if isPaid {
		err = s.Datastore.UpdateOrder(transaction.OrderID, "paid")
		if err != nil {
			sublogger.Error().Err(err).Msg("failed to set the status to paid")
			return nil, errorutils.Wrap(err, "error updating order status")
		}
	}

	return transaction, err
}

// UpdateTransactionFromRequest queries the endpoints and creates a transaciton
func (s *Service) UpdateTransactionFromRequest(ctx context.Context, req CreateTransactionRequest, orderID uuid.UUID, getCustodialTx getCustodialTxFn) (*Transaction, error) {

	sublogger := logging.Logger(ctx, "payments").With().
		Str("func", "UpdateTransactionFromRequest").
		Logger()

	// get the information from the custodian
	amount, status, currency, kind, err := getCustodialTx(ctx, req.ExternalTransactionID.String())
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to get and validate custodian transaction")
		return nil, errorutils.Wrap(err, fmt.Sprintf("failed to get get and validate custodialtx: %s", err.Error()))
	}

	transaction, err := s.Datastore.UpdateTransaction(orderID, req.ExternalTransactionID.String(), status, currency, kind, *amount)
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to create the transaction for the order")
		return nil, errorutils.Wrap(err, "error recording transaction")
	}

	isPaid, err := s.IsOrderPaid(transaction.OrderID)
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to validate the order is paid based on transactions")
		return nil, errorutils.Wrap(err, "error validating order is paid")
	}

	// If the transaction that was satisifies the order then let's update the status
	if isPaid {
		err = s.Datastore.UpdateOrder(transaction.OrderID, "paid")
		if err != nil {
			sublogger.Error().Err(err).Msg("failed to set the status to paid")
			return nil, errorutils.Wrap(err, "error updating order status")
		}
	}

	return transaction, err
}

// CreateAnonCardTransaction takes a signed transaction and executes it on behalf of an anon card
func (s *Service) CreateAnonCardTransaction(ctx context.Context, walletID uuid.UUID, transaction string, orderID uuid.UUID) (*Transaction, error) {

	sublogger := logging.Logger(ctx, "payments").With().
		Str("func", "CreateAnonCardTransaction").
		Logger()

	txInfo, err := s.wallet.SubmitAnonCardTransaction(
		ctx,
		walletID,
		transaction,
		uphold.AnonCardSettlementAddress,
	)
	if err != nil {
		return nil, errorutils.Wrap(err, "error submitting anon card transaction")
	}

	txInfo, err = s.waitForUpholdTxStatus(ctx, walletID, txInfo.ID, "completed")
	if err != nil {
		return nil, errorutils.Wrap(err, "error waiting for completed status for transaction")
	}

	txn, err := s.Datastore.CreateTransaction(orderID, txInfo.ID, txInfo.Status, txInfo.DestCurrency, "anonymous-card", txInfo.DestAmount)
	if err != nil {
		return nil, errorutils.Wrap(err, "error recording anon card transaction")
	}

	err = s.UpdateOrderStatus(orderID)
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to update order status")
		return nil, errorutils.Wrap(err, "error updating order status")
	}

	return txn, err
}

func (s *Service) waitForUpholdTxStatus(ctx context.Context, walletID uuid.UUID, txnID, status string) (*walletutils.TransactionInfo, error) {
	info, err := s.wallet.GetWallet(ctx, walletID)
	if err != nil {
		return nil, err
	}

	providerWallet, err := provider.GetWallet(ctx, *info)
	if err != nil {
		return nil, err
	}

	upholdWallet, ok := providerWallet.(*uphold.Wallet)
	if !ok {
		return nil, errors.New("only uphold wallets are supported")
	}

	var txInfo = &walletutils.TransactionInfo{
		ID: txnID,
	}
	// check status until it matches
	for {
		select {
		case <-ctx.Done():
			return nil, errors.New("timeout waiting for correct status")
		default:
			txInfo, err = upholdWallet.GetTransaction(ctx, txInfo.ID)
			if err != nil {
				return nil, errorutils.Wrap(err, "error getting transaction")
			}
			if strings.ToLower(txInfo.Status) == status {
				return txInfo, nil
			}
			<-time.After(1 * time.Second)
		}
	}
}

// IsOrderPaid determines if the order has been paid
func (s *Service) IsOrderPaid(orderID uuid.UUID) (bool, error) {
	// Now that the transaction has been created let's check to see if that fulfilled the order.
	order, err := s.Datastore.GetOrder(orderID)
	if err != nil {
		return false, err
	}

	sum, err := s.Datastore.GetSumForTransactions(orderID)
	if err != nil {
		return false, err
	}

	return sum.GreaterThanOrEqual(order.TotalPrice), nil
}

func parseURLAddOrderIDParam(u string, orderID uuid.UUID) string {
	// add order id to the stripe success and cancel urls
	surl, err := url.Parse(u)
	if err == nil {
		surlv := surl.Query()
		surlv.Add("order_id", orderID.String())
		surl.RawQuery = surlv.Encode()
		return surl.String()
	}
	// there was a parse error, return whatever was given
	return u
}

const (
	singleUse     = "single-use"
	timeLimited   = "time-limited"
	timeLimitedV2 = "time-limited-v2"
)

var errInvalidCredentialType = errors.New("invalid credential type on order")

// GetCredentials - based on the order, get the associated credentials
func (s *Service) GetCredentials(ctx context.Context, orderID uuid.UUID) (interface{}, int, error) {
	var credentialType string
	// get the order from datastore
	order, err := s.Datastore.GetOrder(orderID)
	if err != nil {
		return nil, http.StatusNotFound, fmt.Errorf("failed to get order: %w", err)
	}
	// look through order, find out what all the order item's credential types are
	for i, v := range order.Items {
		if i > 0 {
			if v.CredentialType != credentialType {
				// all the order items on the order need the same credential type
				return nil, http.StatusConflict, fmt.Errorf("all items must have the same credential type")
			}
		} else {
			credentialType = v.CredentialType
		}
	}

	switch credentialType {
	case singleUse:
		return s.GetSingleUseCreds(ctx, order)
	case timeLimited:
		return s.GetTimeLimitedCreds(ctx, order)
	case timeLimitedV2:
		return s.GetTimeLimitedV2Creds(ctx, order)
	}
	return nil, http.StatusConflict, errInvalidCredentialType
}

// GetSingleUseCreds returns all the single use credentials for a given order.
// If the credentials have been submitted but not yet signed it returns a http.StatusAccepted and an empty body.
// If the credentials have been signed it will return a http.StatusOK and the order credentials.
func (s *Service) GetSingleUseCreds(ctx context.Context, order *Order) ([]OrderCreds, int, error) {
	if order == nil {
		return nil, http.StatusBadRequest, fmt.Errorf("failed to create credentials, bad order")
	}

	creds, err := s.Datastore.GetOrderCreds(order.ID, false)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("error getting credentials: %w", err)
	}

	if creds != nil {
		// TODO: Issues #1541 remove once all creds using RunOrderJob have need processed
		for i := 0; i < len(creds); i++ {
			if creds[i].SignedCreds == nil {
				return nil, http.StatusAccepted, nil
			}
		}
		// TODO: End
		return creds, http.StatusOK, nil
	}

	// check to see if messages are in outbox
	outboxMessages, err := s.Datastore.GetSigningOrderRequestOutbox(ctx, order.ID)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("error getting credentials: %w", err)
	}

	if len(outboxMessages) > 0 {
		return nil, http.StatusAccepted, nil
	}

	return nil, http.StatusNotFound, fmt.Errorf("credentials do not exist")
}

// GetTimeLimitedV2Creds returns all the single use credentials for a given order.
// If the credentials have been submitted but not yet signed it returns a http.StatusAccepted and an empty body.
// If the credentials have been signed it will return a http.StatusOK and the time limited v2 credentials.
func (s *Service) GetTimeLimitedV2Creds(ctx context.Context, order *Order) (*TimeLimitedV2Creds, int, error) {
	if order == nil {
		return nil, http.StatusBadRequest, fmt.Errorf("failed to create credentials, bad order")
	}

	// First check order creds have successfully been submitted for processing.
	outboxMessages, err := s.Datastore.GetSigningOrderRequestOutbox(ctx, order.ID)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("error getting outbox messages: %w", err)
	}

	if len(outboxMessages) == 0 {
		return nil, http.StatusNotFound, fmt.Errorf("credentials do not exist")
	}

	// To ensure we have completed signing all the creds for our order we need to check the total number of creds matches
	// the number of signing results we are expecting otherwise we are not finished signing and return http.StatusAccepted.
	creds, err := s.Datastore.GetTimeLimitedV2OrderCredsByOrder(order.ID)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("error getting credentials: %w", err)
	}

	total, err := calculateTotalExpectedSigningResults(outboxMessages)
	if err != nil {
		return nil, http.StatusInternalServerError,
			fmt.Errorf("error calculating total expected signing results: %w", err)
	}

	if creds != nil && len(creds.Credentials) == total {
		return creds, http.StatusOK, nil
	}

	return nil, http.StatusAccepted, nil
}

// GetActiveCredentialSigningKey get the current active signing key for this merchant
func (s *Service) GetActiveCredentialSigningKey(ctx context.Context, merchantID string) ([]byte, error) {
	// sorted by name, created_at, first result is most recent
	keys, err := s.Datastore.GetKeysByMerchant(merchantID, false)
	if err != nil {
		return nil, fmt.Errorf("error getting keys by merchant: %w", err)
	}
	if keys == nil || len(*keys) < 1 {
		return nil, fmt.Errorf("merchant keys is nil")
	}

	secret, err := (*keys)[0].GetSecretKey()
	if err != nil {
		return nil, fmt.Errorf("error getting key's secret value: %w", err)
	}
	if secret == nil {
		return nil, fmt.Errorf("invalid empty value for secret key")
	}

	return []byte(*secret), nil
}

// GetCredentialSigningKeys get the current list of credential signing keys for this merchant
func (s *Service) GetCredentialSigningKeys(ctx context.Context, merchantID string) ([][]byte, error) {
	var resp = [][]byte{}
	keys, err := s.Datastore.GetKeysByMerchant(merchantID, false)
	if err != nil {
		return nil, fmt.Errorf("error getting keys by merchant: %w", err)
	}
	if keys == nil {
		return nil, fmt.Errorf("merchant keys is nil")
	}
	for _, k := range *keys {
		s, err := k.GetSecretKey()
		if err != nil {
			return nil, fmt.Errorf("error getting key's secret value: %w", err)
		}
		if s == nil {
			return nil, fmt.Errorf("invalid empty value for secret key")
		}
		resp = append(resp, []byte(*s))
	}
	return resp, nil
}

// credChunkFn - given a time, calculate the next increment of time based on interval
func credChunkFn(interval timeutils.ISODuration) func(time.Time) (time.Time, time.Time) {
	return func(t time.Time) (time.Time, time.Time) {
		var (
			start time.Time
			end   time.Time
		)

		// get the future time one credential interval away
		c, err := interval.From(t)
		if err != nil {
			return start, end
		}
		// get the go duration to that future time one credential away
		td := (*c).Sub(t)

		// i.e. 1 day will truncate on the day
		// i.e. 1 month will truncate on the month
		switch interval.String() {
		case "P1M":
			y, m, _ := t.Date()
			// reset the date to be the first of the given month
			start = time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
			end = time.Date(y, m+1, 1, 0, 0, 0, 0, time.UTC)
		default:
			// use truncate
			start = t.Truncate(td)
			end = start.Add(td)
		}

		return start, end
	}
}

// timeChunking - given a duration and interval size of credential, return number of credentials
// to generate, and a function that takes a start time and increments it by an appropriate amount
func timeChunking(ctx context.Context, issuerID string, timeLimitedSecret cryptography.TimeLimitedSecret, orderID, itemID uuid.UUID, issued time.Time, duration, interval timeutils.ISODuration) ([]TimeLimitedCreds, error) {
	expiresAt, err := duration.From(issued)
	if err != nil {
		return nil, fmt.Errorf("unable to compute expiry")
	}
	// Add at least 5 days of grace period
	*expiresAt = (*expiresAt).AddDate(0, 0, 5)

	chunkingFn := credChunkFn(interval)

	// set dEnd to today chunked
	dEnd, _ := chunkingFn(time.Now())

	var credentials []TimeLimitedCreds
	var dStart time.Time
	for dEnd.Before(*expiresAt) {
		dStart, dEnd = chunkingFn(dEnd)
		timeBasedToken, err := timeLimitedSecret.Derive(
			[]byte(issuerID),
			dStart,
			dEnd)
		if err != nil {
			return credentials, fmt.Errorf("error generating credentials: %w", err)
		}
		credentials = append(credentials, TimeLimitedCreds{
			ID:        itemID,
			OrderID:   orderID,
			IssuedAt:  dStart.Format("2006-01-02"),
			ExpiresAt: dEnd.Format("2006-01-02"),
			Token:     timeBasedToken,
		})
	}
	return credentials, nil
}

// GetTimeLimitedCreds get an order's time limited creds
func (s *Service) GetTimeLimitedCreds(ctx context.Context, order *Order) ([]TimeLimitedCreds, int, error) {
	if order == nil {
		return nil, http.StatusBadRequest, fmt.Errorf("failed to create credentials, bad order")
	}

	// is the order paid?
	if !order.IsPaid() || order.LastPaidAt == nil {
		return nil, http.StatusBadRequest, fmt.Errorf("order is not paid, or invalid last paid at")
	}

	issuedAt := order.LastPaidAt

	// if the order has an expiry, use that
	if order.ExpiresAt != nil {
		// check if we are past expiration, if so issue nothing
		if time.Now().After(*order.ExpiresAt) {
			return nil, http.StatusBadRequest, fmt.Errorf("order has expired")
		}
	}

	var credentials []TimeLimitedCreds
	secret, err := s.GetActiveCredentialSigningKey(ctx, order.MerchantID)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to get merchant signing key: %w", err)
	}
	timeLimitedSecret := cryptography.NewTimeLimitedSecret(secret)

	for _, item := range order.Items {

		if item.ValidForISO == nil {
			return nil, http.StatusBadRequest, fmt.Errorf("order item has no valid for time")
		}
		duration, err := timeutils.ParseDuration(*(item.ValidForISO))
		if err != nil {
			return nil, http.StatusInternalServerError, fmt.Errorf("unable to parse order duration for credentials")
		}

		if item.IssuanceIntervalISO == nil {
			item.IssuanceIntervalISO = new(string)
			*(item.IssuanceIntervalISO) = "P1D"
		}
		interval, err := timeutils.ParseDuration(*(item.IssuanceIntervalISO))
		if err != nil {
			return nil, http.StatusInternalServerError, fmt.Errorf("unable to parse issuance interval for credentials")
		}

		issuerID, err := encodeIssuerID(order.MerchantID, item.SKU)
		if err != nil {
			return nil, http.StatusInternalServerError, fmt.Errorf("error encoding issuer: %w", err)
		}

		creds, err := timeChunking(ctx, issuerID, timeLimitedSecret, order.ID, item.ID, *issuedAt, *duration, *interval)
		if err != nil {
			return nil, http.StatusInternalServerError, fmt.Errorf("failed to derive credential chunking: %w", err)
		}
		credentials = append(credentials, creds...)
	}

	if len(credentials) > 0 {
		return credentials, http.StatusOK, nil
	}
	return nil, http.StatusBadRequest, fmt.Errorf("failed to issue credentials")
}

type credential interface {
	GetSku(context.Context) string
	GetType(context.Context) string
	GetMerchantID(context.Context) string
	GetPresentation(context.Context) string
}

// TODO refactor this see issue #1502
// verifyCredential - given a credential, verify it.
func (s *Service) verifyCredential(ctx context.Context, req credential, w http.ResponseWriter) *handlers.AppError {
	logger := logging.Logger(ctx, "verifyCredential")

	merchant, err := GetMerchant(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("failed to get the merchant from the context")
		return handlers.WrapError(err, "Error getting auth merchant", http.StatusInternalServerError)
	}

	logger.Debug().Str("merchant", merchant).Msg("got merchant from the context")

	caveats := GetCaveats(ctx)

	if req.GetMerchantID(ctx) != merchant {
		logger.Warn().
			Str("req.MerchantID", req.GetMerchantID(ctx)).
			Str("merchant", merchant).
			Msg("merchant does not match the key's merchant")
		return handlers.WrapError(nil, "Verify request merchant does not match authentication", http.StatusForbidden)
	}

	logger.Debug().Str("merchant", merchant).Msg("merchant matches the key's merchant")

	if caveats != nil {
		if sku, ok := caveats["sku"]; ok {
			if req.GetSku(ctx) != sku {
				logger.Warn().
					Str("req.SKU", req.GetSku(ctx)).
					Str("sku", sku).
					Msg("sku caveat does not match")
				return handlers.WrapError(nil, "Verify request sku does not match authentication", http.StatusForbidden)
			}
		}
	}
	logger.Debug().Msg("caveats validated")

	if req.GetType(ctx) == singleUse || req.GetType(ctx) == timeLimitedV2 {
		var bytes []byte
		bytes, err = base64.StdEncoding.DecodeString(req.GetPresentation(ctx))
		if err != nil {
			return handlers.WrapError(err, "Error in decoding presentation", http.StatusBadRequest)
		}

		var decodedCredential cbr.CredentialRedemption
		err = json.Unmarshal(bytes, &decodedCredential)
		if err != nil {
			return handlers.WrapError(err, "Error in presentation formatting", http.StatusBadRequest)
		}

		// Ensure that the credential being redeemed (opaque to merchant) matches the outer credential details
		issuerID, err := encodeIssuerID(req.GetMerchantID(ctx), req.GetSku(ctx))
		if err != nil {
			return handlers.WrapError(err, "Error in outer merchantId or sku", http.StatusBadRequest)
		}
		if issuerID != decodedCredential.Issuer {
			return handlers.WrapError(nil, "Error, outer merchant and sku don't match issuer", http.StatusBadRequest)
		}

		switch req.GetType(ctx) {
		case singleUse:
			err = s.cbClient.RedeemCredential(ctx, decodedCredential.Issuer, decodedCredential.TokenPreimage,
				decodedCredential.Signature, decodedCredential.Issuer)
		case timeLimitedV2:
			err = s.cbClient.RedeemCredentialV3(ctx, decodedCredential.Issuer, decodedCredential.TokenPreimage,
				decodedCredential.Signature, decodedCredential.Issuer)
		default:
			return handlers.WrapError(fmt.Errorf("credential type %s not suppoted", req.GetType(ctx)),
				"unknown credential type %s", http.StatusBadRequest)
		}

		if err != nil {
			// if this is a duplicate redemption these are not verified
			if err.Error() == cbr.ErrDupRedeem.Error() || err.Error() == cbr.ErrBadRequest.Error() {
				return handlers.WrapError(err, "invalid credentials", http.StatusForbidden)
			}
			return handlers.WrapError(err, "Error verifying credentials", http.StatusInternalServerError)
		}

		return handlers.RenderContent(ctx, "Credentials successfully verified", w, http.StatusOK)
	}

	if req.GetType(ctx) == "time-limited" {
		// Presentation includes a token and token metadata test test
		type Presentation struct {
			IssuedAt  string `json:"issuedAt"`
			ExpiresAt string `json:"expiresAt"`
			Token     string `json:"token"`
		}

		var bytes []byte
		bytes, err = base64.StdEncoding.DecodeString(req.GetPresentation(ctx))
		if err != nil {
			logger.Error().Err(err).
				Msg("failed to decode the request token presentation")
			return handlers.WrapError(err, "Error in decoding presentation", http.StatusBadRequest)
		}
		logger.Debug().Str("presentation", string(bytes)).Msg("presentation decoded")

		var presentation Presentation
		err = json.Unmarshal(bytes, &presentation)
		if err != nil {
			logger.Error().Err(err).
				Msg("failed to unmarshal the request token presentation")
			return handlers.WrapError(err, "Error in presentation formatting", http.StatusBadRequest)
		}

		logger.Debug().Str("presentation", string(bytes)).Msg("presentation unmarshalled")

		// Ensure that the credential being redeemed (opaque to merchant) matches the outer credential details
		issuerID, err := encodeIssuerID(req.GetMerchantID(ctx), req.GetSku(ctx))
		if err != nil {
			logger.Error().Err(err).
				Msg("failed to encode the issuer id")
			return handlers.WrapError(err, "Error in outer merchantId or sku", http.StatusBadRequest)
		}
		logger.Debug().Str("issuer", issuerID).Msg("issuer encoded")

		keys, err := s.GetCredentialSigningKeys(ctx, req.GetMerchantID(ctx))
		if err != nil {
			return handlers.WrapError(err, "failed to get merchant signing key", http.StatusInternalServerError)
		}

		issuedAt, err := time.Parse("2006-01-02", presentation.IssuedAt)
		if err != nil {
			logger.Error().Err(err).
				Msg("failed to parse issued at time of credential")
			return handlers.WrapError(err, "Error parsing issuedAt", http.StatusBadRequest)
		}
		expiresAt, err := time.Parse("2006-01-02", presentation.ExpiresAt)
		if err != nil {
			logger.Error().Err(err).
				Msg("failed to parse expires at time of credential")
			return handlers.WrapError(err, "Error parsing expiresAt", http.StatusBadRequest)
		}

		for _, key := range keys {
			timeLimitedSecret := cryptography.NewTimeLimitedSecret(key)
			verified, err := timeLimitedSecret.Verify([]byte(issuerID), issuedAt, expiresAt, presentation.Token)
			if err != nil {
				logger.Error().Err(err).
					Msg("failed to verify time limited credential")
				return handlers.WrapError(err, "Error in token verification", http.StatusBadRequest)
			}

			if verified {
				// check against expiration time, issued time
				if time.Now().After(expiresAt) || time.Now().Before(issuedAt) {
					logger.Error().
						Msg("credentials are not valid")
					return handlers.RenderContent(ctx, "Credentials are not valid", w, http.StatusForbidden)
				}
				logger.Debug().Msg("credentials verified")
				return handlers.RenderContent(ctx, "Credentials successfully verified", w, http.StatusOK)
			}
		}
		logger.Error().
			Msg("credentials could not be verified")
		return handlers.RenderContent(ctx, "Credentials could not be verified", w, http.StatusForbidden)
	}
	return handlers.WrapError(nil, "Unknown credential type", http.StatusBadRequest)
}

// RunNextOrderJob Deprecated. Takes the next order job and completes it.
func (s *Service) RunNextOrderJob(ctx context.Context) (bool, error) {
	for {
		attempted, err := s.Datastore.RunNextOrderJob(ctx, s)
		if err != nil {
			return attempted, fmt.Errorf("failed to attempt run next order job: %w", err)
		}
		if !attempted {
			return attempted, err
		}
	}
}

// RunSendSigningRequestJob - send the order credentials signing requests
func (s *Service) RunSendSigningRequestJob(ctx context.Context) (bool, error) {
	return true, s.Datastore.SendSigningRequest(ctx, s)
}

// RunStoreSignedOrderCredentialsJob - store the signed order credentials
func (s *Service) RunStoreSignedOrderCredentialsJob(ctx context.Context) (bool, error) {
	for {
		select {
		case <-ctx.Done():
			return true, ctx.Err()
		default:
			err := s.Datastore.StoreSignedOrderCredentials(ctx, s)
			if err != nil {
				return true, fmt.Errorf("error storing signed order credentails: %w", err)
			}
		}
	}
}

// verifyDeveloperNotification - verify the developer notification from playstore
func (s *Service) verifyDeveloperNotification(ctx context.Context, dn *DeveloperNotification) error {
	if dn == nil || dn.SubscriptionNotification.PurchaseToken == "" {
		return errors.New("notification has no purchase token")
	}

	// lookup the order based on the token as externalID
	o, err := s.Datastore.GetOrderByExternalID(dn.SubscriptionNotification.PurchaseToken)
	if err != nil {
		return fmt.Errorf("failed to get order from db: %w", err)
	}

	if o == nil {
		return fmt.Errorf("failed to get order from db: %w", errNotFound)
	}

	// have order, now validate the receipt from the notification
	_, err = s.validateReceipt(ctx, &o.ID, SubmitReceiptRequestV1{
		Type:           "android",
		Blob:           dn.SubscriptionNotification.PurchaseToken,
		Package:        dn.PackageName,
		SubscriptionID: dn.SubscriptionNotification.SubscriptionID,
	})
	if err != nil {
		return fmt.Errorf("failed to validate purchase token: %w", err)
	}

	switch dn.SubscriptionNotification.NotificationType {
	case androidSubscriptionRenewed,
		androidSubscriptionRecovered,
		androidSubscriptionPurchased,
		androidSubscriptionRestarted,
		androidSubscriptionInGracePeriod,
		androidSubscriptionPriceChangeConfirmed:
		if err = s.Datastore.RenewOrder(ctx, o.ID); err != nil {
			return fmt.Errorf("failed to renew subscription in skus: %w", err)
		}
	case androidSubscriptionExpired,
		androidSubscriptionRevoked,
		androidSubscriptionPausedScheduleChanged,
		androidSubscriptionPaused,
		androidSubscriptionDeferred,
		androidSubscriptionOnHold,
		androidSubscriptionCanceled,
		androidSubscriptionUnknown:
		if err = s.CancelOrder(o.ID); err != nil {
			return fmt.Errorf("failed to cancel subscription in skus: %w", err)
		}
	default:
		return errors.New("failed to act on subscription notification")
	}

	return nil
}

// validateReceipt - perform receipt validation
func (s *Service) validateReceipt(ctx context.Context, orderID *uuid.UUID, receipt interface{}) (string, error) {
	// based on the vendor call the vendor specific apis to check the status of the receipt,
	if v, ok := receipt.(SubmitReceiptRequestV1); ok {
		// and get back the external id
		if fn, ok := receiptValidationFns[v.Type]; ok {
			return fn(ctx, receipt)
		}
	}

	return "", errorutils.ErrNotImplemented
}

// UpdateOrderStatusPaidWithMetadata - update the order status with metadata
func (s *Service) UpdateOrderStatusPaidWithMetadata(ctx context.Context, orderID *uuid.UUID, metadata datastore.Metadata) error {
	// create a tx for use in all datastore calls
	ctx, _, rollback, commit, err := datastore.GetTx(ctx, s.Datastore)
	defer rollback() // doesnt hurt to rollback incase we panic

	if err != nil {
		return fmt.Errorf("failed to get db transaction: %w", err)
	}

	for k, v := range metadata {
		if err := s.Datastore.AppendOrderMetadata(ctx, orderID, k, v); err != nil {
			return fmt.Errorf("failed to append order metadata: %w", err)
		}
	}
	if err := s.Datastore.SetOrderPaid(ctx, orderID); err != nil {
		return fmt.Errorf("failed to set order paid: %w", err)
	}

	return commit()
}

// calculateTotalExpectedSigningResults calculates the expected number of signing results by multiplying the number
// of blinded creds by the number of order items in the order. This function is only relevant to
// skus.TimeLimitedV2Creds credentials so we can compare how many results we have received and if we are done
// signing the order.
func calculateTotalExpectedSigningResults(outboxMessages []SigningOrderRequestOutbox) (int, error) {
	total := 0

	var sor SigningOrderRequest
	for _, outboxMessage := range outboxMessages {
		err := json.Unmarshal(outboxMessage.Message, &sor)
		if err != nil {
			return 0, fmt.Errorf("error unmarshaling outbox message: %w", err)
		}
		for _, data := range sor.Data {
			total += len(data.BlindedTokens)
		}
	}

	return total, nil
}
