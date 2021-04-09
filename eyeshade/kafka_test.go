// +build integration

package eyeshade

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/brave-intl/bat-go/eyeshade/avro"
	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/suite"
)

type ServiceKafkaMockTestSuite struct {
	suite.Suite
	ctx     context.Context
	db      Datastore
	rodb    Datastore
	mock    sqlmock.Sqlmock
	mockRO  sqlmock.Sqlmock
	service *Service
}

func TestServiceKafkaMockTestSuite(t *testing.T) {
	suite.Run(t, new(ServiceKafkaMockTestSuite))
	// if os.Getenv("EYESHADE_DB_URL") != "" {
	// 	suite.Run(t, new(DatastoreTestSuite))
	// }
}

func (suite *ServiceKafkaMockTestSuite) SetupMockDB() {
	suite.ctx = context.Background()
	name := "sqlmock"
	// setup mock DB we will inject into our pg
	mockDB, mock, err := sqlmock.New()
	suite.Require().NoError(err, "failed to create a sql mock")
	mockRODB, mockRO, err := sqlmock.New()
	suite.Require().NoError(err, "failed to create a sql mock")

	suite.db = NewFromConnection(&grantserver.Postgres{
		DB: sqlx.NewDb(mockDB, name),
	}, name)
	suite.mock = mock
	suite.rodb = NewFromConnection(&grantserver.Postgres{
		DB: sqlx.NewDb(mockRODB, name),
	}, name)
	suite.mockRO = mockRO
}

func (suite *ServiceKafkaMockTestSuite) SetupSuite() {
	suite.SetupMockDB()
	topicHandlers := []avro.TopicHandler{
		avro.NewSettlement(), // add more topics here
	}
	service, err := SetupService(
		WithContext(suite.ctx),
		WithBuildInfo,
		WithConnections(suite.db, suite.rodb),
		WithProducer(topicHandlers...),
		WithConsumer(topicHandlers...),
		// WithTopicAutoCreation,
	)
	suite.Require().NoError(err)
	suite.service = service
}

func (suite *ServiceKafkaMockTestSuite) TestSettlements() {
	settlements := []models.Settlement{{}}
	suite.Require().NoError(suite.service.ProduceSettlements(suite.ctx, settlements))
	errChan := suite.service.Consume()
	select {
	case err := <-errChan:
		suite.Require().NoError(err)
	case txs := <-suite.CheckTxs():
		suite.Require().Len(txs, 3)
		return
	}
}

func (suite *ServiceKafkaMockTestSuite) CheckTxs() <-chan []models.Transaction {
	ch := make(chan []models.Transaction)
	checkTxs := func() {
		for {
			<-time.After(time.Millisecond * 100)
			txs, err := suite.service.Datastore(true).
				GetTransactions(suite.ctx)
			if err == sql.ErrNoRows {
				continue
			}
			ch <- *txs
		}
	}
	go checkTxs()
	return ch
}
