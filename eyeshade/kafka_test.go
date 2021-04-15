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
	"github.com/brave-intl/bat-go/eyeshade/countries"
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
	suite.Require().NoError(suite.service.Datastore(false).
		SeedDB(suite.ctx))
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
		WithNewClients,
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
		ch := make(chan []models.Transaction)
		checkErrChan := WaitFor(func() (bool, error) {
			txs, err := suite.service.Datastore(true).
				GetTransactionsByDocumentID(suite.ctx, settlement.Hash)
			if len(*txs) > 0 {
				ch <- *txs
				return true, nil
			}
			return false, err
		})
		select {
		case err := <-errChan:
			suite.Require().NoError(err)
		case err := <-checkErrChan:
			suite.Require().NoError(err)
		case actual := <-ch:
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

func (suite *ServiceKafkaSuite) TestReferrals() {
	// update when kakfa consumption happens in bulk
	referrals := CreateReferrals(3, countries.OriginalRateID)
	suite.Require().NoError(
		suite.service.ProduceReferrals(suite.ctx, referrals),
	)
	errChan := suite.service.Consume()
	for t := 0; t < len(referrals); t += 1 {
		referral := referrals[t]
		ch := make(chan []models.Transaction)
		checkErrChan := WaitFor(func() (bool, error) {
			txs, err := suite.service.Datastore(true).
				GetTransactionsByDocumentID(suite.ctx, referral.GetTransactionID())
			if len(*txs) > 0 {
				ch <- *txs
				return true, nil
			}
			return false, err
		})

		select {
		case err := <-errChan:
			suite.Require().NoError(err)
		case err := <-checkErrChan:
			suite.Require().NoError(err)
		case actual := <-ch:
			expect := referral.ToTxs()
			actualIDMap := map[string]models.Transaction{}
			for _, tx := range actual {
				actualIDMap[tx.ID] = tx
			}
			for i := range expect {
				expect[i].CreatedAt = actualIDMap[expect[i].ID].CreatedAt
				expect[i].Amount = actualIDMap[expect[i].ID].Amount
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
		settlements = append(settlements, models.Settlement{
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
		})
	}
	return settlements
}

func CreateReferrals(count int, countryGroupID uuid.UUID) []models.Referral {
	referrals := []models.Referral{}
	for i := 0; i < count; i += 1 {
		now := time.Now()
		referrals = append(referrals, models.Referral{
			TransactionID:      uuid.NewV4().String(),
			Channel:            models.Channel("brave.com"),
			Owner:              fmt.Sprintf("publishers#uuid:%s", uuid.NewV4().String()),
			FinalizedTimestamp: now,
			ReferralCode:       "ABC123",
			DownloadID:         uuid.NewV4().String(),
			DownloadTimestamp:  now.AddDate(0, 0, -30),
			CountryGroupID:     countryGroupID.String(),
			Platform:           "osx",
		})
	}
	return referrals
}

func WaitFor(
	handler func() (bool, error),
) chan error {
	errChan := make(chan error)
	run := func() {
		for {
			<-time.After(time.Millisecond * 100)
			finished, err := handler()
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				errChan <- err
				return
			}
			if !finished {
				continue
			}
			return
		}
	}
	go run()
	return errChan
}
