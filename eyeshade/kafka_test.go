// +build integration

package eyeshade

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/brave-intl/bat-go/eyeshade/avro"
	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
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

func (suite *ServiceKafkaMockTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	topicHandlers := []avro.TopicHandler{
		avro.NewSettlement(), // add more topics here
	}
	service, err := SetupService(
		WithContext(suite.ctx),
		WithBuildInfo,
		WithNewDBs,
		WithProducer(topicHandlers...),
		WithConsumer(topicHandlers...),
		WithTopicAutoCreation,
	)
	suite.Require().NoError(err)
	suite.service = service
}

func (suite *ServiceKafkaMockTestSuite) TestSettlements() {
	bat := decimal.NewFromFloat(5)
	fees := bat.Mul(decimal.NewFromFloat(0.05))
	batSubFees := bat.Sub(fees)
	settlements := []models.Settlement{{
		AltCurrency:  altcurrency.BAT,
		Probi:        altcurrency.BAT.ToProbi(batSubFees),
		Fees:         altcurrency.BAT.ToProbi(fees),
		Fee:          decimal.Zero,
		Commission:   decimal.Zero,
		Amount:       bat,
		Currency:     altcurrency.BAT.String(),
		Owner:        fmt.Sprintf("publishers#uuid:%s", uuid.NewV4().String()),
		Channel:      models.Channel("brave.com"),
		Hash:         uuid.NewV4().String(),
		Type:         "contribution",
		SettlementID: uuid.NewV4().String(),
		DocumentID:   uuid.NewV4().String(),
		Address:      uuid.NewV4().String(),
	}}
	err := suite.service.ProduceSettlements(suite.ctx, settlements)
	suite.Require().NoError(err)
	errChan := suite.service.Consume()
	txsChan := suite.CheckTxs(errChan)
	select {
	case err := <-errChan:
		suite.Require().NoError(err)
	case txs := <-txsChan:
		suite.Require().Len(txs, 3)
		return
	}
}

func (suite *ServiceKafkaMockTestSuite) CheckTxs(errChan chan error) <-chan []models.Transaction {
	ch := make(chan []models.Transaction)
	checkTxs := func() {
		for {
			<-time.After(time.Millisecond * 100)
			txs, err := suite.service.Datastore(true).
				GetTransactions(suite.ctx)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				errChan <- err
				return
			}
			if len(*txs) == 0 {
				continue
			}
			ch <- *txs
		}
	}
	go checkTxs()
	return ch
}
