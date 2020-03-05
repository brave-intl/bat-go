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

	// Expired SKU token
	expiredSku := "MDAxZmxvY2F0aW9uIGh0dHBzOi8vYnJhdmUuY29tCjAwMWFpZGVudGlmaWVyIGlkZW50aWZpZXIKMDAyYWNpZCBleHBpcnkgPSAyMDAwLTAzLTA0VDEyOjAwOjAwKzAwMDAKMDAxN2NpZCBjdXJyZW5jeSA9IEJBVAowMDMyY2lkIGlkID0gNWM4NDZkYTEtODNjZC00ZTE1LTk4ZGQtOGUxNDdhNTZiNmZhCjAwMmJjaWQgZGVzY3JpcHRpb24gPSBjb2ZmZWUgY29mZmVlIGNvZmZlZQowMDE1Y2lkIHByaWNlID0gMi4wMAowMDJmc2lnbmF0dXJlIAM9lcsqANRYl_H5YLJkckvRW3i8_r1Orp0c3Y6OJQJKCg"
	orderItem, err = CreateOrderItemFromMacaroon(expiredSku, 1)
	suite.Assert().Error(err, "parsing time")
	suite.Assert().Nil(orderItem)

	// Invalid secret
	invalidSecret := "MDAxZmxvY2F0aW9uIGh0dHBzOi8vYnJhdmUuY29tCjAwMWFpZGVudGlmaWVyIGlkZW50aWZpZXIKMDAyYmNpZCBleHBpcnkgPSAzMDIwLTAzLTA1VDEzOjIwOjI3LTA2OjAwCjAwMTdjaWQgY3VycmVuY3kgPSBCQVQKMDAzMmNpZCBpZCA9IDVjODQ2ZGExLTgzY2QtNGUxNS05OGRkLThlMTQ3YTU2YjZmYQowMDJiY2lkIGRlc2NyaXB0aW9uID0gY29mZmVlIGNvZmZlZSBjb2ZmZWUKMDAxNWNpZCBwcmljZSA9IDIuMDAKMDAyZnNpZ25hdHVyZSAE4pxcvbRdm5Yq8c0cUgAy9NZs8rdU-ZAJ9VRMsC_7-wo"
	orderItem, err = CreateOrderItemFromMacaroon(invalidSecret, 1)
	suite.Assert().Error(err, "signature mismatch after caveat verification")
	suite.Assert().Nil(orderItem)

}
