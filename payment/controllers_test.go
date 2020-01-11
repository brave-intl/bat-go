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

func (suite *ControllersTestSuite) TestCreateOrder() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	service := &Service{
		datastore: pg,
	}

	handler := CreateOrder(service)

	createRequest := &CreateOrderRequest{
		Items: []OrderItemRequest{
			{
				SKU:     "MDAxN2xvY2F0aW9uIGJyYXZlLmNvbQowMDFhaWRlbnRpZmllciBwdWJsaWMga2V5CjAwMzJjaWQgaWQgPSA1Yzg0NmRhMS04M2NkLTRlMTUtOThkZC04ZTE0N2E1NmI2ZmEKMDAxN2NpZCBjdXJyZW5jeSA9IEJBVAowMDE1Y2lkIHByaWNlID0gMC4yNQowMDJmc2lnbmF0dXJlICRlYyTuJdmlRFuPJ5XFQXjzHFZCLTek0yQ3Yc8JUKC0Cg",
				Quanity: 40,
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

	// Create the order first
	handler := CreateOrder(service)
	createRequest := &CreateOrderRequest{
		Items: []OrderItemRequest{
			{
				SKU:     "MDAxN2xvY2F0aW9uIGJyYXZlLmNvbQowMDFhaWRlbnRpZmllciBwdWJsaWMga2V5CjAwMzJjaWQgaWQgPSA1Yzg0NmRhMS04M2NkLTRlMTUtOThkZC04ZTE0N2E1NmI2ZmEKMDAxN2NpZCBjdXJyZW5jeSA9IEJBVAowMDE1Y2lkIHByaWNlID0gMC4yNQowMDJmc2lnbmF0dXJlICRlYyTuJdmlRFuPJ5XFQXjzHFZCLTek0yQ3Yc8JUKC0Cg",
				Quanity: 20,
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
	err = json.Unmarshal([]byte(rr.Body.String()), &order)
	suite.Assert().NoError(err)

	req, err = http.NewRequest("GET", "/v1/orders/{id}", nil)
	suite.Require().NoError(err)

	getOrderHandler := GetOrder(service)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", order.ID.String())
	getReq := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr = httptest.NewRecorder()
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
