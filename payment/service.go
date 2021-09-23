package payment

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	session "github.com/stripe/stripe-go/v71/checkout/session"
	client "github.com/stripe/stripe-go/v71/client"
	sub "github.com/stripe/stripe-go/v71/sub"

	"errors"

	"github.com/brave-intl/bat-go/utils/cryptography"
	"github.com/brave-intl/bat-go/utils/logging"
	srv "github.com/brave-intl/bat-go/utils/service"
	timeutils "github.com/brave-intl/bat-go/utils/time"
	"github.com/brave-intl/bat-go/utils/wallet/provider"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/getsentry/sentry-go"
	"github.com/linkedin/goavro"
	stripe "github.com/stripe/stripe-go/v71"

	"github.com/brave-intl/bat-go/utils/clients/cbr"
	"github.com/brave-intl/bat-go/utils/clients/gemini"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	kafkautils "github.com/brave-intl/bat-go/utils/kafka"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	uuid "github.com/satori/go.uuid"
	kafka "github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
)

var (
	voteTopic = os.Getenv("ENV") + ".payment.vote"
)

const (
	// OrderStatusCanceled - string literal used in db for canceled status
	OrderStatusCanceled = "canceled"
	// OrderStatusPaid - string literal used in db for canceled status
	OrderStatusPaid = "paid"
	// OrderStatusPending - string literal used in db for pending status
	OrderStatusPending = "pending"
)

// Service contains datastore
type Service struct {
	wallet           *wallet.Service
	cbClient         cbr.Client
	scClient         *client.API
	Datastore        Datastore
	codecs           map[string]*goavro.Codec
	kafkaWriter      *kafka.Writer
	kafkaDialer      *kafka.Dialer
	jobs             []srv.Job
	pauseVoteUntil   time.Time
	pauseVoteUntilMu sync.RWMutex
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

	var err error
	s.kafkaWriter, s.kafkaDialer, err = kafkautils.InitKafkaWriter(ctx, voteTopic)
	if err != nil {
		return fmt.Errorf("failed to initialize kafka: %w", err)
	}

	s.codecs, err = kafkautils.GenerateCodecs(map[string]string{
		"vote": voteSchema,
	})

	if err != nil {
		return fmt.Errorf("failed to generate codecs kafka: %w", err)
	}
	return nil
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(ctx context.Context, datastore Datastore, walletService *wallet.Service) (service *Service, err error) {
	sublogger := logging.Logger(ctx, "payments").With().Str("func", "InitService").Logger()

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

	service = &Service{
		wallet:           walletService,
		cbClient:         cbClient,
		scClient:         scClient,
		Datastore:        datastore,
		pauseVoteUntilMu: sync.RWMutex{},
	}

	// setup runnable jobs
	service.jobs = []srv.Job{
		{
			Func:    service.RunNextVoteDrainJob,
			Cadence: 5 * time.Second,
			Workers: 1,
		},
		{
			Func:    service.RunNextOrderJob,
			Cadence: 1 * time.Second,
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
	orderItems := []OrderItem{}
	var (
		currency              string
		location              string
		validFor              *time.Duration
		stripeSuccessURI      string
		stripeCancelURI       string
		status                string
		allowedPaymentMethods = new(Methods)
	)

	for i := 0; i < len(req.Items); i++ {
		orderItem, pm, err := s.CreateOrderItemFromMacaroon(ctx, req.Items[i].SKU, req.Items[i].Quantity)
		if err != nil {
			return nil, err
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

	order, err := s.Datastore.CreateOrder(totalPrice, "brave.com", status, currency, location, validFor, orderItems, allowedPaymentMethods)

	if err != nil {
		return nil, fmt.Errorf("failed to create order: %w", err)
	}

	var freeTrialDays int64
	// TODO: make this part of the sku
	if location == "talk.brave.com" {
		freeTrialDays = 30
	}

	if !order.IsPaid() && order.IsStripePayable() {
		checkoutSession, err := order.CreateStripeCheckoutSession(
			req.Email,
			parseURLAddOrderIDParam(stripeSuccessURI, order.ID),
			parseURLAddOrderIDParam(stripeCancelURI, order.ID),
			freeTrialDays,
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
		return nil, err
	}

	// check if this order has an expired checkout session
	expired, cs, err := s.Datastore.CheckExpiredCheckoutSession(orderID)
	if expired {
		// if expired update with new checkout session
		if !order.IsPaid() && order.IsStripePayable() {

			// get old checkout session from stripe by id
			stripeSession, err := session.Get(cs, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get stripe checkout session: %w", err)
			}

			var freeTrialDays int64
			// TODO: make this part of the sku
			if order.Location.String == "talk.brave.com" {
				freeTrialDays = 30
			}

			checkoutSession, err := order.CreateStripeCheckoutSession(
				stripeSession.CustomerEmail,
				stripeSession.SuccessURL, stripeSession.CancelURL,
				freeTrialDays,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to create checkout session: %w", err)
			}

			err = s.Datastore.UpdateOrderMetadata(order.ID, "stripeCheckoutSessionId", checkoutSession.SessionID)
			if err != nil {
				return nil, fmt.Errorf("failed to update order metadata: %w", err)
			}
		}

		// get the order
		order, err = s.Datastore.GetOrder(orderID)
		if err != nil {
			return nil, err
		}
	}
	return order, err
}

// CancelOrder - cancels an order, propogates to stripe if needed
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

// getUpholdCustodialTx - the the custodial tx information from uphold
func getUpholdCustodialTx(ctx context.Context, txRef string) (*decimal.Decimal, string, string, string, error) {
	var wallet uphold.Wallet
	upholdTransaction, err := wallet.GetTransaction(txRef)

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

// returns gemini client, api key, client id, settlement address, error
func getGeminiInfoFromCtx(ctx context.Context) (gemini.Client, string, string, string, error) {
	// get gemini client from context
	geminiClient, ok := ctx.Value(appctx.GeminiClientCTXKey).(gemini.Client)
	if !ok {
		return nil, "", "", "", fmt.Errorf("no gemini client in ctx: %w", appctx.ErrNotInContext)
	}
	// get gemini client from context
	apiKey, ok := ctx.Value(appctx.GeminiAPIKeyCTXKey).(string)
	if !ok {
		return nil, "", "", "", fmt.Errorf("no gemini api key in ctx: %w", appctx.ErrNotInContext)
	}

	// get gemini client id from context
	clientID, ok := ctx.Value(appctx.GeminiBrowserClientIDCTXKey).(string)
	if !ok {
		return nil, "", "", "", fmt.Errorf("no gemini browser client id in ctx: %w", appctx.ErrNotInContext)
	}

	// get gemini settlement address from context
	settlementAddress, ok := ctx.Value(appctx.GeminiSettlementAddressCTXKey).(string)
	if !ok {
		return nil, "", "", "", fmt.Errorf("no gemini settlement address in ctx: %w", appctx.ErrNotInContext)
	}

	return geminiClient, apiKey, clientID, settlementAddress, nil
}

// getGeminiCustodialTx - the the custodial tx information from gemini
func getGeminiCustodialTx(ctx context.Context, txRef string) (*decimal.Decimal, string, string, string, error) {
	sublogger := logging.Logger(ctx, "payments").With().
		Str("func", "getGeminiCustodialTx").
		Logger()

	custodian := "gemini"
	// get gemini client from tx
	client, geminiAPIKey, geminiBrowserClientID, settlementAddress, err := getGeminiInfoFromCtx(ctx)
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to get gemini configuration")
		return nil, "", "", custodian, fmt.Errorf("error getting gemini client/info from ctx: %w", err)
	}

	// call client.CheckTxStatus
	resp, err := client.CheckTxStatus(ctx, geminiAPIKey, geminiBrowserClientID, txRef)
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to check tx status")
		return nil, "", "", custodian, fmt.Errorf("error getting tx status: %w", err)
	}

	// check if destination is the right address
	if *resp.Destination != settlementAddress {
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
			txInfo, err = upholdWallet.GetTransaction(txInfo.ID)
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

// RunNextOrderJob takes the next order job and completes it
func (s *Service) RunNextOrderJob(ctx context.Context) (bool, error) {
	for {
		attempted, err := s.Datastore.RunNextOrderJob(ctx, s)
		if err != nil {
			sentry.CaptureMessage(err.Error())
			sentry.Flush(time.Second * 2)
			return attempted, fmt.Errorf("failed to attempt run next order job: %w", err)
		}
		if !attempted {
			return attempted, err
		}
	}
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
	singleUse   = "single-use"
	timeLimited = "time-limited"
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
	}
	return nil, http.StatusConflict, errInvalidCredentialType
}

// GetSingleUseCreds get an order's single use creds
func (s *Service) GetSingleUseCreds(ctx context.Context, order *Order) ([]OrderCreds, int, error) {

	if order == nil {
		return nil, http.StatusBadRequest, fmt.Errorf("failed to create credentials, bad order")
	}

	creds, err := s.Datastore.GetOrderCreds(order.ID, false)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("error getting credentials: %w", err)
	}

	if creds == nil {
		return nil, http.StatusNotFound, fmt.Errorf("Credentials do not exist")
	}

	status := http.StatusOK
	for i := 0; i < len(*creds); i++ {
		if (*creds)[i].SignedCreds == nil {
			status = http.StatusAccepted
			break
		}
	}
	return *creds, status, nil
}

const oneDay = 24 * time.Hour

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
	timeLimitedSecret := cryptography.NewTimeLimitedSecret([]byte(os.Getenv("BRAVE_MERCHANT_KEY")))

	for _, item := range order.Items {
		numCreds := 0

		if item.ValidForISO != nil {
			// Use the sku's duration on the item
			isoD, err := timeutils.ParseDuration(*item.ValidForISO)
			if err != nil {
				return nil, http.StatusInternalServerError, fmt.Errorf("error decoding valid duration: %w", err)
			}
			expiry, err := isoD.From(*issuedAt)
			if err != nil {
				return nil, http.StatusInternalServerError, fmt.Errorf("calculating expiry: %w", err)
			}

			// check if we are past valid for, if so issue nothing and return
			if time.Now().After(*expiry) {
				return nil, http.StatusBadRequest, fmt.Errorf("order item has expired")
			}

			validFor := time.Until(*expiry)
			// number of day passes +5 to account for stripe lag on subscription webhook renewal
			numCreds = int((validFor).Hours()/24) + 5
		}

		issuerID, err := encodeIssuerID(order.MerchantID, item.SKU)
		if err != nil {
			return nil, http.StatusInternalServerError, fmt.Errorf("error encoding issuer: %w", err)
		}

		now := time.Now()
		dStart := now.Truncate(oneDay)
		dEnd := now.Add(oneDay).Truncate(oneDay)

		// for the number of days order is valid for, create per day creds

		for i := 0; i < numCreds; i++ {
			// iterate through order items, derive the time limited creds
			timeBasedToken, err := timeLimitedSecret.Derive(
				[]byte(issuerID),
				dStart,
				dEnd)
			if err != nil {
				return nil, http.StatusInternalServerError, fmt.Errorf("error generating credentials: %w", err)
			}
			credentials = append(credentials, TimeLimitedCreds{
				ID:        item.ID,
				OrderID:   order.ID,
				IssuedAt:  dStart.Format("2006-01-02"),
				ExpiresAt: dEnd.Format("2006-01-02"),
				Token:     timeBasedToken,
			})

			// increment dStart and dEnd
			dStart = dStart.Add(oneDay)
			dEnd = dEnd.Add(oneDay)
		}
	}

	if len(credentials) > 0 {
		return credentials, http.StatusOK, nil
	}
	return nil, http.StatusBadRequest, fmt.Errorf("failed to issue credentials")

}
