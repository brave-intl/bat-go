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
	sku := "MDAxY2xvY2F0aW9uIGxvY2FsaG9zdDo4MDgwCjAwMWVpZGVudGlmaWVyIEJyYXZlIFNLVSB2MS4wCjAwMWFjaWQgc2t1ID0gQlJBVkUtMTIzNDUKMDAxMmNpZCBwcmljZSA9IDgKMDAxN2NpZCBjdXJyZW5jeSA9IEJBVAowMDJhY2lkIGRlc2NyaXB0aW9uID0gMTIgb3VuY2VzIG9mIENvZmZlZQowMDFjY2lkIGV4cGlyeSA9IDE1ODU2MDczNTkKMDAyZnNpZ25hdHVyZSB60s2IxrUuE0SYqFM3mD2p85nogryrOkkaNUkrHgjEPQo"

	orderItem, err := CreateOrderItemFromMacaroon(sku, 1)
	suite.NoError(err)

	suite.Assert().Equal("BAT", orderItem.Currency)
	suite.Assert().Equal("BRAVE-12345", orderItem.SKU)
	suite.Assert().Equal("8", orderItem.Price.String())
	suite.Assert().Equal("12 ounces of Coffee", orderItem.Description.String)
	suite.Assert().Equal("localhost:8080", orderItem.Location.String)
}
