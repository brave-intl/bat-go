package payment

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"errors"

	srv "github.com/brave-intl/bat-go/utils/service"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/linkedin/goavro"

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
	var currency string
	var location string
	var status string

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

	// If order consists entirely of zero cost items ( e.g. trials ), we can consider it paid
	if totalPrice.IsZero() {
		status = "paid"
	} else {
		status = "pending"
	}

	order, err := s.Datastore.CreateOrder(totalPrice, "brave.com", status, currency, location, orderItems)

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
