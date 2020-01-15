package payment

import (
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Service contains datastore
type Service struct {
	datastore Datastore
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(datastore Datastore) (*Service, error) {
	service := &Service{
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
