package payment

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/stripe/stripe-go/client"

	"errors"

	srv "github.com/brave-intl/bat-go/utils/service"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/getsentry/sentry-go"
	"github.com/linkedin/goavro"
	"github.com/stripe/stripe-go"

	"github.com/brave-intl/bat-go/utils/clients/cbr"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	kafkautils "github.com/brave-intl/bat-go/utils/kafka"
	uuid "github.com/satori/go.uuid"
	kafka "github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
)

var (
	voteTopic = os.Getenv("ENV") + ".payment.vote"
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
	sublogger := logger(ctx).With().Str("func", "InitService").Logger()

	// setup stripe if exists in context and enabled
	var scClient = &client.API{}
	if enabled, ok := ctx.Value(appctx.StripeEnabledCTXKey).(bool); ok && enabled {
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
func (s *Service) CreateOrderFromRequest(req CreateOrderRequest) (*Order, error) {
	totalPrice := decimal.New(0, 0)
	orderItems := []OrderItem{}
	var (
		currency              string
		location              string
		stripeSuccessURI      string
		stripeCancelURI       string
		status                string
		allowedPaymentMethods = new(Methods)
	)

	for i := 0; i < len(req.Items); i++ {
		orderItem, pm, err := s.CreateOrderItemFromMacaroon(req.Items[i].SKU, req.Items[i].Quantity)
		if err != nil {
			return nil, err
		}

		// make sure all the order item skus have the same allowed Payment Methods
		if i > 1 {
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
		status = "paid"
	} else {
		status = "pending"
	}

	order, err := s.Datastore.CreateOrder(totalPrice, "brave.com", status, currency, location, orderItems, allowedPaymentMethods)

	if err != nil {
		return nil, fmt.Errorf("failed to create order: %w", err)
	}

	if !order.IsPaid() && order.IsStripePayable() {
		checkoutSession, err := order.CreateStripeCheckoutSession(
			req.Email, stripeSuccessURI, stripeCancelURI)
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

// UpdateOrderStatus checks to see if an order has been paid and updates it if so
func (s *Service) UpdateOrderStatus(orderID uuid.UUID) error {
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

// UpdateOrderMetadata checks to see if an order has been paid and updates it if so
func (s *Service) UpdateOrderMetadata(orderID uuid.UUID, key string, value string) error {
	err := s.Datastore.UpdateOrderMetadata(orderID, key, value)
	if err != nil {
		return err
	}
	return nil
}

// CreateTransactionFromRequest queries the endpoints and creates a transaciton
func (s *Service) CreateTransactionFromRequest(req CreateTransactionRequest, orderID uuid.UUID) (*Transaction, error) {
	var wallet uphold.Wallet
	upholdTransaction, err := wallet.GetTransaction(req.ExternalTransactionID.String())

	if err != nil {
		return nil, err
	}

	amount := upholdTransaction.AltCurrency.FromProbi(upholdTransaction.Probi)
	status := upholdTransaction.Status
	currency := upholdTransaction.AltCurrency.String()
	kind := "uphold"

	// check if destination is the right address
	if upholdTransaction.Destination != uphold.UpholdSettlementAddress {
		return nil, errors.New("error recording transaction: invalid settlement address")
	}

	transaction, err := s.Datastore.CreateTransaction(orderID, req.ExternalTransactionID.String(), status, currency, kind, amount)
	if err != nil {
		return nil, errorutils.Wrap(err, "error recording transaction")
	}

	isPaid, err := s.IsOrderPaid(transaction.OrderID)
	if err != nil {
		return nil, errorutils.Wrap(err, "error submitting anon card transaction")
	}

	// If the transaction that was satisifies the order then let's update the status
	if isPaid {
		err = s.Datastore.UpdateOrder(transaction.OrderID, "paid")
		if err != nil {
			return nil, errorutils.Wrap(err, "error updating order status")
		}
	}

	return transaction, err
}

// CreateAnonCardTransaction takes a signed transaction and executes it on behalf of an anon card
func (s *Service) CreateAnonCardTransaction(ctx context.Context, walletID uuid.UUID, transaction string, orderID uuid.UUID) (*Transaction, error) {
	txInfo, err := s.wallet.SubmitAnonCardTransaction(
		ctx,
		walletID,
		transaction,
		uphold.AnonCardSettlementAddress,
	)
	if err != nil {
		return nil, errorutils.Wrap(err, "error submitting anon card transaction")
	}

	txn, err := s.Datastore.CreateTransaction(orderID, txInfo.ID, txInfo.Status, txInfo.DestCurrency, "anonymous-card", txInfo.DestAmount)
	if err != nil {
		return nil, errorutils.Wrap(err, "error recording anon card transaction")
	}

	err = s.UpdateOrderStatus(orderID)
	if err != nil {
		return nil, errorutils.Wrap(err, "error updating order status")
	}

	return txn, err
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
