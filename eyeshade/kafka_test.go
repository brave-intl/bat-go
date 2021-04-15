// +build integration

package eyeshade

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"math/rand"

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
	ch := make(chan []models.Transaction)
	txs := models.SettlementsToConvertableTransactions(settlements...)
	ids := models.CollectTransactionIDs(txs...)

	go WaitFor(errChan, CheckTransactions(
		ch,
		suite.service,
		ids...,
	))

	select {
	case err := <-errChan:
		suite.Require().NoError(err)
	case actual := <-ch:
		settlements := models.SettlementBackfillFromTransactions(settlements, actual)
		txs := models.SettlementsToConvertableTransactions(settlements...)
		expect := models.CollectTransactions(txs...)
		suite.Require().JSONEq(
			MustMarshal(suite.Require(), expect),
			MustMarshal(suite.Require(), actual),
		)
	}
}

func (suite *ServiceKafkaSuite) TestReferrals() {
	// update when kakfa consumption happens in bulk
	referrals := CreateReferrals(3, countries.OriginalRateID)
	suite.Require().NoError(
		suite.service.ProduceReferrals(suite.ctx, referrals),
	)
	errChan := suite.service.Consume()
	ch := make(chan []models.Transaction)
	convertableTransactions := models.ReferralsToConvertableTransactions(referrals...)
	ids := models.CollectTransactionIDs(convertableTransactions...)
	go WaitFor(errChan, CheckTransactions(
		ch,
		suite.service,
		ids...,
	))

	select {
	case err := <-errChan:
		suite.Require().NoError(err)
	case actual := <-ch:
		modified, err := suite.service.ModifyReferrals(&referrals)
		suite.Require().NoError(err)
		referrals := models.ReferralBackfillFromTransactions(*modified, actual)
		txs := models.ReferralsToConvertableTransactions(referrals...)
		expect := models.CollectTransactions(txs...)
		suite.Require().JSONEq(
			MustMarshal(suite.Require(), expect),
			MustMarshal(suite.Require(), actual),
		)
	}
}

func (suite *ServiceKafkaSuite) TestSuggestions() {
	// update when kakfa consumption happens in bulk
	suggestions := CreateSuggestions(4)
	suite.Require().NoError(
		suite.service.ProduceSuggestions(suite.ctx, suggestions),
	)
	date := time.Now().Format("2021-01-01")
	errChan := suite.service.Consume()
	votes := models.SuggestionsToVotes(suggestions...)
	voteIDs := models.CollectBallotIDs(date, votes...)
	ch := make(chan []models.Ballot)
	go WaitFor(errChan, CheckBallots(
		ch,
		suite.service,
		voteIDs...,
	))
	select {
	case err := <-errChan:
		suite.Require().NoError(err)
	case actual := <-ch:
		_, ballots := models.CollectBallots(
			date,
			votes,
			map[string]bool{},
		)
		expect := models.CondenseBallots(&ballots, true)
		suite.Require().JSONEq(
			MustMarshal(suite.Require(), expect),
			MustMarshal(suite.Require(), actual),
		)
	}
}

func CheckTransactions(
	ch chan []models.Transaction,
	service *Service,
	ids ...string,
) func(chan error) (bool, error) {
	return func(errChan chan error) (bool, error) {
		txs, err := service.Datastore(true).
			GetTransactionsByID(service.Context(), ids...)
		if err != nil {
			return false, err
		}
		if len(*txs) == len(ids) {
			<-time.After(time.Second)
			go func() {
				txs, err := service.Datastore(true).
					GetTransactionsByID(service.Context(), ids...)
				if err != nil {
					errChan <- err
				}
				ch <- *txs
			}()
			return true, nil
		}
		return false, nil
	}
}

func CheckBallots(
	ch chan []models.Ballot,
	service *Service,
	ids ...string,
) func(chan error) (bool, error) {
	return func(errChan chan error) (bool, error) {
		ballots, err := service.Datastore(true).
			GetBallotsByID(service.Context(), ids...)
		if err != nil {
			return false, err
		}
		if len(*ballots) == len(ids) {
			<-time.After(time.Second * 2) // let the kafka consumer catch up
			go func() {
				ballots, err := service.Datastore(true).
					GetBallotsByID(service.Context(), ids...)
				if err != nil {
					errChan <- err
				}
				ch <- *ballots
			}()
			return true, nil
		}
		return false, nil
	}
}

func WaitFor(
	errChan chan error,
	handler func(chan error) (bool, error),
) {
	for {
		<-time.After(time.Millisecond * 100)
		finished, err := handler(errChan)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			errChan <- err
			return
		}
		if finished {
			return
		}
	}
}

func CreateSettlements(count int, txType string) []models.Settlement {
	settlements := []models.Settlement{}
	for i := 0; i < count; i++ {
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
	for i := 0; i < count; i++ {
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

func CreateSuggestions(count int) []models.Suggestion {
	suggestions := []models.Suggestion{}
	promotionIDs := []uuid.UUID{}
	promotionLimit := 5
	for i := 0; i < promotionLimit; i++ {
		promotionIDs = append(promotionIDs, uuid.NewV4())
	}
	suggestionTypes := []string{"auto-contribute", "oneoff-tip", "recurring-tip", "payment"}
	fundingTypes := []string{"ugp", "ads"}
	for i := 0; i < count; i++ {
		now := time.Now()
		fundings := []models.Funding{}
		total := decimal.Zero
		rand.Seed(int64(i))
		for _, suggestionType := range suggestionTypes {
			for j := 0; j < 5; j += 1 {
				random := rand.Int()
				promotionIDIndex := random % promotionLimit
				amount := decimal.NewFromFloat(float64(random%10 + 1)).Mul(models.VoteValue)
				total = total.Add(amount)
				for _, fundingType := range fundingTypes {
					fundings = append(fundings, models.Funding{
						Type:      fundingType,
						Amount:    amount,
						Cohort:    "control",
						Promotion: promotionIDs[promotionIDIndex].String(),
					})
				}
			}
			suggestions = append(suggestions, models.Suggestion{
				ID:          uuid.NewV4().String(),
				Type:        suggestionType,
				Channel:     models.Channel("brave.com"),
				CreatedAt:   now,
				TotalAmount: total,
				OrderID:     uuid.NewV4().String(),
				Funding:     fundings,
			})
		}
	}
	return suggestions
}
