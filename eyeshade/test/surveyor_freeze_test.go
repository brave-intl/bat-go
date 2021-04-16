// +build integration

package test

import (
	"context"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/brave-intl/bat-go/eyeshade"
	"github.com/brave-intl/bat-go/eyeshade/datastore"
	"github.com/stretchr/testify/suite"
)

type SurveyorFreezeSuite struct {
	suite.Suite
	ctx     context.Context
	db      datastore.Datastore
	rodb    datastore.Datastore
	mock    sqlmock.Sqlmock
	mockRO  sqlmock.Sqlmock
	service *eyeshade.Service
}

func TestSurveyorFreezeSuite(t *testing.T) {
	suite.Run(t, new(SurveyorFreezeSuite))
}

func (suite *SurveyorFreezeSuite) SetupTest() {
	tables := []string{"transactions", "votes", "surveyor_groups"}
	for _, table := range tables {
		statement := fmt.Sprintf(`delete
from %s`, table)
		_, err := suite.service.Datastore().
			RawDB().
			ExecContext(suite.ctx, statement)
		if err != nil {
			fmt.Println(err)
		}
	}
	suite.Require().NoError(suite.service.Datastore().
		SeedDB(suite.ctx))
}

func (suite *SurveyorFreezeSuite) SetupSuite() {
	suite.ctx = context.Background()
	service, err := eyeshade.SetupService(
		eyeshade.WithContext(suite.ctx),
		eyeshade.WithBuildInfo,
		eyeshade.WithNewDBs,
		eyeshade.WithNewClients,
	)
	suite.Require().NoError(err)
	suite.service = service
}

func (suite *SurveyorFreezeSuite) TestSurveyorFreeze() {
	// contributions := must.CreateContributions(5)
}
