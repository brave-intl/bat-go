package payment

import (
	"context"
	"fmt"

	"github.com/brave-intl/bat-go/utils/clients/cbr"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	wallet "github.com/brave-intl/bat-go/wallet/service"
	"github.com/pkg/errors"
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
		orderItem := createOrderItemFromMacaroon(req.Items[i].SKU, req.Items[i].Quanity)
		totalPrice = totalPrice.Add(orderItem.Subtotal)

		orderItems = append(orderItems, orderItem)
	}

	order, err := service.datastore.CreateOrder(totalPrice, "brave.com", "pending", orderItems)

	return order, err
}

// UpdateOrderStatus checks to see if an order has been paid and updates it if so
func (service *Service) UpdateOrderStatus(orderID uuid.UUID) error {
	order, err := service.datastore.GetOrder(orderID)
	if err != nil {
		return err
	}

	sum, err := service.datastore.GetSumForTransactions(orderID)
	if err != nil {
		return err
	}

	if sum.GreaterThanOrEqual(order.TotalPrice) {
		err = service.datastore.UpdateOrder(orderID, "paid")
		if err != nil {
			return err
		}
	}

	return nil
}

// CreateTransactionFromRequest queries the endpoints and creates a transaciton
func (service *Service) CreateTransactionFromRequest(req CreateTransactionRequest, orderID uuid.UUID) (*Transaction, error) {

	var wallet uphold.Wallet
	upholdTransaction, err := wallet.GetPublicTransaction(req.ExternalTransactionID)

	if err != nil {
		return nil, err
	}

	amount := upholdTransaction.AltCurrency.FromProbi(upholdTransaction.Probi)
	status := upholdTransaction.Status
	currency := upholdTransaction.AltCurrency.String()
	kind := "uphold"

	transaction, err := service.datastore.CreateTransaction(orderID, req.ExternalTransactionID, status, currency, kind, amount)
	if err != nil {
		return nil, errors.Wrap(err, "Error recording transaction")
	}

	err = service.UpdateOrderStatus(orderID)
	if err != nil {
		return nil, errors.Wrap(err, "Error updating order status")
	}

	return transaction, err
}

// CreateAnonCardTransaction takes a signed transaction and executes it on behalf of an anon card
func (service *Service) CreateAnonCardTransaction(ctx context.Context, walletID uuid.UUID, transaction string, orderID uuid.UUID) (*Transaction, error) {
	txInfo, err := service.wallet.SubmitAnonCardTransaction(ctx, walletID, transaction)
	if err != nil {
		return nil, errors.Wrap(err, "Error submitting anon card transaction")
	}
	fmt.Println(txInfo)

	txn, err := service.datastore.CreateTransaction(orderID, txInfo.ID, txInfo.Status, txInfo.DestCurrency, "anonymous-card", txInfo.DestAmount)
	if err != nil {
		return nil, errors.Wrap(err, "Error recording anon card transaction")
	}

	err = service.UpdateOrderStatus(orderID)
	if err != nil {
		return nil, errors.Wrap(err, "Error updating order status")
	}

	return txn, err
}

// RunNextOrderJob takes the next order job and completes it
func (service *Service) RunNextOrderJob(ctx context.Context) (bool, error) {
	return service.datastore.RunNextOrderJob(ctx, service)
}
