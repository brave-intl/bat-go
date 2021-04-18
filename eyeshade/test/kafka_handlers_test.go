// +build integration

package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/brave-intl/bat-go/eyeshade/avro"
	"github.com/brave-intl/bat-go/eyeshade/datastore"
	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/brave-intl/bat-go/eyeshade/must"
	eyeshade "github.com/brave-intl/bat-go/eyeshade/service"
	timeutils "github.com/brave-intl/bat-go/utils/time"
	"github.com/stretchr/testify/suite"
)

type KafkaHandlersSuite struct {
	suite.Suite
	ctx     context.Context
	db      datastore.Datastore
	rodb    datastore.Datastore
	mock    sqlmock.Sqlmock
	mockRO  sqlmock.Sqlmock
	service *eyeshade.Service
}

func TestKafkaHandlersSuite(t *testing.T) {
	suite.Run(t, new(KafkaHandlersSuite))
}

func (suite *KafkaHandlersSuite) SetupTest() {
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

func (suite *KafkaHandlersSuite) SetupSuite() {
	suite.ctx = context.Background()
	topics := []string{
		avro.TopicKeys.Settlement,
		avro.TopicKeys.Contribution,
		avro.TopicKeys.Referral,
		avro.TopicKeys.Suggestion,
	}
	service, err := eyeshade.SetupService(
		eyeshade.WithContext(suite.ctx),
		eyeshade.WithBuildInfo,
		eyeshade.WithNewDBs,
		eyeshade.WithNewClients,
		eyeshade.WithProducer(topics...),
		eyeshade.WithConsumer(topics...),
		eyeshade.WithTopicAutoCreation,
	)
	suite.Require().NoError(err)
	suite.service = service
}

func (suite *KafkaHandlersSuite) TestSettlements() {
	// update when kakfa consumption happens in bulk
	contributions := must.CreateSettlements(3, models.TransactionTypes.Contribution)
	referrals := must.CreateSettlements(3, models.TransactionTypes.Referral)
	settlements := append(contributions, referrals...)

	err := suite.service.ProduceSettlements(suite.ctx, settlements)
	suite.Require().NoError(err)

	errChan := suite.service.Consume()
	ch := make(chan []models.Transaction)
	txs := models.SettlementsToConvertableTransactions(settlements...)
	ids := models.CollectTransactionIDs(txs...)

	go must.WaitFor(errChan, func(
		errChan chan error,
		last bool,
	) (bool, error) {
		txs, err := suite.service.Datastore(true).
			GetTransactionsByID(suite.service.Context(), ids...)
		if last {
			// we are in async land
			if err != nil {
				errChan <- err
			}
			ch <- *txs
			return false, err
		}
		if err == nil {
			last = len(*txs) == len(ids) && suite.service.Consumer(avro.TopicKeys.Settlement).IdleFor(time.Second)
		}
		return last, err
	})

	select {
	case err := <-errChan:
		suite.Require().NoError(err)
	case actual := <-ch:
		settlements := models.SettlementBackfillFromTransactions(settlements, actual)
		txs := models.SettlementsToConvertableTransactions(settlements...)
		expect := models.CollectTransactions(txs...)
		suite.Require().JSONEq(
			must.Marshal(suite.Require(), expect),
			must.Marshal(suite.Require(), actual),
		)
	}
}

func (suite *KafkaHandlersSuite) TestReferrals() {
	// update when kakfa consumption happens in bulk
	referrals := must.CreateReferrals(3, models.OriginalRateID)
	suite.Require().NoError(
		suite.service.ProduceReferrals(suite.ctx, referrals),
	)
	errChan := suite.service.Consume()
	ch := make(chan []models.Transaction)
	convertableTransactions := models.ReferralsToConvertableTransactions(referrals...)
	ids := models.CollectTransactionIDs(convertableTransactions...)
	go must.WaitFor(errChan, func(
		errChan chan error,
		last bool,
	) (bool, error) {
		txs, err := suite.service.Datastore(true).
			GetTransactionsByID(suite.service.Context(), ids...)
		if last {
			// we are in async land
			if err != nil {
				errChan <- err
			}
			ch <- *txs
			return false, err
		}
		if err == nil {
			last = len(*txs) == len(ids) && suite.service.Consumer(avro.TopicKeys.Referral).IdleFor(time.Second)
		}
		return last, err
	})

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
			must.Marshal(suite.Require(), expect),
			must.Marshal(suite.Require(), actual),
		)
	}
}

func (suite *KafkaHandlersSuite) TestSuggestions() {
	// update when kakfa consumption happens in bulk
	suggestions := must.CreateSuggestions(4)
	suite.Require().NoError(
		suite.service.ProduceSuggestions(suite.ctx, suggestions),
	)
	date := timeutils.JustDate(time.Now().UTC())
	errChan := suite.service.Consume()
	votes := models.SuggestionsToVotes(suggestions...)
	voteIDs := models.CollectBallotIDs(date, votes...)
	ch := make(chan []models.Ballot)
	go must.WaitFor(errChan, func(
		errChan chan error,
		last bool,
	) (bool, error) {
		ballots, err := suite.service.Datastore(true).
			GetBallotsByID(suite.service.Context(), voteIDs...)
		if last {
			// we are in async land
			if err != nil {
				errChan <- err
			}
			ch <- *ballots
			return false, err
		}
		if err == nil {
			last = len(*ballots) == len(voteIDs) && suite.service.Consumer(avro.TopicKeys.Suggestion).IdleFor(time.Second)
		}
		return last, err
	})
	select {
	case err := <-errChan:
		suite.Require().NoError(err)
	case actual := <-ch:
		_, ballots := models.CollectBallots(
			date,
			votes,
			map[string]bool{},
		)
		// if this works for both 1 and n, then the
		// tally query is working correctly as well
		expect := models.CondenseBallots(&ballots, true)
		suite.Require().JSONEq(
			must.Marshal(suite.Require(), expect),
			must.Marshal(suite.Require(), actual),
		)
	}
}

func (suite *KafkaHandlersSuite) TestContributions() {
	// update when kakfa consumption happens in bulk
	contributions := must.CreateContributions(4)
	suite.Require().NoError(
		suite.service.ProduceContributions(suite.ctx, contributions),
	)
	date := timeutils.JustDate(time.Now().UTC())
	errChan := suite.service.Consume()
	votes := models.ContributionsToVotes(contributions...)
	voteIDs := models.CollectBallotIDs(date, votes...)
	ch := make(chan []models.Ballot)
	go must.WaitFor(errChan, func(
		errChan chan error,
		last bool,
	) (bool, error) {
		ballots, err := suite.service.Datastore(true).
			GetBallotsByID(suite.service.Context(), voteIDs...)
		if last {
			// we are in async land
			if err != nil {
				errChan <- err
			}
			ch <- *ballots
			return false, err
		}
		if err == nil {
			last = len(*ballots) == len(voteIDs) && suite.service.Consumer(avro.TopicKeys.Contribution).IdleFor(time.Second)
		}
		return last, err
	})
	select {
	case err := <-errChan:
		suite.Require().NoError(err)
	case actual := <-ch:
		_, ballots := models.CollectBallots(
			date,
			votes,
			map[string]bool{},
		)
		// if this works for both 1 and n, then the
		// tally query is working correctly as well
		expect := models.CondenseBallots(&ballots, true)
		suite.Require().JSONEq(
			must.Marshal(suite.Require(), expect),
			must.Marshal(suite.Require(), actual),
		)
	}
}
