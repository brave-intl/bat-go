package payment

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type OrderTestSuite struct {
	suite.Suite
}

func TestOrderTestSuite(t *testing.T) {
	suite.Run(t, new(OrderTestSuite))
}

func (suite *OrderTestSuite) TestCreateOrderItemFromMacaroon() {
	sku := "MDAxZmxvY2F0aW9uIGh0dHBzOi8vYnJhdmUuY29tCjAwMWFpZGVudGlmaWVyIGlkZW50aWZpZXIKMDAyYmNpZCBleHBpcnkgPSAzMDIwLTAzLTA1VDEzOjIwOjI3LTA2OjAwCjAwMTdjaWQgY3VycmVuY3kgPSBCQVQKMDAzMmNpZCBpZCA9IDVjODQ2ZGExLTgzY2QtNGUxNS05OGRkLThlMTQ3YTU2YjZmYQowMDJiY2lkIGRlc2NyaXB0aW9uID0gY29mZmVlIGNvZmZlZSBjb2ZmZWUKMDAxNWNpZCBwcmljZSA9IDIuMDAKMDAyZnNpZ25hdHVyZSBBejw7oGlCuSe61soF1nJsWVQeJwRucj5jZwtUc2y4Ugo"
	// identifier: identifier
	// rootKey: secret
	// location: brave.com
	// caveats:
	//     id = 5c846da1-83cd-4e15-98dd-8e147a56b6fa
	//     currency = BAT
	//     price = 2.00
	//     description = coffee coffee coffee
	//     expiry = 2020-04-01

	orderItem, err := CreateOrderItemFromMacaroon(sku, 1)
	suite.NoError(err)

	suite.Assert().Equal("BAT", orderItem.Currency)
	suite.Assert().Equal("5c846da1-83cd-4e15-98dd-8e147a56b6fa", orderItem.ID.String())
	suite.Assert().Equal("2", orderItem.Price.String())
	suite.Assert().Equal("coffee coffee coffee", orderItem.Description)
	suite.Assert().Equal("https://brave.com", orderItem.Location)
}
