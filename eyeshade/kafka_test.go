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

type ServiceKafkaSuite struct {
	suite.Suite
	ctx     context.Context
	db      Datastore
	rodb    Datastore
	mock    sqlmock.Sqlmock
	mockRO  sqlmock.Sqlmock
	service *Service
}

func TestServiceKafkaSuite(t *testing.T) {
	suite.Run(t, new(ServiceKafkaSuite))
	// if os.Getenv("EYESHADE_DB_URL") != "" {
	// 	suite.Run(t, new(DatastoreTestSuite))
	// }
}

func (suite *ServiceKafkaSuite) SetupTest() {
	suite.service.Datastore(false).
		RawDB().
		Exec(`delete
from transactions`)
}

func (suite *ServiceKafkaSuite) SetupSuite() {
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

func (suite *ServiceKafkaSuite) TestSettlements() {
	// update when kakfa consumption happens in bulk
	contributions := CreateSettlements(3, "contribution")
	referrals := CreateSettlements(3, "referral")
	settlements := append(contributions, referrals...)
	err := suite.service.ProduceSettlements(suite.ctx, settlements)
	suite.Require().NoError(err)
	errChan := suite.service.Consume()
	for t := 0; t < len(settlements); t += 1 {
		settlement := settlements[t]
		txsChan := suite.CheckTxs(errChan, settlement.Hash)
		select {
		case err := <-errChan:
			suite.Require().NoError(err)
		case actual := <-txsChan:
			hashMap := map[string]models.Transaction{}
			for j := 0; j < len(actual); j += 1 {
				hashMap[actual[j].DocumentID] = actual[j]
			}
			expect := []models.Transaction{}
			target := hashMap[settlement.Hash]
			settlement.ExecutedAt = target.CreatedAt.Format(time.RFC3339)
			if settlement.Type == "contribution" {
				contributionTxs := settlement.ToContributionTransactions()
				expect = append(
					expect,
					contributionTxs[0],
					contributionTxs[1],
					settlement.ToSettlementTransaction(),
				)
			} else {
				expect = append(
					expect,
					settlement.ToSettlementTransaction(),
				)
			}
			suite.Require().JSONEq(
				MustMarshal(suite.Require(), expect),
				MustMarshal(suite.Require(), actual),
			)
		}
	}
}

func CreateSettlements(count int, txType string) []models.Settlement {
	settlements := []models.Settlement{}
	for i := 0; i < count; i += 1 {
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
			Type:         txType,
			SettlementID: uuid.NewV4().String(),
			DocumentID:   uuid.NewV4().String(),
			Address:      uuid.NewV4().String(),
		}
		settlements = append(settlements, settlement)
	}
	return settlements
}

func (suite *ServiceKafkaSuite) CheckTxs(errChan chan error, ids ...string) <-chan []models.Transaction {
	ch := make(chan []models.Transaction)
	checkTxs := func() {
		for {
			<-time.After(time.Second)
			txs, err := suite.service.Datastore(true).
				GetTransactionsByDocumentID(suite.ctx, ids...)
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
