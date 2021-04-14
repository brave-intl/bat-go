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

type ServiceKafkaTestSuite struct {
	suite.Suite
	ctx     context.Context
	db      Datastore
	rodb    Datastore
	mock    sqlmock.Sqlmock
	mockRO  sqlmock.Sqlmock
	service *Service
}

func TestServiceKafkaTestSuite(t *testing.T) {
	suite.Run(t, new(ServiceKafkaTestSuite))
	// if os.Getenv("EYESHADE_DB_URL") != "" {
	// 	suite.Run(t, new(DatastoreTestSuite))
	// }
}

func (suite *ServiceKafkaTestSuite) SetupTest() {
	suite.service.Datastore(false).
		RawDB().
		Exec(`delete
from transactions`)
}

func (suite *ServiceKafkaTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	topics := []string{
		avro.TopicKeys.Settlement,
		avro.TopicKeys.Contribution,
		avro.TopicKeys.Referral,
		avro.TopicKeys.Suggestion,
	}
	service, err := SetupService(
		WithContext(suite.ctx),
		WithBuildInfo,
		WithNewDBs,
		WithProducer(topics...),
		WithConsumer(topics...),
		WithTopicAutoCreation,
	)
	suite.Require().NoError(err)
	suite.service = service
}

func (suite *ServiceKafkaTestSuite) TestSettlements() {
	bat := decimal.NewFromFloat(5)
	fees := bat.Mul(decimal.NewFromFloat(0.05))
	batSubFees := bat.Sub(fees)
	settlement := models.Settlement{
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
	}
	settlements := []models.Settlement{settlement}
	err := suite.service.ProduceSettlements(suite.ctx, settlements)
	suite.Require().NoError(err)
	errChan := suite.service.Consume()
	txsChan := suite.CheckTxs(errChan)
	select {
	case err := <-errChan:
		suite.Require().NoError(err)
	case actual := <-txsChan:
		settlement.ExecutedAt = actual[2].CreatedAt.Format(time.RFC3339)
		contributionTxs := settlement.ToContributionTransactions()
		expect := []models.Transaction{
			contributionTxs[0],
			contributionTxs[1],
			settlement.ToSettlementTransaction(),
		}
		suite.Require().JSONEq(
			MustMarshal(suite.Require(), expect),
			MustMarshal(suite.Require(), actual),
		)
		return
	}
}

func (suite *ServiceKafkaTestSuite) CheckTxs(errChan chan error) <-chan []models.Transaction {
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
