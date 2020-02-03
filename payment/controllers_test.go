// +build integration

package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type ControllersTestSuite struct {
	suite.Suite
}

func TestControllersTestSuite(t *testing.T) {
	suite.Run(t, new(ControllersTestSuite))
}

func (suite *ControllersTestSuite) SetupSuite() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	m, err := pg.NewMigrate()
	suite.Require().NoError(err, "Failed to create migrate instance")

	ver, dirty, _ := m.Version()
	if dirty {
		suite.Require().NoError(m.Force(int(ver)))
	}
	if ver > 0 {
		suite.Require().NoError(m.Down(), "Failed to migrate down cleanly")
	}

	suite.Require().NoError(pg.Migrate(), "Failed to fully migrate")
}

func (suite *ControllersTestSuite) setupCreateOrder(quantity int) Order {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	service := &Service{
		datastore: pg,
	}
	handler := CreateOrder(service)

	createRequest := &CreateOrderRequest{
		Items: []OrderItemRequest{
			{
				SKU:      "MDAxN2xvY2F0aW9uIGJyYXZlLmNvbQowMDFhaWRlbnRpZmllciBwdWJsaWMga2V5CjAwMzJjaWQgaWQgPSA1Yzg0NmRhMS04M2NkLTRlMTUtOThkZC04ZTE0N2E1NmI2ZmEKMDAxN2NpZCBjdXJyZW5jeSA9IEJBVAowMDE1Y2lkIHByaWNlID0gMC4yNQowMDJmc2lnbmF0dXJlICRlYyTuJdmlRFuPJ5XFQXjzHFZCLTek0yQ3Yc8JUKC0Cg",
				Quantity: quantity,
			},
		},
	}
	body, err := json.Marshal(&createRequest)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/v1/orders", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Assert().Equal(http.StatusCreated, rr.Code)

	var order Order
	err = json.Unmarshal(rr.Body.Bytes(), &order)
	suite.Assert().NoError(err)

	return order
}

func (suite *ControllersTestSuite) TestCreateOrder() {
	order := suite.setupCreateOrder(40)

	// Check the order
	suite.Assert().Equal("10", order.TotalPrice.String())
	suite.Assert().Equal("brave.com", order.MerchantID)
	suite.Assert().Equal("pending", order.Status)

	// Check the order items
	suite.Assert().Equal(len(order.Items), 1)
	suite.Assert().Equal("BAT", order.Items[0].Currency)
	suite.Assert().Equal("0.25", order.Items[0].Price.String())
	suite.Assert().Equal(40, order.Items[0].Quantity)
	suite.Assert().Equal(decimal.New(10, 0), order.Items[0].Subtotal)
	suite.Assert().Equal(order.ID, order.Items[0].OrderID)
}

func (suite *ControllersTestSuite) TestGetOrder() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	service := &Service{
		datastore: pg,
	}

	order := suite.setupCreateOrder(20)

	req, err := http.NewRequest("GET", "/v1/orders/{id}", nil)
	suite.Require().NoError(err)

	getOrderHandler := GetOrder(service)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", order.ID.String())
	getReq := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	getOrderHandler.ServeHTTP(rr, getReq)
	suite.Assert().Equal(http.StatusOK, rr.Code)

	err = json.Unmarshal(rr.Body.Bytes(), &order)
	suite.Assert().NoError(err)

	suite.Assert().Equal("5", order.TotalPrice.String())
	suite.Assert().Equal("brave.com", order.MerchantID)
	suite.Assert().Equal("pending", order.Status)

	// Check the order items
	suite.Assert().Equal(len(order.Items), 1)
	suite.Assert().Equal("BAT", order.Items[0].Currency)
	suite.Assert().Equal("0.25", order.Items[0].Price.String())
	suite.Assert().Equal(20, order.Items[0].Quantity)
	suite.Assert().Equal(decimal.New(5, 0), order.Items[0].Subtotal)
	suite.Assert().Equal(order.ID, order.Items[0].OrderID)
}

func (suite *ControllersTestSuite) TestCreateTransaction() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	service := &Service{
		datastore: pg,
	}
	order := suite.setupCreateOrder(4.75 / .25)

	handler := CreateTransaction(service)

	createRequest := &CreateTransactionRequest{
		ExternalTransactionID: "3db2f74e-df23-42e2-bf25-a302a93baa2d",
	}

	body, err := json.Marshal(&createRequest)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/v1/orders/{orderID}/transactions", bytes.NewBuffer(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	postReq := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	suite.Require().NoError(err)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, postReq)

	suite.Assert().Equal(http.StatusCreated, rr.Code)

	var transaction Transaction
	err = json.Unmarshal(rr.Body.Bytes(), &transaction)
	suite.Assert().NoError(err)

	// Check the transaction
	suite.Assert().Equal(decimal.NewFromFloat32(4.75), transaction.Amount)
	suite.Assert().Equal("uphold", transaction.Kind)
	suite.Assert().Equal("completed", transaction.Status)
	suite.Assert().Equal("BAT", transaction.Currency)
	suite.Assert().Equal(createRequest.ExternalTransactionID, transaction.ExternalTransactionID)
	suite.Assert().Equal(order.ID, transaction.OrderID, order.TotalPrice)

	// Check the order was updated to paid
	// Old order
	suite.Assert().Equal("pending", order.Status)
	// Check the new order
	updatedOrder, err := service.datastore.GetOrder(order.ID)
	suite.Assert().NoError(err)
	suite.Assert().Equal("paid", updatedOrder.Status)

	// Test to make sure we can't submit the same externalTransactionID twice

	req, err = http.NewRequest("POST", "/v1/orders/{orderID}/transactions", bytes.NewBuffer(body))
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	postReq = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	suite.Require().NoError(err)

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, postReq)
	suite.Assert().Equal(http.StatusBadRequest, rr.Code)
	suite.Assert().Equal(rr.Body.String(), "{\"message\":\"Error creating the transaction: External Transaction ID: 3db2f74e-df23-42e2-bf25-a302a93baa2d has already been added to the order\",\"code\":400}\n")
}

func (suite *ControllersTestSuite) TestGetTransactions() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	service := &Service{
		datastore: pg,
	}

	// Delete transactions so we don't run into any validation errors
	_, err = pg.DB.Exec("DELETE FROM transactions;")
	suite.Require().NoError(err)

	order := suite.setupCreateOrder(4.75 / .25)

	handler := CreateTransaction(service)

	createRequest := &CreateTransactionRequest{
		ExternalTransactionID: "3db2f74e-df23-42e2-bf25-a302a93baa2d",
	}

	body, err := json.Marshal(&createRequest)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/v1/orders/{orderID}/transactions", bytes.NewBuffer(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	postReq := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	suite.Require().NoError(err)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, postReq)

	suite.Assert().Equal(http.StatusCreated, rr.Code)

	var transaction Transaction
	err = json.Unmarshal(rr.Body.Bytes(), &transaction)
	suite.Assert().NoError(err)

	// Check the transaction
	suite.Assert().Equal(decimal.NewFromFloat32(4.75), transaction.Amount)
	suite.Assert().Equal("uphold", transaction.Kind)
	suite.Assert().Equal("completed", transaction.Status)
	suite.Assert().Equal("BAT", transaction.Currency)
	suite.Assert().Equal(createRequest.ExternalTransactionID, transaction.ExternalTransactionID)
	suite.Assert().Equal(order.ID, transaction.OrderID)

	// Check the order was updated to paid
	// Old order
	suite.Assert().Equal("pending", order.Status)
	// Check the new order
	updatedOrder, err := service.datastore.GetOrder(order.ID)
	suite.Assert().NoError(err)
	suite.Assert().Equal("paid", updatedOrder.Status)

	// Get all the transactions, should only be one

	handler = GetTransactions(service)
	req, err = http.NewRequest("GET", "/v1/orders/{orderID}/transactions", nil)
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("orderID", order.ID.String())
	getReq := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	suite.Require().NoError(err)

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, getReq)

	suite.Assert().Equal(http.StatusOK, rr.Code)
	var transactions []Transaction
	err = json.Unmarshal(rr.Body.Bytes(), &transactions)
	suite.Assert().NoError(err)

	// Check the transaction
	suite.Assert().Equal(decimal.NewFromFloat32(4.75), transactions[0].Amount)
	suite.Assert().Equal("uphold", transactions[0].Kind)
	suite.Assert().Equal("completed", transactions[0].Status)
	suite.Assert().Equal("BAT", transactions[0].Currency)
	suite.Assert().Equal(createRequest.ExternalTransactionID, transactions[0].ExternalTransactionID)
	suite.Assert().Equal(order.ID, transactions[0].OrderID)
}
