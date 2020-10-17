package payment

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"errors"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	srv "github.com/brave-intl/bat-go/utils/service"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/linkedin/goavro"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/brave-intl/bat-go/utils/clients/cbr"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	kafkautils "github.com/brave-intl/bat-go/utils/kafka"
	uuid "github.com/satori/go.uuid"
	kafka "github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
)

var (
	voteTopic          = os.Getenv("ENV") + ".payment.vote"
	kafkaCertNotBefore = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "kafka_cert_not_before",
		Help: "Date when the kafka certificate becomes valid.",
	})
	kafkaCertNotAfter = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "kafka_cert_not_after",
		Help: "Date when the kafka certificate expires.",
	})
)

func init() {
	// put environment on context before logger setup
	ctx := context.WithValue(context.Background(), appctx.EnvironmentCTXKey, os.Getenv("ENV"))
	_, logger := logging.SetupLogger(ctx)
	var err error
	// gracefully try to register collectors for prom, no need to panic
	err = prometheus.Register(kafkaCertNotBefore)
	if _, ok := err.(prometheus.AlreadyRegisteredError); ok {
		logger.Warn().Err(err).Msg("already registered kafkaCertNotBefore collector")
	}
	err = prometheus.Register(kafkaCertNotAfter)
	if _, ok := err.(prometheus.AlreadyRegisteredError); ok {
		logger.Warn().Err(err).Msg("already registered kafkaCertNotAfter collector")
	}
}

// Service contains datastore
type Service struct {
	wallet           *wallet.Service
	cbClient         cbr.Client
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

// InitCodecs used for Avro encoding / decoding
func (s *Service) InitCodecs() error {
	s.codecs = make(map[string]*goavro.Codec)

	voteEventCodec, err := goavro.NewCodec(string(voteSchema))
	s.codecs["vote"] = voteEventCodec
	if err != nil {
		return err
	}
	return nil
}

// InitKafka by creating a kafka writer and creating local copies of codecs
func (s *Service) InitKafka() error {

	_, logger := logging.SetupLogger(context.Background())

	dialer, x509Cert, err := kafkautils.TLSDialer()
	if err != nil {
		return err
	}
	s.kafkaDialer = dialer

	kafkaCertNotBefore.Set(float64(x509Cert.NotBefore.Unix()))
	kafkaCertNotAfter.Set(float64(x509Cert.NotAfter.Unix()))

	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	kafkaWriter := kafka.NewWriter(kafka.WriterConfig{
		// by default we are waitng for acks from all nodes
		Brokers:  strings.Split(kafkaBrokers, ","),
		Topic:    voteTopic,
		Balancer: &kafka.LeastBytes{},
		Dialer:   dialer,
		Logger:   kafka.LoggerFunc(logger.Printf), // FIXME
	})

	s.kafkaWriter = kafkaWriter
	err = s.InitCodecs()
	if err != nil {
		return err
	}

	return nil
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(ctx context.Context, datastore Datastore, walletService *wallet.Service) (*Service, error) {
	cbClient, err := cbr.New()
	if err != nil {
		return nil, err
	}

	service := &Service{
		wallet:           walletService,
		cbClient:         cbClient,
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
			Cadence: 5 * time.Second,
			Workers: 1,
		},
	}

	err = service.InitKafka()
	if err != nil {
		return nil, err
	}

	return service, nil
}

// CreateOrderFromRequest creates an order from the request
func (s *Service) CreateOrderFromRequest(req CreateOrderRequest) (*Order, error) {
	totalPrice := decimal.New(0, 0)
	orderItems := []OrderItem{}
	var currency string
	var location string

	for i := 0; i < len(req.Items); i++ {
		orderItem, err := CreateOrderItemFromMacaroon(req.Items[i].SKU, req.Items[i].Quantity)
		if err != nil {
			return nil, err
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
		orderItems = append(orderItems, *orderItem)
	}

	order, err := s.Datastore.CreateOrder(totalPrice, "brave.com", "pending", currency, location, orderItems)

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
	return s.Datastore.RunNextOrderJob(ctx, s)
}
