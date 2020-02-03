package payment

import (
	"context"
	"fmt"

	"github.com/brave-intl/bat-go/utils/clients/cbr"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	wallet "github.com/brave-intl/bat-go/wallet/service"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Service contains datastore
type Service struct {
	wallet    wallet.Service
	cbClient  cbr.Client
	datastore Datastore
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(datastore Datastore) (*Service, error) {
	cbClient, err := cbr.New()
	if err != nil {
		return nil, err
	}

	walletService, err := wallet.InitService(datastore, nil)
	if err != nil {
		return nil, err
	}

	service := &Service{
		wallet:    *walletService,
		cbClient:  cbClient,
		datastore: datastore,
	}
	return service, nil
}

// CreateOrderFromRequest creates an order from the request
func (service *Service) CreateOrderFromRequest(req CreateOrderRequest) (*Order, error) {
	totalPrice := decimal.New(0, 0)
	orderItems := []OrderItem{}
	for i := 0; i < len(req.Items); i++ {
		orderItem, err := createOrderItemFromMacaroon(req.Items[i].SKU, req.Items[i].Quantity)
		if err != nil {
			return nil, err
		}
		totalPrice = totalPrice.Add(orderItem.Subtotal)

		orderItems = append(orderItems, *orderItem)
	}

	order, err := service.datastore.CreateOrder(totalPrice, "brave.com", "pending", orderItems)

	return order, err
}

// CreateTransactionFromRequest queries the endpoints and creates a transaciton
func (service *Service) CreateTransactionFromRequest(req CreateTransactionRequest, orderID uuid.UUID) (*Transaction, error) {
	// Ensure the transaction hasn't already been added to any orders.
	transaction, err := service.datastore.GetTransaction(req.ExternalTransactionID)

	if err != nil {
		return nil, err
	}
	if transaction != nil {
		return nil, fmt.Errorf("External Transaction ID: %s has already been added to the order", req.ExternalTransactionID)
	}

	var wallet uphold.Wallet
	upholdTransaction, err := wallet.GetPublicTransaction(req.ExternalTransactionID)

	if err != nil {
		return nil, err
	}

	amount := upholdTransaction.AltCurrency.FromProbi(upholdTransaction.Probi)
	status := upholdTransaction.Status
	currency := upholdTransaction.AltCurrency.String()
	kind := "uphold"

	transaction, err = service.datastore.CreateTransaction(orderID, req.ExternalTransactionID, status, currency, kind, amount)
	if err != nil {
		return nil, err
	}

	// Now that the transaction has been created let's check to see if that fulfilled the order.
	order, err := service.datastore.GetOrder(orderID)
	if err != nil {
		return nil, err
	}

	// Get current sums for transactions
	sum, err := service.datastore.GetSumForTransactions(orderID)
	if err != nil {
		return nil, err
	}

	// If the transaction that was inserted satisifies the order then let's update the status
	if sum.GreaterThanOrEqual(order.TotalPrice) {
		err = service.datastore.UpdateOrder(orderID, "paid")
		if err != nil {
			return nil, err
		}
	}

	return transaction, err
}

// RunNextOrderJob takes the next order job and completes it
func (service *Service) RunNextOrderJob(ctx context.Context) (bool, error) {
	return service.datastore.RunNextOrderJob(ctx, service)
}
