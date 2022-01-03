// +build integration

package ratios_test

import (
	"testing"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/ratios"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/suite"
)

type ControllersTestSuite struct {
	service  *ratios.Service
	mockCtrl *gomock.Controller
	suite.Suite
}

func TestControllersTestSuite(t *testing.T) {
	suite.Run(t, new(ControllersTestSuite))
}

func (suite *ControllersTestSuite) SetupSuite() {
	govalidator.SetFieldsRequiredByDefault(true)
}

func (suite *ControllersTestSuite) BeforeTest(sn, tn string) {
	suite.mockCtrl = gomock.NewController(suite.T())
}

func (suite *ControllersTestSuite) AfterTest(sn, tn string) {
	suite.mockCtrl.Finish()
}

func (suite *ControllersTestSuite) TestCreateOrder() {
}
