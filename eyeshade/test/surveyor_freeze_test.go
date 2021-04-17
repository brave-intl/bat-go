// +build integration

package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	eyeshade "github.com/brave-intl/bat-go/eyeshade/service"
	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/brave-intl/bat-go/eyeshade/must"
	timeutils "github.com/brave-intl/bat-go/utils/time"
	"github.com/lib/pq"
	"github.com/stretchr/testify/suite"
)

type SurveyorFreezeSuite struct {
	suite.Suite
	ctx     context.Context
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
	contributions := must.CreateContributions(5)
	votes := models.ContributionsToVotes(contributions...)
	suite.Require().NoError(
		suite.service.Datastore().InsertVotes(suite.ctx, votes),
	)
	date := timeutils.JustDate(time.Now().UTC())
	_, ids := models.CollectSurveyorIDs(
		date,
		votes,
	)
	// have to do this for the virual surveyors
	statement := `
UPDATE surveyor_groups
SET created_at = current_date - INTERVAL '1d'
WHERE id = ANY($1::TEXT[])`
	_, err := suite.service.Datastore().RawDB().ExecContext(
		suite.ctx,
		statement,
		pq.Array(ids),
	)
	suite.Require().NoError(err)
	suite.Require().NoError(
		suite.service.FreezeSurveyors(),
	)
	ballotIDs := models.CollectBallotIDs(date, votes...)
	ballots, err := suite.service.Datastore().GetBallotsByID(suite.ctx, ballotIDs...)
	suite.Require().NoError(err)
	convertables := models.BallotsToConvertableTransactions(*ballots...)
	convertableIDs := models.CollectTransactionIDs(convertables...)
	txs, err := suite.service.Datastore().GetTransactionsByID(suite.ctx, convertableIDs...)
	suite.Require().NoError(err)
	suite.Require().Len(*txs, len(ballotIDs))
}
