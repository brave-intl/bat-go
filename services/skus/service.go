package skus

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/linkedin/goavro"
	uuid "github.com/satori/go.uuid"
	"github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/client"
	"github.com/stripe/stripe-go/v72/sub"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"

	"github.com/brave-intl/bat-go/libs/backoff"
	"github.com/brave-intl/bat-go/libs/clients/cbr"
	"github.com/brave-intl/bat-go/libs/clients/gemini"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/brave-intl/bat-go/libs/datastore"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/handlers"
	kafkautils "github.com/brave-intl/bat-go/libs/kafka"
	"github.com/brave-intl/bat-go/libs/logging"
	srv "github.com/brave-intl/bat-go/libs/service"
	timeutils "github.com/brave-intl/bat-go/libs/time"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/libs/wallet/provider"
	"github.com/brave-intl/bat-go/libs/wallet/provider/uphold"
	"github.com/brave-intl/bat-go/services/wallet"

	"github.com/brave-intl/bat-go/services/skus/model"
	"github.com/brave-intl/bat-go/services/skus/radom"
	"github.com/brave-intl/bat-go/services/skus/xstripe"
)

var (
	voteTopic = os.Getenv("ENV") + ".payment.vote"

	// TODO address in kafka refactor. Check topics are correct
	// kafka topic for requesting order credentials are signed, write to by sku service
	kafkaUnsignedOrderCredsTopic = os.Getenv("GRANT_CBP_SIGN_PRODUCER_TOPIC")

	// kafka topic which receives order creds once they have been signed, read by sku service
	kafkaSignedOrderCredsTopic      = os.Getenv("GRANT_CBP_SIGN_CONSUMER_TOPIC")
	kafkaSignedOrderCredsDLQTopic   = os.Getenv("GRANT_CBP_SIGN_CONSUMER_TOPIC_DLQ")
	kafkaSignedRequestReaderGroupID = os.Getenv("KAFKA_CONSUMER_GROUP_SIGNED_ORDER_CREDENTIALS")
)

const (
	// TODO(pavelb): Gradually replace it everywhere.
	//
	// OrderStatusCanceled - string literal used in db for canceled status
	OrderStatusCanceled = model.OrderStatusCanceled
	// OrderStatusPaid - string literal used in db for canceled status
	OrderStatusPaid = model.OrderStatusPaid
	// OrderStatusPending - string literal used in db for pending status
	OrderStatusPending = model.OrderStatusPending
)

const (
	// Default issuer V3 config default values
	defaultBuffer  = 30
	defaultOverlap = 5

	singleUse     = "single-use"
	timeLimited   = "time-limited"
	timeLimitedV2 = "time-limited-v2"

	errSetRetryAfter             = model.Error("set retry-after")
	errClosingResource           = model.Error("error closing resource")
	errGeminiClientNotConfigured = model.Error("service: gemini client not configured")
	errLegacyOutboxNotFound      = model.Error("error no order credentials have been submitted for signing")
	errWrongOrderIDForRequestID  = model.Error("signed request order id does not belong to request id")
	errLegacySUCredsNotFound     = model.Error("credentials do not exist")
)

type orderStoreSvc interface {
	Create(ctx context.Context, dbi sqlx.QueryerContext, oreq *model.OrderNew) (*model.Order, error)
	Get(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error)
	GetByExternalID(ctx context.Context, dbi sqlx.QueryerContext, extID string) (*model.Order, error)
	SetStatus(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, status string) error
	SetExpiresAt(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error
	SetLastPaidAt(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, when time.Time) error
	AppendMetadata(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key, val string) error
	AppendMetadataInt(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key string, val int) error
	AppendMetadataInt64(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, key string, val int64) error
	GetExpiredStripeCheckoutSessionID(ctx context.Context, dbi sqlx.QueryerContext, orderID uuid.UUID) (string, error)
}

type tlv2Store interface {
	GetCredSubmissionReport(ctx context.Context, dbi sqlx.QueryerContext, orderID, itemID, reqID uuid.UUID, firstBCred string) (model.TLV2CredSubmissionReport, error)
	UniqBatches(ctx context.Context, dbi sqlx.QueryerContext, orderID, itemID uuid.UUID, from, to time.Time) (int, error)
	DeleteLegacy(ctx context.Context, dbi sqlx.ExecerContext, orderID uuid.UUID) error
}

type vendorReceiptValidator interface {
	validateApple(ctx context.Context, req model.ReceiptRequest) (model.ReceiptData, error)
	validateGoogle(ctx context.Context, req model.ReceiptRequest) (model.ReceiptData, error)
	fetchSubPlayStore(ctx context.Context, pkgName, subID, token string) (*playStoreSubPurchase, error)
}

type gpsMessageAuthenticator interface {
	authenticate(ctx context.Context, token string) error
}

type stripeClient interface {
	Session(ctx context.Context, id string, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error)
	CreateSession(ctx context.Context, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error)
	Subscription(ctx context.Context, id string, params *stripe.SubscriptionParams) (*stripe.Subscription, error)
	FindCustomer(ctx context.Context, email string) (*stripe.Customer, bool)
}

// Service contains datastore
type radomClient interface {
	CreateCheckoutSession(ctx context.Context, creq *radom.CheckoutSessionRequest) (radom.CheckoutSessionResponse, error)
	GetSubscription(ctx context.Context, subID string) (*radom.SubscriptionResponse, error)
}

type radomMessageAuthenticator interface {
	Authenticate(ctx context.Context, token string) error
}

type Service struct {
	orderRepo     orderStoreSvc
	orderItemRepo orderItemStore
	issuerRepo    issuerStore
	payHistRepo   orderPayHistoryStore
	tlv2Repo      tlv2Store

	// TODO: Eventually remove it.
	Datastore Datastore

	wallet           *wallet.Service
	cbClient         cbr.Client
	geminiClient     gemini.Client
	geminiConf       *gemini.Conf
	stripeCl         stripeClient
	codecs           map[string]*goavro.Codec
	kafkaWriter      *kafka.Writer
	kafkaDialer      *kafka.Dialer
	jobs             []srv.Job
	pauseVoteUntil   time.Time
	pauseVoteUntilMu sync.RWMutex
	retry            backoff.RetryFunc

	radomClient  radomClient
	radomGateway *radom.Gateway
	radomAuth    radomMessageAuthenticator

	vendorReceiptValid vendorReceiptValidator
	gpsAuth            gpsMessageAuthenticator
	assnCertVrf        *assnCertVerifier

	payProcCfg    *premiumPaymentProcConfig
	newItemReqSet map[string]model.OrderItemRequestNew
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
	// TODO address in kafka refactor
	// passing an empty string will not set topic on writer, so it can be defined at message write time
	var err error
	s.kafkaWriter, s.kafkaDialer, err = kafkautils.InitKafkaWriter(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to initialize kafka: %w", err)
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

// InitService creates a Service using the passed datastore and clients configured from the environment.
func InitService(
	ctx context.Context,
	datastore Datastore,
	walletService *wallet.Service,
	orderRepo orderStoreSvc,
	orderItemRepo orderItemStore,
	issuerRepo issuerStore,
	payHistRepo orderPayHistoryStore,
	tlv2repo tlv2Store,
) (*Service, error) {
	lg := logging.Logger(ctx, "payments").With().Str("func", "InitService").Logger()

	scClient := &client.API{}
	if enabled, ok := ctx.Value(appctx.StripeEnabledCTXKey).(bool); ok && enabled {
		stripe.EnableTelemetry = false

		var err error
		stripe.Key, err = appctx.GetStringFromContext(ctx, appctx.StripeSecretCTXKey)
		if err != nil {
			lg.Panic().Err(err).Msg("failed to get Stripe secret from context, and Stripe enabled")
		}

		scClient.Init(stripe.Key, nil)
	}

	env, err := appctx.GetStringFromContext(ctx, appctx.EnvironmentCTXKey)
	if err != nil {
		return nil, err
	}

	var radomCl *radom.Client
	var radomGateway *radom.Gateway
	var radomAuthCfg radom.MessageAuthConfig

	if enabled, _ := strconv.ParseBool(os.Getenv("RADOM_ENABLED")); enabled {
		srvURL := os.Getenv("RADOM_SERVER")
		if srvURL == "" {
			return nil, model.Error("skus: invalid radom url")
		}

		authToken := os.Getenv("RADOM_SECRET")
		if authToken == "" {
			return nil, model.Error("skus: radom secret not found")
		}

		{
			var err error

			radomCl, err = radom.New(srvURL, authToken)
			if err != nil {
				return nil, err
			}

			radomGateway, err = newRadomGateway(env)
			if err != nil {
				return nil, err
			}
		}

		radKey := os.Getenv("RADOM_VERIFICATION_KEY")
		if radKey == "" {
			return nil, model.Error("skus: radom verification key not found")
		}

		radomAuthCfg = radom.MessageAuthConfig{
			Enabled: enabled,
			Token:   []byte(radKey),
		}
	}

	radomAuth := radom.NewMessageAuthenticator(radomAuthCfg)

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

	cl := &http.Client{Timeout: 30 * time.Second}

	asKey, _ := ctx.Value(appctx.AppleReceiptSharedKeyCTXKey).(string)
	playKey, _ := ctx.Value(appctx.PlaystoreJSONKeyCTXKey).([]byte)
	rcptValidator, err := newReceiptVerifier(cl, asKey, playKey)
	if err != nil {
		return nil, err
	}

	assnCertVrf, err := newASSNCertVerifier()
	if err != nil {
		return nil, err
	}

	idv, err := idtoken.NewValidator(ctx, option.WithTelemetryDisabled())
	if err != nil {
		return nil, err
	}

	disabled, _ := strconv.ParseBool(os.Getenv("GCP_PUSH_NOTIFICATION"))
	if disabled {
		lg.Warn().Msg("gcp push notification is disabled")
	}

	aud := os.Getenv("GCP_PUSH_SUBSCRIPTION_AUDIENCE")
	if aud == "" {
		lg.Warn().Msg("gcp push subscription audience is empty")
	}

	iss := os.Getenv("GCP_CERT_ISSUER")
	if iss == "" {
		lg.Warn().Msg("gcp cert issuer is empty")
	}

	sa := os.Getenv("GCP_PUSH_SUBSCRIPTION_SERVICE_ACCOUNT")
	if sa == "" {
		lg.Warn().Msg("gcp push subscription service account is empty")
	}

	gpsCfg := gpsValidatorConfig{
		aud:      aud,
		iss:      iss,
		svcAcct:  sa,
		disabled: disabled,
	}

	service := &Service{
		orderRepo:     orderRepo,
		orderItemRepo: orderItemRepo,
		issuerRepo:    issuerRepo,
		payHistRepo:   payHistRepo,
		tlv2Repo:      tlv2repo,

		Datastore: datastore,

		wallet:           walletService,
		geminiClient:     geminiClient,
		geminiConf:       geminiConf,
		cbClient:         cbClient,
		stripeCl:         xstripe.NewClient(scClient),
		pauseVoteUntilMu: sync.RWMutex{},
		retry:            backoff.Retry,

		radomClient:  radomCl,
		radomGateway: radomGateway,
		radomAuth:    radomAuth,

		vendorReceiptValid: rcptValidator,
		gpsAuth:            newGPSNtfAuthenticator(gpsCfg, idv),
		assnCertVrf:        assnCertVrf,

		payProcCfg:    newPaymentProcessorConfig(env),
		newItemReqSet: newOrderItemReqNewMobileSet(env),
	}

	service.jobs = []srv.Job{
		{
			Func:    service.RunNextVoteDrainJob,
			Cadence: 2 * time.Second,
			Workers: 1,
		},
		{
			Func:    service.RunSendSigningRequestJob,
			Cadence: 100 * time.Millisecond,
			Workers: 1,
		},
	}

	if err := service.InitKafka(ctx); err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, appctx.KafkaBrokersCTXKey, os.Getenv("KAFKA_BROKERS"))

	if enabled, ok := ctx.Value(appctx.SkusEnableStoreSignedOrderCredsConsumer).(bool); ok && enabled {
		if consumers, ok := ctx.Value(appctx.SkusNumberStoreSignedOrderCredsConsumer).(int); ok {
			for i := 0; i < consumers; i++ {
				go service.RunStoreSignedOrderCredentials(ctx, 10*time.Second)
			}
		}
	}

	return service, nil
}

// CreateOrderFromRequest creates orders for Auto Contribute and Search Captcha.
//
// Deprecated: This method MUST NOT be used for Premium orders.
func (s *Service) CreateOrderFromRequest(ctx context.Context, req model.CreateOrderRequest) (*Order, error) {
	const merchantID = "brave.com"

	var (
		totalPrice            = decimal.New(0, 0)
		currency              string
		orderItems            []OrderItem
		location              string
		validFor              *time.Duration
		stripeSuccessURI      string
		stripeCancelURI       string
		allowedPaymentMethods []string
		numIntervals          int
		numPerInterval        = 2 // two per interval credentials to be submitted for signing
	)

	for i := 0; i < len(req.Items); i++ {
		orderItem, pm, issuerConfig, err := s.CreateOrderItemFromMacaroon(ctx, req.Items[i].SKU, req.Items[i].Quantity)
		if err != nil {
			return nil, err
		}

		// TODO: we ultimately need to figure out how to provision numPerInterval and numIntervals
		// on the order item instead of the order itself to support multiple orders with
		// different time limited v2 issuers.
		// For now leo sku needs 192 as num per interval.
		if orderItem.IsLeo() {
			numPerInterval = 192 // 192 credentials per day for leo
		}

		// Create issuer for sku. This only happens when a new sku is created.
		switch orderItem.CredentialType {
		case singleUse:
			if err := s.CreateIssuer(ctx, s.Datastore.RawDB(), merchantID, orderItem); err != nil {
				return nil, errorutils.Wrap(err, "error finding issuer")
			}
		case timeLimitedV2:
			if err := s.CreateIssuerV3(ctx, s.Datastore.RawDB(), merchantID, orderItem, *issuerConfig); err != nil {
				return nil, fmt.Errorf(
					"error creating issuer for merchantID %s and sku %s: %w",
					merchantID, orderItem.SKU, err,
				)
			}

			// set num tokens and token multi
			numIntervals = issuerConfig.Buffer + issuerConfig.Overlap
		}

		// make sure all the order item skus have the same allowed Payment Methods
		if i >= 1 {
			if err := model.EnsureEqualPaymentMethods(allowedPaymentMethods, pm); err != nil {
				return nil, err
			}
		} else {
			// first order item
			allowedPaymentMethods = pm
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
		metadataStripeSuccessURI, ok := orderItem.Metadata["stripe_success_uri"].(string)
		if ok {
			if stripeSuccessURI == "" {
				stripeSuccessURI = metadataStripeSuccessURI
			} else if stripeSuccessURI != metadataStripeSuccessURI {
				return nil, errors.New("all order items must have same stripe success uri")
			}
		}

		metadataStripeCancelURI, ok := orderItem.Metadata["stripe_cancel_uri"].(string)
		if ok {
			if stripeCancelURI == "" {
				stripeCancelURI = metadataStripeCancelURI
			} else if stripeCancelURI != metadataStripeCancelURI {
				return nil, errors.New("all order items must have same stripe cancel uri")
			}
		}

		orderItems = append(orderItems, *orderItem)
	}

	oreq := &model.OrderNew{
		MerchantID:            merchantID,
		Currency:              currency,
		Status:                OrderStatusPending,
		TotalPrice:            totalPrice,
		AllowedPaymentMethods: pq.StringArray(allowedPaymentMethods),
		ValidFor:              validFor,
	}

	// Consider the order paid if it consists entirely of zero cost items.
	if oreq.TotalPrice.IsZero() {
		oreq.Status = OrderStatusPaid
	}

	if location != "" {
		oreq.Location.Valid = true
		oreq.Location.String = location
	}

	tx, err := s.Datastore.BeginTx()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	order, err := s.Datastore.CreateOrder(ctx, tx, oreq, orderItems)
	if err != nil {
		return nil, fmt.Errorf("failed to create order: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	tx2, err := s.Datastore.BeginTx()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx2.Rollback() }()

	if numIntervals > 0 {
		if err := s.orderRepo.AppendMetadataInt(ctx, tx2, order.ID, "numIntervals", numIntervals); err != nil {
			return nil, fmt.Errorf("failed to update order metadata: %w", err)
		}
	}

	if numPerInterval > 0 {
		if err := s.orderRepo.AppendMetadataInt(ctx, tx2, order.ID, "numPerInterval", numPerInterval); err != nil {
			return nil, fmt.Errorf("failed to update order metadata: %w", err)
		}
	}

	if err := tx2.Commit(); err != nil {
		return nil, err
	}

	return order, nil
}

func (s *Service) getTransformOrder(ctx context.Context, orderID uuid.UUID) (*model.Order, error) {
	tx, err := s.Datastore.RawDB().BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	result, err := s.getTransformOrderTx(ctx, tx, orderID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return result, nil
}

func (s *Service) getTransformOrderTx(ctx context.Context, dbi sqlx.ExtContext, orderID uuid.UUID) (*model.Order, error) {
	ord, err := s.getOrderFullTx(ctx, dbi, orderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get order (%s): %w", orderID.String(), err)
	}

	// Nothing more to do for orders with a Stripe subscription.
	if _, ok := ord.StripeSubID(); ok {
		return ord, nil
	}

	if !shouldTransformStripeOrder(ord) {
		return ord, nil
	}

	if err := s.updateOrderStripeSession(ctx, dbi, ord); err != nil {
		return nil, fmt.Errorf("failed to transform stripe order (%s): %w", orderID.String(), err)
	}

	return s.getOrderFullTx(ctx, dbi, orderID)
}

// updateOrderStripeSession checks the status of the checkout session, updates it if expired.
func (s *Service) updateOrderStripeSession(ctx context.Context, dbi sqlx.ExtContext, ord *model.Order) error {
	expSessID, err := s.orderRepo.GetExpiredStripeCheckoutSessionID(ctx, dbi, ord.ID)
	if err != nil && !errors.Is(err, model.ErrExpiredStripeCheckoutSessionIDNotFound) {
		return fmt.Errorf("failed to check for expired stripe checkout session: %w", err)
	}

	var newSessID string

	if expSessID != "" {
		nsessID, err := s.recreateStripeSession(ctx, dbi, ord, expSessID)
		if err != nil {
			return fmt.Errorf("failed to create checkout session: %w", err)
		}

		newSessID = nsessID
	}

	// Below goes some leagcy stuff.
	// There was also a bug where the old subscription would be tested for payment.
	// The code below did not take into account that the session could have been updated just above.
	//
	// If this is a stripe order, and there is a checkout session, check it with Stripe.
	// The redirect flow sometimes is too fast for the webhook to be delivered.
	sessID, ok := chooseStripeSessID(ord, newSessID)
	if !ok || sessID == "" {
		// Nothing to do here.
		return nil
	}

	sess, err := s.stripeCl.Session(ctx, sessID, nil)
	if err != nil {
		return fmt.Errorf("failed to get stripe checkout session: %w", err)
	}

	// Skip unpaid sessions.
	if sess.PaymentStatus != "paid" {
		return nil
	}

	// Need to update the order as paid.
	// This requires fetching the subscription as the expiry time is needed.
	sub, err := s.stripeCl.Subscription(ctx, sess.Subscription.ID, nil)
	if err != nil {
		return err
	}

	expt := time.Unix(sub.CurrentPeriodEnd, 0).UTC()
	paidt := time.Unix(sub.CurrentPeriodStart, 0).UTC()

	return s.renewOrderStripe(ctx, dbi, ord, sub.ID, expt, paidt)
}

func (s *Service) CancelOrder(ctx context.Context, id uuid.UUID) error {
	tx, err := s.Datastore.RawDB().BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.cancelOrderTx(ctx, tx, id); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Service) cancelOrderTx(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID) error {
	return s.orderRepo.SetStatus(ctx, dbi, id, model.OrderStatusCanceled)
}

// CancelOrderLegacy cancels an order, propagates to stripe if needed.
func (s *Service) CancelOrderLegacy(orderID uuid.UUID) error {
	// TODO: Refactor this later. Now here's a quick fix.
	ord, err := s.Datastore.GetOrder(orderID)
	if err != nil {
		return err
	}

	if ord == nil {
		return model.ErrOrderNotFound
	}

	subID, ok := ord.StripeSubID()
	if ok && subID != "" {
		// Cancel the stripe subscription.
		if _, err := sub.Cancel(subID, nil); err != nil {
			// Error out if it's not 404.
			if !isErrStripeNotFound(err) {
				return fmt.Errorf("failed to cancel stripe subscription: %w", err)
			}
		}

		// Cancel even for 404.
		return s.Datastore.UpdateOrder(orderID, OrderStatusCanceled)
	}

	if ord.IsIOS() || ord.IsAndroid() {
		return s.Datastore.UpdateOrder(orderID, OrderStatusCanceled)
	}

	// Try to find by order_id in Stripe.
	params := &stripe.SubscriptionSearchParams{}
	params.Query = fmt.Sprintf("status:'active' AND metadata['orderID']:'%s'", orderID.String())

	ctx := context.TODO()

	iter := sub.Search(params)
	for iter.Next() {
		sb := iter.Subscription()
		if _, err := sub.Cancel(sb.ID, nil); err != nil {
			// It seems that already canceled subscriptions might return 404.
			if isErrStripeNotFound(err) {
				continue
			}

			return fmt.Errorf("failed to cancel stripe subscription: %w", err)
		}

		if err := s.Datastore.AppendOrderMetadata(ctx, &orderID, "stripeSubscriptionId", sb.ID); err != nil {
			return fmt.Errorf("failed to update order metadata with subscription id: %w", err)
		}
	}

	return s.Datastore.UpdateOrder(orderID, OrderStatusCanceled)
}

func (s *Service) SetOrderTrialDays(ctx context.Context, orderID *uuid.UUID, days int64) error {
	ord, err := s.Datastore.SetOrderTrialDays(ctx, orderID, days)
	if err != nil {
		return fmt.Errorf("failed to set the order's trial days: %w", err)
	}

	if !ord.ShouldSetTrialDays() {
		return nil
	}

	oldSessID, ok := ord.StripeSessID()
	if !ok {
		return model.ErrNoStripeCheckoutSessID
	}

	_, err = s.recreateStripeSession(ctx, s.Datastore.RawDB(), ord, oldSessID)

	return err
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

// getGeminiCustodialTx returns the custodial tx information from Gemini
func (s *Service) getGeminiCustodialTx(ctx context.Context, txRef string) (*decimal.Decimal, string, string, string, error) {
	if s.geminiConf == nil {
		return nil, "", "", "", errGeminiClientNotConfigured
	}

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

// CreateTransactionFromRequest queries the endpoints and creates a transaction
func (s *Service) CreateTransactionFromRequest(ctx context.Context, req CreateTransactionRequest, orderID uuid.UUID, getCustodialTx getCustodialTxFn) (*Transaction, error) {

	sublogger := logging.Logger(ctx, "payments").With().
		Str("func", "CreateAnonCardTransaction").
		Logger()

	// get the information from the custodian
	amount, status, currency, kind, err := getCustodialTx(ctx, req.ExternalTransactionID)
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to get and validate custodian transaction")
		return nil, errorutils.Wrap(err, fmt.Sprintf("failed to get get and validate custodialtx: %s", err.Error()))
	}

	transaction, err := s.Datastore.CreateTransaction(orderID, req.ExternalTransactionID, status, currency, kind, *amount)
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
	amount, status, currency, kind, err := getCustodialTx(ctx, req.ExternalTransactionID)
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to get and validate custodian transaction")
		return nil, errorutils.Wrap(err, fmt.Sprintf("failed to get get and validate custodialtx: %s", err.Error()))
	}

	transaction, err := s.Datastore.UpdateTransaction(orderID, req.ExternalTransactionID, status, currency, kind, *amount)
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

// UniqBatches returns the limit for active batches and the current number of active batches.
func (s *Service) UniqBatches(ctx context.Context, orderID, itemID uuid.UUID) (int, int, error) {
	now := time.Now()

	return s.uniqBatchesTxTime(ctx, s.Datastore.RawDB(), orderID, itemID, now, now)
}

func (s *Service) uniqBatchesTxTime(ctx context.Context, dbi sqlx.QueryerContext, orderID, itemID uuid.UUID, from, to time.Time) (int, int, error) {
	ord, err := s.getOrderFullTx(ctx, dbi, orderID)
	if err != nil {
		return 0, 0, err
	}

	if !ord.IsPaid() {
		return 0, 0, model.ErrOrderNotPaid
	}

	if len(ord.Items) == 0 {
		return 0, 0, model.ErrInvalidOrderNoItems
	}

	// Legacy: the method can be called with no itemID.
	item := &ord.Items[0]
	if !uuid.Equal(itemID, uuid.Nil) {
		var ok bool
		item, ok = ord.HasItem(itemID)
		if !ok {
			return 0, 0, model.ErrOrderItemNotFound
		}
	}

	if item.CredentialType != timeLimitedV2 {
		return 0, 0, model.ErrUnsupportedCredType
	}

	nact, err := s.tlv2Repo.UniqBatches(ctx, dbi, item.OrderID, item.ID, from, to)
	if err != nil {
		return 0, 0, err
	}

	return maxTLV2ActiveDailyItemCreds, nact, nil
}

// GetItemCredentials returns credentials based on the order, item and request id.
func (s *Service) GetItemCredentials(ctx context.Context, orderID, itemID, reqID uuid.UUID) (interface{}, int, error) {
	order, err := s.Datastore.GetOrder(orderID)
	if err != nil {
		return nil, http.StatusNotFound, fmt.Errorf("failed to get order: %w", err)
	}

	if order == nil {
		return nil, http.StatusNotFound, fmt.Errorf("failed to get order: %w", err)
	}

	item, ok := order.HasItem(itemID)
	if !ok {
		return nil, http.StatusNotFound, fmt.Errorf("failed to get item: %w", err)
	}

	switch item.CredentialType {
	case singleUse:
		return s.GetSingleUseCreds(ctx, order.ID, itemID, reqID)
	case timeLimited:
		return s.GetTimeLimitedCreds(ctx, order, itemID, reqID)
	case timeLimitedV2:
		return s.GetTimeLimitedV2Creds(ctx, order.ID, itemID, reqID)
	default:
		return nil, http.StatusConflict, model.ErrInvalidCredType
	}
}

// GetCredentials returns credentials on the order.
//
// This is a legacy method.
// For backward compatibility, similar to creating credentials, it uses item id as request id.
func (s *Service) GetCredentials(ctx context.Context, orderID uuid.UUID) (interface{}, int, error) {
	order, err := s.Datastore.GetOrder(orderID)
	if err != nil {
		return nil, http.StatusNotFound, fmt.Errorf("failed to get order: %w", err)
	}

	if order == nil {
		return nil, http.StatusNotFound, fmt.Errorf("failed to get order: %w", err)
	}

	if len(order.Items) != 1 {
		return nil, http.StatusConflict, model.Error("order must only have one item")
	}

	itemID := order.Items[0].ID

	switch order.Items[0].CredentialType {
	case singleUse:
		return s.GetSingleUseCreds(ctx, order.ID, itemID, itemID)
	case timeLimited:
		return s.GetTimeLimitedCreds(ctx, order, itemID, itemID)
	case timeLimitedV2:
		return s.GetTimeLimitedV2Creds(ctx, order.ID, itemID, itemID)
	default:
		return nil, http.StatusConflict, model.ErrInvalidCredType
	}
}

// GetSingleUseCreds returns single use credentials for a given order, item and request.
//
// If the credentials have been submitted but not yet signed it returns a http.StatusAccepted and an empty body.
// If the credentials have been signed it will return a http.StatusOK and the order credentials.
func (s *Service) GetSingleUseCreds(ctx context.Context, orderID, itemID, reqID uuid.UUID) ([]OrderCreds, int, error) {
	// Single use credentials retain the old semantics, only one request is ever allowed.
	creds, err := s.Datastore.GetOrderCredsByItemID(orderID, itemID, false)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to get single use creds: %w", err)
	}

	if creds != nil {
		return []OrderCreds{*creds}, http.StatusOK, nil
	}

	if _, err := s.Datastore.GetSigningOrderRequestOutboxByRequestID(ctx, s.Datastore.RawDB(), reqID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, http.StatusNotFound, errLegacySUCredsNotFound
		}

		return nil, http.StatusInternalServerError, fmt.Errorf("error getting outbox messages: %w", err)
	}

	// Given an outbox message was found i.e. it did not return a sql.ErrNoRows we can assume a message has been
	// submitted for signing but not yet complete as no creds were found.
	return nil, http.StatusAccepted, nil
}

// GetTimeLimitedV2Creds returns all the tlv2 credentials for a given order, item and request id.
//
// If the credentials have been submitted but not yet signed it returns a http.StatusAccepted and an empty body.
// If the credentials have been signed it will return a http.StatusOK and the time limited v2 credentials.
//
// Browser's api_request_helper does not understand Go's nil slices, hence explicit empty slice is returned.
func (s *Service) GetTimeLimitedV2Creds(ctx context.Context, orderID, itemID, reqID uuid.UUID) ([]TimeAwareSubIssuedCreds, int, error) {
	creds, err := s.Datastore.GetTLV2Creds(ctx, s.Datastore.RawDB(), orderID, itemID, reqID)
	if err != nil {
		return []TimeAwareSubIssuedCreds{}, http.StatusInternalServerError, fmt.Errorf("error getting credentials: %w", err)
	}

	if haveCreds(creds) {
		if active := filterActiveCreds(creds.Credentials, time.Now().UTC()); len(active) > 0 {
			return active, http.StatusOK, nil
		}

		return creds.Credentials, http.StatusOK, nil
	}

	obmsg, err := s.Datastore.GetSigningOrderRequestOutboxByRequestID(ctx, s.Datastore.RawDB(), reqID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []TimeAwareSubIssuedCreds{}, http.StatusNotFound, errLegacyOutboxNotFound
		}

		return []TimeAwareSubIssuedCreds{}, http.StatusInternalServerError, fmt.Errorf("error getting outbox messages: %w", err)
	}

	if !uuid.Equal(obmsg.OrderID, orderID) {
		return []TimeAwareSubIssuedCreds{}, http.StatusBadRequest, errWrongOrderIDForRequestID
	}

	if obmsg.CompletedAt == nil {
		return []TimeAwareSubIssuedCreds{}, http.StatusAccepted, errSetRetryAfter
	}

	// We have neither credentials nor a signing request so return an error.
	return []TimeAwareSubIssuedCreds{}, http.StatusInternalServerError, fmt.Errorf("error getting credentials: %w", err)
}

func filterActiveCreds(creds []TimeAwareSubIssuedCreds, now time.Time) []TimeAwareSubIssuedCreds {
	act := make([]TimeAwareSubIssuedCreds, 0)
	for i := range creds {
		if creds[i].ValidTo.After(now) {
			act = append(act, creds[i])
		}
	}

	return act
}

func haveCreds(creds *TimeLimitedV2Creds) bool {
	return creds != nil && len(creds.Credentials) > 0
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

	// Add a grace period of 5 days.
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

// GetTimeLimitedCreds returns get an order's time limited creds.
func (s *Service) GetTimeLimitedCreds(ctx context.Context, order *Order, itemID, reqID uuid.UUID) ([]TimeLimitedCreds, int, error) {
	if !order.IsPaid() || order.LastPaidAt == nil {
		return nil, http.StatusBadRequest, model.Error("order is not paid, or invalid last paid at")
	}

	issuedAt := order.LastPaidAt

	if order.ExpiresAt != nil {
		// Check if it's past expiration, if so issue nothing.
		if time.Now().After(*order.ExpiresAt) {
			return nil, http.StatusBadRequest, model.Error("order has expired")
		}
	}

	secret, err := s.GetActiveCredentialSigningKey(ctx, order.MerchantID)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to get merchant signing key: %w", err)
	}

	timeLimitedSecret := cryptography.NewTimeLimitedSecret(secret)

	item, ok := order.HasItem(itemID)
	if !ok {
		return nil, http.StatusBadRequest, model.Error("could not find specified item")
	}

	if item.ValidForISO == nil {
		return nil, http.StatusBadRequest, model.Error("order item has no valid for time")
	}

	duration, err := timeutils.ParseDuration(*item.ValidForISO)
	if err != nil {
		return nil, http.StatusInternalServerError, model.Error("unable to parse order duration for credentials")
	}

	if item.IssuanceIntervalISO == nil {
		item.IssuanceIntervalISO = ptrTo("P1D")
	}

	interval, err := timeutils.ParseDuration(*(item.IssuanceIntervalISO))
	if err != nil {
		return nil, http.StatusInternalServerError, model.Error("unable to parse issuance interval for credentials")
	}

	issuerID, err := encodeIssuerID(order.MerchantID, item.Issuer())
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("error encoding issuer: %w", err)
	}

	credentials, err := timeChunking(ctx, issuerID, timeLimitedSecret, order.ID, item.ID, *issuedAt, *duration, *interval)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to derive credential chunking: %w", err)
	}

	if len(credentials) == 0 {
		return nil, http.StatusBadRequest, model.Error("failed to issue credentials")
	}

	return credentials, http.StatusOK, nil
}

type credential interface {
	GetSKU() string
	GetType() string
	GetMerchantID() string
	GetPresentation() string
}

// verifyCredential - given a credential, verify it.
func (s *Service) verifyCredential(ctx context.Context, cred credential, w http.ResponseWriter) *handlers.AppError {
	logger := logging.Logger(ctx, "verifyCredential")

	merchant, err := merchantFromCtx(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("failed to get the merchant from the context")
		return handlers.WrapError(err, "Error getting auth merchant", http.StatusInternalServerError)
	}

	caveats := caveatsFromCtx(ctx)

	if merchID := cred.GetMerchantID(); merchID != merchant {
		logger.Warn().Str("req.MerchantID", merchID).Str("merchant", merchant).Msg("merchant does not match the key's merchant")
		return handlers.WrapError(nil, "Verify request merchant does not match authentication", http.StatusForbidden)
	}

	if caveats != nil {
		if sku, ok := caveats["sku"]; ok {
			if csku := cred.GetSKU(); csku != sku {
				logger.Warn().Str("req.SKU", csku).Str("sku", sku).Msg("sku caveat does not match")
				return handlers.WrapError(nil, "Verify request sku does not match authentication", http.StatusForbidden)
			}
		}
	}

	kind := cred.GetType()
	switch kind {
	case singleUse, timeLimitedV2:
		return s.verifyBlindedTokenCredential(ctx, cred, w)
	case timeLimited:
		return s.verifyTimeLimitedV1Credential(ctx, cred, w)
	default:
		return handlers.WrapError(nil, "Unknown credential type", http.StatusBadRequest)
	}
}

// verifyBlindedTokenCredential verifies a single use or time limited v2 credential.
func (s *Service) verifyBlindedTokenCredential(ctx context.Context, req credential, w http.ResponseWriter) *handlers.AppError {
	bytes, err := base64.StdEncoding.DecodeString(req.GetPresentation())
	if err != nil {
		return handlers.WrapError(err, "Error in decoding presentation", http.StatusBadRequest)
	}

	decodedCred := &cbr.CredentialRedemption{}
	if err := json.Unmarshal(bytes, decodedCred); err != nil {
		return handlers.WrapError(err, "Error in presentation formatting", http.StatusBadRequest)
	}

	// Ensure that the credential being redeemed (opaque to merchant) matches the outer credential details.
	issuerID, err := encodeIssuerID(req.GetMerchantID(), req.GetSKU())
	if err != nil {
		return handlers.WrapError(err, "Error in outer merchantId or sku", http.StatusBadRequest)
	}

	if issuerID != decodedCred.Issuer {
		lg := logging.Logger(ctx, "skus").With().Str("func", "verifyBlindedTokenCredential").Logger()
		lg.Err(model.Error("tlv2 issuer mismatch")).Str("issuer_id", issuerID).Str("decoded_issuer", decodedCred.Issuer).Msg("tlv2 issuer mismatch")

		return handlers.WrapError(nil, "Error, outer merchant and sku don't match issuer", http.StatusBadRequest)
	}

	return s.redeemBlindedCred(ctx, w, req.GetType(), decodedCred)
}

// verifyTimeLimitedV1Credential verifies a time limited v1 credential.
func (s *Service) verifyTimeLimitedV1Credential(ctx context.Context, req credential, w http.ResponseWriter) *handlers.AppError {
	data, err := base64.StdEncoding.DecodeString(req.GetPresentation())
	if err != nil {
		return handlers.WrapError(err, "Error in decoding presentation", http.StatusBadRequest)
	}

	present := &tlv1CredPresentation{}
	if err := json.Unmarshal(data, present); err != nil {
		return handlers.WrapError(err, "Error in presentation formatting", http.StatusBadRequest)
	}

	merchID := req.GetMerchantID()

	// Ensure that the credential being redeemed (opaque to merchant) matches the outer credential details.
	issuerID, err := encodeIssuerID(merchID, req.GetSKU())
	if err != nil {
		return handlers.WrapError(err, "Error in outer merchantId or sku", http.StatusBadRequest)
	}

	keys, err := s.GetCredentialSigningKeys(ctx, merchID)
	if err != nil {
		return handlers.WrapError(err, "failed to get merchant signing key", http.StatusInternalServerError)
	}

	issuedAt, err := time.Parse("2006-01-02", present.IssuedAt)
	if err != nil {
		return handlers.WrapError(err, "Error parsing issuedAt", http.StatusBadRequest)
	}

	expiresAt, err := time.Parse("2006-01-02", present.ExpiresAt)
	if err != nil {
		return handlers.WrapError(err, "Error parsing expiresAt", http.StatusBadRequest)
	}

	for _, key := range keys {
		timeLimitedSecret := cryptography.NewTimeLimitedSecret(key)

		verified, err := timeLimitedSecret.Verify([]byte(issuerID), issuedAt, expiresAt, present.Token)
		if err != nil {
			return handlers.WrapError(err, "Error in token verification", http.StatusBadRequest)
		}

		if verified {
			// Check against expiration time, issued time.
			now := time.Now()
			if now.After(expiresAt) || now.Before(issuedAt) {
				return handlers.WrapError(nil, "Credentials are not valid", http.StatusForbidden)
			}

			return handlers.RenderContent(ctx, "Credentials successfully verified", w, http.StatusOK)
		}
	}

	return handlers.WrapError(nil, "Credentials could not be verified", http.StatusForbidden)
}

// RunSendSigningRequestJob - send the order credentials signing requests
func (s *Service) RunSendSigningRequestJob(ctx context.Context) (bool, error) {
	return true, s.Datastore.SendSigningRequest(ctx, s)
}

// TODO: Address in kafka refactor

// RunStoreSignedOrderCredentials starts a signed order credentials consumer.
// This function creates a new signed order credentials consumer and starts processing messages.
// If the consumers errors we backoff, close the reader and restarts the consumer.
func (s *Service) RunStoreSignedOrderCredentials(ctx context.Context, backoff time.Duration) {
	logger := logging.Logger(ctx, "skus.RunStoreSignedOrderCredentials")

	decoder := &SigningOrderResultDecoder{
		codec: s.codecs[kafkaSignedOrderCredsTopic],
	}

	handler := &SignedOrderCredentialsHandler{
		decoder:   decoder,
		datastore: s.Datastore,
		tlv2Repo:  s.tlv2Repo,
	}

	errorHandler := &SigningOrderResultErrorHandler{
		kafkaWriter: s.kafkaWriter,
	}

	run := func() (err error) {
		reader, err := kafkautils.NewKafkaReader(ctx, kafkaSignedRequestReaderGroupID, kafkaSignedOrderCredsTopic)
		if err != nil {
			return fmt.Errorf("error creating kafka signed order credentials reader: %w", err)
		}
		defer func() {
			closeErr := reader.Close()
			if closeErr != nil {
				if err != nil {
					logger.Err(err).Msg("consumer error")
				}
				err = fmt.Errorf("error closing kafka reader: %w", errClosingResource)
			}
		}()

		err = kafkautils.Consume(ctx, reader, handler, errorHandler)
		if err != nil {
			return fmt.Errorf("consumer error: %w", err)
		}

		return nil
	}

	for {
		select {
		case <-ctx.Done():
			err := ctx.Err()
			if err != nil {
				logger.Err(err).Msg("error calling context")
			}
			return
		default:
			err := run()
			if err != nil {
				logger.Err(err).Msg("error running consumer")
				sentry.CaptureException(err)
				if errors.Is(err, errClosingResource) {
					return
				}
				time.Sleep(backoff)
			}
		}
	}
}

func (s *Service) processStripeNotification(ctx context.Context, ntf *stripeNotification) error {
	if !ntf.shouldProcess() {
		return nil
	}

	tx, err := s.Datastore.RawDB().BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.processStripeNotificationTx(ctx, tx, ntf); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Service) processStripeNotificationTx(ctx context.Context, dbi sqlx.ExtContext, ntf *stripeNotification) error {
	switch {
	case ntf.shouldRenew():
		subID, err := ntf.subID()
		if err != nil {
			return err
		}

		oid, err := ntf.orderID()
		if err != nil {
			return err
		}

		ord, err := s.orderRepo.Get(ctx, dbi, oid)
		if err != nil {
			return err
		}

		expt, err := ntf.expiresTime()
		if err != nil {
			return err
		}

		paidt := time.Now()

		return s.renewOrderStripe(ctx, dbi, ord, subID, expt, paidt)

	case ntf.shouldCancel():
		oid, err := ntf.orderID()
		if err != nil {
			return err
		}

		return s.cancelOrderTx(ctx, dbi, oid)

	default:
		return nil
	}
}

// processAppStoreNotification determines whether ntf is worth processing, and does it if it is.
//
// More on ntf types https://developer.apple.com/documentation/appstoreservernotifications/notificationtype#4304524.
func (s *Service) processAppStoreNotification(ctx context.Context, ntf *appStoreSrvNotification) error {
	if !ntf.shouldProcess() {
		return nil
	}

	txn, err := parseTxnInfo(ntf.pubKey, ntf.val.Data.SignedTransactionInfo)
	if err != nil {
		return err
	}

	tx, err := s.Datastore.RawDB().BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.processAppStoreNotificationTx(ctx, tx, ntf, txn); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Service) processAppStoreNotificationTx(ctx context.Context, dbi sqlx.ExtContext, ntf *appStoreSrvNotification, txn *appStoreTransaction) error {
	ord, err := s.orderRepo.GetByExternalID(ctx, dbi, txn.OriginalTransactionId)
	if err != nil {
		return err
	}

	switch {
	case ntf.shouldRenew():
		expt := txn.expiresTime().Add(24 * time.Hour)
		paidt := time.Now()

		return s.renewOrderWithExpPaidTimeTx(ctx, dbi, ord.ID, expt, paidt)

	case ntf.shouldCancel():
		return s.cancelOrderTx(ctx, dbi, ord.ID)

	default:
		return nil
	}
}

func (s *Service) processPlayStoreNotification(ctx context.Context, ntf *playStoreDevNotification) error {
	if !ntf.shouldProcess() {
		return nil
	}

	extID, ok := ntf.purchaseToken()
	if !ok {
		return nil
	}

	tx, err := s.Datastore.RawDB().BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.processPlayStoreNotificationTx(ctx, tx, ntf, extID); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Service) processPlayStoreNotificationTx(ctx context.Context, dbi sqlx.ExtContext, ntf *playStoreDevNotification, extID string) error {
	ord, err := s.orderRepo.GetByExternalID(ctx, dbi, extID)
	if err != nil {
		return err
	}

	switch {
	// Renewal.
	case ntf.SubscriptionNtf != nil && ntf.SubscriptionNtf.shouldRenew():
		sub, err := s.vendorReceiptValid.fetchSubPlayStore(ctx, ntf.PackageName, ntf.SubscriptionNtf.SubID, ntf.SubscriptionNtf.PurchaseToken)
		if err != nil {
			return err
		}

		expt := sub.expiresTime().Add(24 * time.Hour)
		paidt := time.Now()

		return s.renewOrderWithExpPaidTimeTx(ctx, dbi, ord.ID, expt, paidt)

	// Sub cancellation.
	case ntf.SubscriptionNtf != nil && ntf.SubscriptionNtf.shouldCancel():
		return s.cancelOrderTx(ctx, dbi, ord.ID)

	// Voiding.
	case ntf.VoidedPurchaseNtf != nil && ntf.VoidedPurchaseNtf.shouldProcess():
		return s.cancelOrderTx(ctx, dbi, ord.ID)

	default:
		return nil
	}
}

// validateReceipt validates receipt.
func (s *Service) validateReceipt(ctx context.Context, req model.ReceiptRequest) (model.ReceiptData, error) {
	switch req.Type {
	case model.VendorApple:
		return s.vendorReceiptValid.validateApple(ctx, req)

	case model.VendorGoogle:
		return s.vendorReceiptValid.validateGoogle(ctx, req)

	default:
		return model.ReceiptData{}, model.ErrInvalidVendor
	}
}

// CreateOrder creates a Premium order for the given req.
//
// For AC and Search Captcha, see s.CreateOrderFromRequest.
func (s *Service) CreateOrder(ctx context.Context, req *model.CreateOrderRequestNew) (*Order, error) {
	items, err := createOrderItems(req)
	if err != nil {
		return nil, err
	}

	ordNew, err := newOrderNewForReq(req, items, model.MerchID, model.OrderStatusPending)
	if err != nil {
		return nil, err
	}

	return s.createOrderPremium(ctx, req, ordNew, items)
}

func (s *Service) createOrderPremium(ctx context.Context, req *model.CreateOrderRequestNew, ordNew *model.OrderNew, items []model.OrderItem) (*model.Order, error) {
	tx, err := s.Datastore.RawDB().Beginx()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	numIntervals, err := s.createOrderIssuers(ctx, tx, model.MerchID, items)
	if err != nil {
		return nil, err
	}

	order, err := s.createOrderTx(ctx, tx, ordNew, items)
	if err != nil {
		return nil, fmt.Errorf("failed to create order: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	tx2, err := s.Datastore.RawDB().Beginx()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx2.Rollback() }()

	if !order.IsPaid() {
		switch {
		case order.IsStripePayable():
			ssid, err := s.createStripeSession(ctx, req, order)
			if err != nil {
				return nil, err
			}

			if err := s.orderRepo.AppendMetadata(ctx, tx2, order.ID, "stripeCheckoutSessionId", ssid); err != nil {
				return nil, fmt.Errorf("failed to update order metadata: %w", err)
			}

		// Backporting this from the legacy method CreateOrderFromRequest.
		case order.IsRadomPayable():
			ssid, err := s.createRadomSession(ctx, req, order)
			if err != nil {
				return nil, fmt.Errorf("failed to create checkout session: %w", err)
			}

			if err := s.orderRepo.AppendMetadata(ctx, tx2, order.ID, "radomCheckoutSessionId", ssid); err != nil {
				return nil, fmt.Errorf("failed to update order metadata: %w", err)
			}
		}
	}

	if err := s.updateOrderIntervals(ctx, tx2, order.ID, items, numIntervals); err != nil {
		return nil, fmt.Errorf("failed to update order metadata: %w", err)
	}

	if err := tx2.Commit(); err != nil {
		return nil, err
	}

	return order, nil
}

func (s *Service) updateOrderIntervals(ctx context.Context, dbi sqlx.ExecerContext, oid uuid.UUID, items []model.OrderItem, nintvals int) error {
	if nintvals > 0 {
		if err := s.orderRepo.AppendMetadataInt(ctx, dbi, oid, "numIntervals", nintvals); err != nil {
			return err
		}
	}

	// Backporting changes from https://github.com/brave-intl/bat-go/pull/1998.
	{
		numPerInterval := 2
		if len(items) == 1 && items[0].IsLeo() {
			numPerInterval = 192
		}

		if err := s.orderRepo.AppendMetadataInt(ctx, dbi, oid, "numPerInterval", numPerInterval); err != nil {
			return err
		}
	}

	return nil
}

// createOrderIssuers checks that the issuer exists for the item's product.
//
// TODO: Remove this when products & issuers have been reworked.
// The issuer for a product must be created when the product is created.
func (s *Service) createOrderIssuers(ctx context.Context, dbi sqlx.QueryerContext, merchID string, items []model.OrderItem) (int, error) {
	var numIntervals int
	for i := range items {
		switch items[i].CredentialType {
		case singleUse:
			if err := s.CreateIssuer(ctx, dbi, merchID, &items[i]); err != nil {
				return 0, errorutils.Wrap(err, "error finding issuer")
			}
		case timeLimitedV2:
			if err := s.CreateIssuerV3(ctx, dbi, merchID, &items[i], *items[i].IssuerConfig); err != nil {
				const msg = "error creating issuer for merchantID %s and sku %s: %w"
				return 0, fmt.Errorf(msg, merchID, items[i].SKU, err)
			}

			numIntervals = items[i].IssuerConfig.NumIntervals()
		}
	}

	return numIntervals, nil
}

func (s *Service) createStripeSession(ctx context.Context, req *model.CreateOrderRequestNew, order *model.Order) (string, error) {
	oid := order.ID.String()

	surl, err := req.StripeMetadata.SuccessURL(oid)
	if err != nil {
		return "", err
	}

	curl, err := req.StripeMetadata.CancelURL(oid)
	if err != nil {
		return "", err
	}

	sreq := createStripeSessionRequest{
		orderID:    oid,
		email:      req.Email,
		successURL: surl,
		cancelURL:  curl,
		trialDays:  order.GetTrialDays(),
		items:      buildStripeLineItems(order.Items),
	}

	return createStripeSession(ctx, s.stripeCl, sreq)
}

func (s *Service) createRadomSession(ctx context.Context, req *model.CreateOrderRequestNew, order *model.Order) (string, error) {
	oid := order.ID.String()

	surl, err := req.RadomMetadata.SuccessURL(oid)
	if err != nil {
		return "", err
	}

	curl, err := req.RadomMetadata.CancelURL(oid)
	if err != nil {
		return "", err
	}

	items, err := orderItemsToRadomLineItems(order.Items)
	if err != nil {
		return "", err
	}

	reqx := &radom.CheckoutSessionRequest{
		LineItems:  items,
		Gateway:    s.radomGateway,
		SuccessURL: surl,
		CancelURL:  curl,
		Metadata: []radom.Metadata{
			{
				Key:   "brave_order_id",
				Value: oid,
			},
		},
		ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
	}

	resp, err := s.radomClient.CreateCheckoutSession(ctx, reqx)
	if err != nil {
		return "", err
	}

	return resp.SessionID, nil
}

const errRadomProductIDNotFound = model.Error("product id not found in metadata")

func orderItemsToRadomLineItems(orderItems []model.OrderItem) ([]radom.LineItem, error) {
	lineItems := make([]radom.LineItem, 0, len(orderItems))
	for i := range orderItems {
		pid, ok := orderItems[i].RadomProductID()
		if !ok {
			return nil, errRadomProductIDNotFound
		}

		item := radom.LineItem{
			ProductID: pid,
		}

		lineItems = append(lineItems, item)
	}

	return lineItems, nil
}

func (s *Service) redeemBlindedCred(ctx context.Context, w http.ResponseWriter, kind string, cred *cbr.CredentialRedemption) *handlers.AppError {
	var redeemFn func(ctx context.Context, issuer, preimage, signature, payload string) error

	switch kind {
	case singleUse:
		redeemFn = s.cbClient.RedeemCredential
	case timeLimitedV2:
		redeemFn = s.cbClient.RedeemCredentialV3
	default:
		return handlers.WrapError(fmt.Errorf("credential type %s not suppoted", kind), "unknown credential type %s", http.StatusBadRequest)
	}

	// FIXME: we shouldn't be using the issuer as the payload, it ideally would be a unique request identifier
	// to allow for more flexible idempotent behavior.
	if err := redeemFn(ctx, cred.Issuer, cred.TokenPreimage, cred.Signature, cred.Issuer); err != nil {
		if !shouldRetryRedeemFn(kind, cred.Issuer, err) {
			return handleRedeemFnError(ctx, w, kind, cred, err)
		}

		// TODO: remove this as there should be no credentials in Production signed by brave-leo-premium-year.
		//
		// Fix for https://github.com/brave-intl/challenge-bypass-server/pull/371.
		const leoa = "brave.com?sku=brave-leo-premium-year"
		if err := redeemFn(ctx, leoa, cred.TokenPreimage, cred.Signature, cred.Issuer); err != nil {
			return handleRedeemFnError(ctx, w, kind, cred, err)
		}
	}

	// TODO(clD11): cleanup after quick fix
	if kind == timeLimitedV2 {
		return handlers.RenderContent(ctx, &blindedCredVrfResult{ID: cred.TokenPreimage}, w, http.StatusOK)
	}

	return handlers.RenderContent(ctx, "Credentials successfully verified", w, http.StatusOK)
}

func (s *Service) createOrderTx(ctx context.Context, dbi sqlx.ExtContext, oreq *model.OrderNew, items []model.OrderItem) (*model.Order, error) {
	result, err := s.orderRepo.Create(ctx, dbi, oreq)
	if err != nil {
		return nil, err
	}

	model.OrderItemList(items).SetOrderID(result.ID)

	result.Items, err = s.orderItemRepo.InsertMany(ctx, dbi, items...)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *Service) renewOrderWithExpPaidTime(ctx context.Context, id uuid.UUID, expt, paidt time.Time) error {
	tx, err := s.Datastore.RawDB().BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.renewOrderWithExpPaidTimeTx(ctx, tx, id, expt, paidt); err != nil {
		return err
	}

	return tx.Commit()
}

// renewOrderWithExpPaidTimeTx performs updates relevant to advancing a paid order forward after renewal.
//
// TODO: Add a repo method to update all three fields at once.
func (s *Service) renewOrderWithExpPaidTimeTx(ctx context.Context, dbi sqlx.ExecerContext, id uuid.UUID, expt, paidt time.Time) error {
	if err := s.orderRepo.SetStatus(ctx, dbi, id, model.OrderStatusPaid); err != nil {
		return err
	}

	if err := s.orderRepo.SetExpiresAt(ctx, dbi, id, expt); err != nil {
		return err
	}

	if err := s.orderRepo.SetLastPaidAt(ctx, dbi, id, paidt); err != nil {
		return err
	}

	if err := s.payHistRepo.Insert(ctx, dbi, id, paidt); err != nil {
		return err
	}

	return nil
}

func (s *Service) getOrderFull(ctx context.Context, id uuid.UUID) (*model.Order, error) {
	return s.getOrderFullTx(ctx, s.Datastore.RawDB(), id)
}

func (s *Service) getOrderFullTx(ctx context.Context, dbi sqlx.QueryerContext, id uuid.UUID) (*model.Order, error) {
	result, err := s.orderRepo.Get(ctx, dbi, id)
	if err != nil {
		return nil, err
	}

	result.Items, err = s.orderItemRepo.FindByOrderID(ctx, dbi, id)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *Service) appendOrderMetadata(ctx context.Context, oid uuid.UUID, mdata datastore.Metadata) error {
	tx, err := s.Datastore.RawDB().BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.appendOrderMetadataTx(ctx, tx, oid, mdata); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Service) appendOrderMetadataTx(ctx context.Context, dbi sqlx.ExecerContext, oid uuid.UUID, mdata datastore.Metadata) error {
	for k, v := range mdata {
		switch val := v.(type) {
		case string:
			if err := s.orderRepo.AppendMetadata(ctx, dbi, oid, k, val); err != nil {
				return err
			}

		case int:
			if err := s.orderRepo.AppendMetadataInt(ctx, dbi, oid, k, val); err != nil {
				return err
			}

		case int64:
			if err := s.orderRepo.AppendMetadataInt64(ctx, dbi, oid, k, val); err != nil {
				return err
			}

		// Related to the bug https://github.com/brave-intl/bat-go/blob/master/libs/datastore/models.go#L29.
		// Since no floats are stored originally, it's find to cast it back to int.
		case float64:
			if err := s.orderRepo.AppendMetadataInt(ctx, dbi, oid, k, int(val)); err != nil {
				return err
			}

		default:
			return model.ErrInvalidOrderMetadataType
		}
	}

	return nil
}

func (s *Service) createOrderWithReceipt(ctx context.Context, req model.ReceiptRequest) (*model.Order, error) {
	// 1. Fetch and validate the receipt.
	rcpt, err := s.validateReceipt(ctx, req)
	if err != nil {
		return nil, &receiptValidError{err: err}
	}

	// 2. Check for existence.
	ord, err := s.orderRepo.GetByExternalID(ctx, s.Datastore.RawDB(), rcpt.ExtID)
	if err != nil && !errors.Is(err, model.ErrOrderNotFound) {
		return nil, err
	}

	// 3. Return it if found. The caller must handle it accordingly.
	if err == nil {
		return ord, model.ErrOrderExistsForReceipt
	}

	// 4. Create if missing.
	paidt := time.Now()

	return createOrderWithReceipt(ctx, s, s.newItemReqSet, s.payProcCfg, rcpt, paidt)
}

func (s *Service) checkOrderReceipt(ctx context.Context, req model.ReceiptRequest, orderID uuid.UUID) error {
	rcpt, err := s.validateReceipt(ctx, req)
	if err != nil {
		return &receiptValidError{err: err}
	}

	return checkOrderReceipt(ctx, s.Datastore.RawDB(), s.orderRepo, orderID, rcpt.ExtID)
}

// processSubmitReceipt was meant to be used for updating the order based on the receipt.
//
// Deprecated: This code exists only temporary and is not used anymore.
// It will be deleted as soon as /submit-receipt no longer exists.
func (s *Service) processSubmitReceipt(ctx context.Context, req model.ReceiptRequest, oid uuid.UUID) (model.ReceiptData, error) {
	rcpt, err := s.validateReceipt(ctx, req)
	if err != nil {
		return model.ReceiptData{}, &receiptValidError{err: err}
	}

	tx, err := s.Datastore.RawDB().Beginx()
	if err != nil {
		return model.ReceiptData{}, err
	}
	defer func() { _ = tx.Rollback() }()

	ord, err := s.orderRepo.GetByExternalID(ctx, tx, rcpt.ExtID)
	if err != nil && !errors.Is(err, model.ErrOrderNotFound) {
		return model.ReceiptData{}, err
	}

	if err == nil {
		if !uuid.Equal(ord.ID, oid) {
			return model.ReceiptData{}, model.ErrNoMatchOrderReceipt
		}

		return model.ReceiptData{}, model.ErrReceiptAlreadyLinked
	}

	paidt := time.Now()

	if err := s.renewOrderWithExpPaidTimeTx(ctx, tx, oid, rcpt.ExpiresAt, paidt); err != nil {
		return model.ReceiptData{}, err
	}

	mdata := newMobileOrderMdata(rcpt.Type, rcpt.ExtID)
	if err := s.appendOrderMetadataTx(ctx, tx, ord.ID, mdata); err != nil {
		return model.ReceiptData{}, err
	}

	if err := tx.Commit(); err != nil {
		return model.ReceiptData{}, err
	}

	return rcpt, nil
}

const errRadomUnknownAction = model.Error("skus: unknown radom action")

func (s *Service) processRadomNotification(ctx context.Context, ntf *radom.Notification) error {
	if !ntf.ShouldProcess() {
		return nil
	}

	tx, err := s.Datastore.RawDB().BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.processRadomNotificationTx(ctx, tx, ntf); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Service) processRadomNotificationTx(ctx context.Context, dbi sqlx.ExtContext, ntf *radom.Notification) error {
	switch {
	case ntf.IsNewSub():
		oid, err := ntf.OrderID()
		if err != nil {
			return err
		}

		subID, err := ntf.SubID()
		if err != nil {
			return err
		}

		rsub, err := s.radomClient.GetSubscription(ctx, subID.String())
		if err != nil {
			return err
		}

		nxtB, err := rsub.NextBillingDate()
		if err != nil {
			return err
		}

		expAt := nxtB.Add(24 * time.Hour)

		paidAt, err := rsub.LastPaid()
		if err != nil {
			return err
		}

		if err := s.renewOrderWithExpPaidTimeTx(ctx, dbi, oid, expAt, paidAt); err != nil {
			return err
		}

		if err := s.orderRepo.AppendMetadata(ctx, dbi, oid, "externalID", subID.String()); err != nil {
			return err
		}

		return s.orderRepo.AppendMetadata(ctx, dbi, oid, "paymentProcessor", model.RadomPaymentMethod)

	case ntf.ShouldRenew():
		subID, err := ntf.SubID()
		if err != nil {
			return err
		}

		ord, err := s.orderRepo.GetByExternalID(ctx, dbi, subID.String())
		if err != nil {
			return err
		}

		rsub, err := s.radomClient.GetSubscription(ctx, subID.String())
		if err != nil {
			return err
		}

		nxtB, err := rsub.NextBillingDate()
		if err != nil {
			return err
		}

		expAt := nxtB.Add(24 * time.Hour)

		paidAt, err := rsub.LastPaid()
		if err != nil {
			return err
		}

		return s.renewOrderWithExpPaidTimeTx(ctx, dbi, ord.ID, expAt, paidAt)

	case ntf.ShouldCancel():
		subID, err := ntf.SubID()
		if err != nil {
			return err
		}

		ord, err := s.orderRepo.GetByExternalID(ctx, dbi, subID.String())
		if err != nil {
			return err
		}

		return s.cancelOrderTx(ctx, dbi, ord.ID)

	default:
		return errRadomUnknownAction
	}
}

func checkOrderReceipt(ctx context.Context, dbi sqlx.QueryerContext, repo orderStoreSvc, orderID uuid.UUID, extID string) error {
	ord, err := repo.GetByExternalID(ctx, dbi, extID)
	if err != nil {
		return err
	}

	if !uuid.Equal(orderID, ord.ID) {
		return model.ErrNoMatchOrderReceipt
	}

	return nil
}

// paidOrderCreator creates an order and sets its status to paid.
//
// This interface exists because in its current form Service is hardly testable.
type paidOrderCreator interface {
	createOrderPremium(ctx context.Context, req *model.CreateOrderRequestNew, ordNew *model.OrderNew, items []model.OrderItem) (*model.Order, error)
	renewOrderWithExpPaidTime(ctx context.Context, id uuid.UUID, expt, paidt time.Time) error
	appendOrderMetadata(ctx context.Context, oid uuid.UUID, mdata datastore.Metadata) error
}

// createOrderWithReceipt creates a paid order with the supplied inputs.
//
// The function does not re-fetch the order after the final update to metadata.
// This might change if there is such a need.
//
// NOTE: This is expressed as a function and not a method on Service due to the ugly dependency on Datastore inside s.createOrderPremium.
// That will eventually be refactored, and this will be promoted to a method once testing is possible without Datastore.
func createOrderWithReceipt(
	ctx context.Context,
	svc paidOrderCreator,
	itemReqSet map[string]model.OrderItemRequestNew,
	ppcfg *premiumPaymentProcConfig,
	rcpt model.ReceiptData,
	paidt time.Time,
) (*model.Order, error) {
	// 1. Find out what's being purchased from SubscriptionID.
	/*
		Android:
		- brave.leo.monthly -> brave-leo-premium
		- brave.leo.yearly -> brave-leo-premium-year
	*/
	itemNew, err := newOrderItemReqForSubID(itemReqSet, rcpt.ProductID)
	if err != nil {
		return nil, err
	}

	oreq := newCreateOrderReqNewMobile(ppcfg, itemNew)

	// 2. Craft a request for creating an order.
	items, err := createOrderItems(&oreq)
	if err != nil {
		return nil, err
	}

	// Use status paid as it's been already paid in-app.
	ordNew, err := newOrderNewForReq(&oreq, items, model.MerchID, model.OrderStatusPaid)
	if err != nil {
		return nil, err
	}

	// 3. Create an order.
	order, err := svc.createOrderPremium(ctx, &oreq, ordNew, items)
	if err != nil {
		return nil, err
	}

	// 4. Mark order as paid with proper expiration.
	if err := svc.renewOrderWithExpPaidTime(ctx, order.ID, rcpt.ExpiresAt, paidt); err != nil {
		return nil, err
	}

	// 5. Save mobile metadata.
	mdata := newMobileOrderMdata(rcpt.Type, rcpt.ExtID)
	if err := svc.appendOrderMetadata(ctx, order.ID, mdata); err != nil {
		return nil, err
	}

	// Not re-fetching the order after updating metadata.
	// At the moment, the only caller of this code is only interested
	// in the order id.

	return order, nil
}

func (s *Service) renewOrderStripe(ctx context.Context, dbi sqlx.ExecerContext, ord *model.Order, subID string, expt, paidt time.Time) error {
	if shouldUpdateOrderStripeSubID(ord, subID) {
		if err := s.orderRepo.AppendMetadata(ctx, dbi, ord.ID, "stripeSubscriptionId", subID); err != nil {
			return err
		}
	}

	// Add 1-day leeway in case next billing cycle's webhook gets delayed.
	expt = expt.Add(24 * time.Hour)

	if err := s.renewOrderWithExpPaidTimeTx(ctx, dbi, ord.ID, expt, paidt); err != nil {
		return err
	}

	// Skip updating payment processor if it's already Stripe.
	if ord.IsStripe() {
		return nil
	}

	return s.orderRepo.AppendMetadata(ctx, dbi, ord.ID, "paymentProcessor", model.StripePaymentMethod)
}

func (s *Service) recreateStripeSession(ctx context.Context, dbi sqlx.ExecerContext, ord *model.Order, oldSessID string) (string, error) {
	oldSess, err := s.stripeCl.Session(ctx, oldSessID, nil)
	if err != nil {
		return "", err
	}

	req := createStripeSessionRequest{
		orderID:    ord.ID.String(),
		email:      xstripe.CustomerEmailFromSession(oldSess),
		successURL: oldSess.SuccessURL,
		cancelURL:  oldSess.CancelURL,
		trialDays:  ord.GetTrialDays(),
		items:      buildStripeLineItems(ord.Items),
	}

	sessID, err := createStripeSession(ctx, s.stripeCl, req)
	if err != nil {
		return "", err
	}

	if err := s.orderRepo.AppendMetadata(ctx, dbi, ord.ID, "stripeCheckoutSessionId", sessID); err != nil {
		return "", err
	}

	return sessID, nil
}

func newOrderNewForReq(req *model.CreateOrderRequestNew, items []model.OrderItem, merchID, status string) (*model.OrderNew, error) {
	// Check for number of items to be above 0.
	//
	// Validation should already have taken care of this.
	// This function does not know about it, hence the explicit check.
	nitems := len(items)
	if nitems == 0 {
		return nil, model.ErrInvalidOrderRequest
	}

	result := &model.OrderNew{
		MerchantID:            merchID,
		Currency:              req.Currency,
		Status:                status,
		TotalPrice:            model.OrderItemList(items).TotalCost(),
		AllowedPaymentMethods: pq.StringArray(req.PaymentMethods),
	}

	if result.TotalPrice.IsZero() {
		result.Status = model.OrderStatusPaid
	}

	// Location on the order is only defined when there is only one item.
	//
	// Multi-item orders have NULL location.
	if nitems == 1 && items[0].Location.Valid {
		result.Location.Valid = true
		result.Location.String = items[0].Location.String
	}

	{
		// Use validFor from the first item.
		//
		// TODO: Deprecate the use of valid_for:
		// valid_for_iso is now used instead of valid_for for calculating order's expiration time.
		//
		// The old code in CreateOrderFromRequest does a contradictory thing – it takes validFor from last item.
		// It does not make any sense, but it's working because there is only one item normally.
		var vf time.Duration
		if items[0].ValidFor != nil {
			vf = *items[0].ValidFor
		}

		result.ValidFor = &vf
	}

	return result, nil
}

func createOrderItems(req *model.CreateOrderRequestNew) ([]model.OrderItem, error) {
	result := make([]model.OrderItem, 0)

	for i := range req.Items {
		item, err := createOrderItem(&req.Items[i])
		if err != nil {
			return nil, err
		}

		item.Currency = req.Currency

		result = append(result, *item)
	}

	return result, nil
}

func createOrderItem(req *model.OrderItemRequestNew) (*model.OrderItem, error) {
	if req.CredentialValidDurationEach != nil {
		if _, err := timeutils.ParseDuration(*req.CredentialValidDurationEach); err != nil {
			return nil, err
		}
	}

	validFor, err := durationFromISO(req.CredentialValidDuration)
	if err != nil {
		return nil, err
	}

	result := &model.OrderItem{
		SKU:    req.SKU,
		SKUVnt: req.SKUVnt,
		// Set Currency separately as it should be at the Order level.
		CredentialType:            req.CredentialType,
		ValidFor:                  &validFor,
		ValidForISO:               &req.CredentialValidDuration,
		EachCredentialValidForISO: req.CredentialValidDurationEach,
		IssuanceIntervalISO:       req.IssuanceInterval,

		Price: req.Price,
		Location: datastore.NullString{
			NullString: sql.NullString{
				Valid:  true,
				String: req.Location,
			},
		},
		Description: datastore.NullString{
			NullString: sql.NullString{
				Valid:  true,
				String: req.Description,
			},
		},
		Quantity: req.Quantity,
		Metadata: req.Metadata(),
		Subtotal: req.Price.Mul(decimal.NewFromInt(int64(req.Quantity))),
		IssuerConfig: &model.IssuerConfig{
			Buffer:  req.TokenBufferOrDefault(),
			Overlap: req.TokenOverlapOrDefault(),
		},
	}

	return result, nil
}

func newMobileOrderMdata(vnd model.Vendor, extID string) datastore.Metadata {
	result := datastore.Metadata{
		"externalID":       extID,
		"paymentProcessor": vnd.String(),
		"vendor":           vnd.String(),
	}

	return result
}

func durationFromISO(v string) (time.Duration, error) {
	dur, err := timeutils.ParseDuration(v)
	if err != nil {
		return 0, err
	}

	durt, err := dur.FromNow()
	if err != nil {
		return 0, err
	}

	return time.Until(*durt), nil
}

type blindedCredVrfResult struct {
	ID        string `json:"id"`
	Duplicate bool   `json:"duplicate"`
}

type tlv1CredPresentation struct {
	Token     string `json:"token"`
	IssuedAt  string `json:"issuedAt"`
	ExpiresAt string `json:"expiresAt"`
}

func ptrTo[T any](v T) *T {
	return &v
}

func isErrStripeNotFound(err error) bool {
	var serr *stripe.Error
	if !errors.As(err, &serr) {
		return false
	}

	return serr.HTTPStatusCode == http.StatusNotFound && serr.Code == stripe.ErrorCodeResourceMissing
}

type receiptValidError struct {
	err error
}

func (x *receiptValidError) Error() string {
	if x == nil || x.err == nil {
		return "nil"
	}

	return x.err.Error()
}

func shouldUpdateOrderStripeSubID(ord *model.Order, subID string) bool {
	if sid, ok := ord.StripeSubID(); !ok || sid != subID {
		return true
	}

	return false
}

func shouldTransformStripeOrder(ord *model.Order) bool {
	if ord.IsIOS() {
		return false
	}

	if ord.IsAndroid() {
		return false
	}

	return !ord.IsPaid() && ord.IsStripePayable()
}

func chooseStripeSessID(ord *model.Order, canBeNewSessID string) (string, bool) {
	if canBeNewSessID != "" {
		return canBeNewSessID, true
	}

	return ord.StripeSessID()
}

type createStripeSessionRequest struct {
	orderID    string
	email      string
	successURL string
	cancelURL  string
	trialDays  int64
	items      []*stripe.CheckoutSessionLineItemParams
}

func createStripeSession(ctx context.Context, cl stripeClient, req createStripeSessionRequest) (string, error) {
	params := &stripe.CheckoutSessionParams{
		PaymentMethodTypes: []*string{ptrTo("card")},
		Mode:               ptrTo(string(stripe.CheckoutSessionModeSubscription)),
		SuccessURL:         &req.successURL,
		CancelURL:          &req.cancelURL,
		ClientReferenceID:  &req.orderID,
		SubscriptionData:   &stripe.CheckoutSessionSubscriptionDataParams{},
		LineItems:          req.items,
	}

	// Email might not be given.
	// This could happen while recreating a session, and the email was not extracted from the old one.
	if req.email != "" {
		if cust, ok := cl.FindCustomer(ctx, req.email); ok && cust.Email != "" {
			params.Customer = &cust.ID
		} else {
			params.CustomerEmail = &req.email
		}
	}

	if req.trialDays > 0 {
		params.SubscriptionData.TrialPeriodDays = &req.trialDays
	}

	params.SubscriptionData.AddMetadata("orderID", req.orderID)
	params.AddExtra("allow_promotion_codes", "true")

	sess, err := cl.CreateSession(ctx, params)
	if err != nil {
		return "", err
	}

	return sess.ID, nil
}

func buildStripeLineItems(items []model.OrderItem) []*stripe.CheckoutSessionLineItemParams {
	var result []*stripe.CheckoutSessionLineItemParams

	for i := range items {
		priceID, ok := items[i].StripeItemID()
		if !ok {
			continue
		}

		result = append(result, &stripe.CheckoutSessionLineItemParams{
			Price:    ptrTo(priceID),
			Quantity: ptrTo(int64(items[i].Quantity)),
		})
	}

	return result
}

func handleRedeemFnError(ctx context.Context, w http.ResponseWriter, kind string, cred *cbr.CredentialRedemption, err error) *handlers.AppError {
	msg := err.Error()

	// Time limited v2: Expose a credential id so the caller can decide whether to allow multiple redemptions.
	if kind == timeLimitedV2 && msg == cbr.ErrDupRedeem.Error() {
		data := &blindedCredVrfResult{ID: cred.TokenPreimage, Duplicate: true}

		return handlers.RenderContent(ctx, data, w, http.StatusOK)
	}

	// Duplicate redemptions are not verified.
	if msg == cbr.ErrDupRedeem.Error() || msg == cbr.ErrBadRequest.Error() {
		return handlers.WrapError(err, "invalid credentials", http.StatusForbidden)
	}

	return handlers.WrapError(err, "Error verifying credentials", http.StatusInternalServerError)
}

func shouldRetryRedeemFn(kind, issuer string, err error) bool {
	const leo = "brave.com?sku=brave-leo-premium"

	return kind == timeLimitedV2 && issuer == leo && err.Error() == cbr.ErrBadRequest.Error()
}

func newRadomGateway(env string) (*radom.Gateway, error) {
	switch env {
	case "development", "staging":
		return &radom.Gateway{
			Managed: radom.Managed{
				Methods: []radom.Method{
					{
						Network: "SepoliaTestnet",
						Token:   "0x5D684d37922dAf7Aa2013E65A22880a11C475e25",
					},

					{
						Network: "PolygonTestnet",
						Token:   "0xd445cAAbb9eA6685D3A512439256866563a16E93",
					},
				},
			},
		}, nil
	case "production":
		return &radom.Gateway{
			Managed: radom.Managed{
				Methods: []radom.Method{
					{
						Network: "Polygon",
						Token:   "0x3cef98bb43d732e2f285ee605a8158cde967d219",
					},

					{
						Network: "Ethereum",
						Token:   "0x0d8775f648430679a709e98d2b0cb6250d2887ef",
					},
				},
			},
		}, nil
	default:
		return nil, model.Error("skus: unknown environment")
	}
}
