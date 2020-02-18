// +build integration

package grant

import (
	"github.com/stretchr/testify/suite"
)

type ServiceTestSuite struct {
	suite.Suite
}

func (suite *ServiceTestSuite) TestNewService() {
	_, err := NewService()
	suite.Require().NoError(err)
	_, err = NewService()
	suite.Require().NoError(err)
}
