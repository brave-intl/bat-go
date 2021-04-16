// +build integration

package eyeshade

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/maikelmclauflin/go-boom"
	uuid "github.com/satori/go.uuid"
)

func (suite *ControllersSuite) TestRouterStatic() {
	_, body := suite.DoRequest("GET", "/", nil, "")
	suite.Require().Equal(".", string(body))
}

func (suite *ControllersSuite) TestRouterDefunct() {
	re := regexp.MustCompile(`\{.+\}`)
	for _, route := range defunctRoutes {
		path := re.ReplaceAllString(route.Path, uuid.NewV4().String())
		_, body := suite.DoRequest(route.Method, path, nil, "")
		var defunctResponse boom.Err
		err := json.Unmarshal(body, &defunctResponse)
		suite.Require().NoError(err)
		suite.Require().Equal(boom.Gone(), defunctResponse)
	}
}

func (suite *ControllersSuite) TestGETAccountEarnings() {
	options := models.AccountEarningsOptions{
		Ascending: true,
		Type:      "contributions",
		Limit:     5,
	}
	expecting := SetupMockGetAccountEarnings(
		suite.mockRO,
		options,
	)
	path := fmt.Sprintf("/v1/accounts/earnings/contributions/total?limit=%d", options.Limit)
	res, body := suite.DoRequest(
		"GET",
		path,
		nil,
		suite.tokens["publishers"],
	)
	suite.Require().Equal(http.StatusOK, res.StatusCode)
	marshalled, err := json.Marshal(expecting)
	suite.Require().NoError(err)
	suite.Require().JSONEq(string(marshalled), string(body))
}

func (suite *ControllersSuite) TestGETAccountSettlementEarnings() {
	actual := []models.AccountSettlementEarnings{}
	options := models.AccountSettlementEarningsOptions{
		Ascending: true,
		Type:      "contributions",
		Limit:     5,
	}
	expect := SetupMockGetAccountSettlementEarnings(
		suite.mockRO,
		options,
	)
	suite.DoGETAccountSettlementEarnings(
		options,
		suite.tokens["publishers"],
		&actual,
	)
	suite.Require().JSONEq(
		MustMarshal(suite.Require(), expect),
		MustMarshal(suite.Require(), actual),
	)

	actual = []models.AccountSettlementEarnings{}
	now := time.Now()
	startDate := now.Truncate(time.Second)
	untilDate := startDate.AddDate(0, 0, 2)
	options = models.AccountSettlementEarningsOptions{
		Ascending: true,
		Type:      "contributions",
		Limit:     5,
		StartDate: &startDate,
		UntilDate: &untilDate,
	}

	expect = SetupMockGetAccountSettlementEarnings(
		suite.mockRO,
		options,
	)
	suite.DoGETAccountSettlementEarnings(
		options,
		suite.tokens["publishers"],
		&actual,
	)
	suite.Require().JSONEq(
		MustMarshal(suite.Require(), expect),
		MustMarshal(suite.Require(), actual),
	)
}

func (suite *ControllersSuite) DoGETAccountSettlementEarnings(
	options models.AccountSettlementEarningsOptions,
	token string,
	p interface{},
	status ...int,
) {
	values := url.Values{}
	values.Add("limit", fmt.Sprint(options.Limit))
	if options.StartDate != nil {
		values.Add("start", options.StartDate.Format(time.RFC3339))
		if options.UntilDate != nil {
			values.Add("until", options.UntilDate.Format(time.RFC3339))
		}
	}
	path := fmt.Sprintf(
		"/v1/accounts/settlements/contributions/total?%s",
		values.Encode(),
	)
	res, body := suite.DoRequest(
		"GET",
		path,
		nil,
		token,
	)
	suite.CheckAndUnmarshal(p, body, res.StatusCode, status...)
}

func (suite *ControllersSuite) CheckAndUnmarshal(
	p interface{},
	body []byte,
	resStatusCode int,
	status ...int,
) {
	s := http.StatusOK
	if len(status) > 0 {
		s = status[0]
	}
	suite.Require().Equal(s, resStatusCode, string(body))
	MustUnmarshal(suite.Require(), body, p)
}

func MustUnmarshal(
	assertions *require.Assertions,
	bytes []byte,
	structure interface{},
) {
	assertions.NoError(json.Unmarshal(bytes, structure))
}

func (suite *ControllersSuite) DoGETAccountBalances(
	accountIDs []string,
	token string,
	p interface{},
	status ...int,
) {
	values := url.Values{}
	for _, accountID := range accountIDs {
		values.Add("account", accountID)
	}
	path := fmt.Sprintf("/v1/accounts/balances?%s", values.Encode())
	res, body := suite.DoRequest(
		"GET",
		path,
		nil,
		token,
	)
	suite.CheckAndUnmarshal(p, body, res.StatusCode, status...)
}

func (suite *ControllersSuite) TestGETBalances() {
	accountIDs := []string{uuid.NewV4().String()}
	expect := SetupMockGetBalances(
		suite.mockRO,
		accountIDs,
	)
	var actual []models.Balance
	suite.DoGETAccountBalances(
		accountIDs,
		suite.tokens["publishers"],
		&actual,
	)
	suite.Require().JSONEq(
		MustMarshal(suite.Require(), expect),
		MustMarshal(suite.Require(), actual),
	)
}

func (suite *ControllersSuite) TestGETTransactionsByAccount() {
	unescapedAccountID := fmt.Sprintf("publishers#uuid:%s", uuid.NewV4().String())
	escapedAccountID := url.PathEscape(unescapedAccountID)
	scenarios := map[string]struct {
		path   string
		mock   bool
		types  []string
		status int
		body   interface{}
		auth   string
	}{
		"200 success": {
			path:   escapedAccountID,
			mock:   true,
			status: http.StatusOK,
			body:   nil,
			auth:   suite.tokens["publishers"],
		},
		"403 if token is not valid": {
			path:   escapedAccountID,
			mock:   false,
			status: http.StatusForbidden,
			body:   boom.Forbidden(),
			auth:   uuid.NewV4().String(),
		},
		"404s if id not escaped": {
			path:   unescapedAccountID,
			mock:   false,
			status: http.StatusNotFound,
			auth:   suite.tokens["publishers"],
		},
		"referrals only": {
			path:   escapedAccountID,
			mock:   true,
			status: http.StatusOK,
			types:  []string{"referral"},
			body:   nil,
			auth:   suite.tokens["publishers"],
		},
		"unknown type": {
			path:   escapedAccountID,
			mock:   true,
			status: http.StatusOK,
			types:  []string{"garble"},
			auth:   suite.tokens["publishers"],
		},
	}
	for description, scenario := range scenarios {
		testName := fmt.Sprintf(
			"GetTransactionsByAccount(%s,%d):%d",
			description,
			len(scenario.types),
			scenario.status,
		)
		suite.T().Run(testName, func(t *testing.T) {
			var expected interface{}
			if scenario.mock {
				expectedTxs := SetupMockGetTransactionsByAccount(
					suite.mockRO,
					unescapedAccountID,
					scenario.types...,
				)
				if scenario.body == nil {
					expected = transformTransactions(unescapedAccountID, &expectedTxs)
				}
			}
			if scenario.body != nil {
				expected = scenario.body
			}
			actual := suite.DoGETTransactionsByAccount(
				scenario.path,
				scenario.status,
				scenario.auth,
				scenario.types...,
			)
			if scenario.body == nil {
				return
			}
			suite.Require().JSONEq(
				MustMarshal(suite.Require(), expected),
				actual,
			)
		})
	}
}

func (suite *ControllersSuite) DoGETTransactionsByAccount(
	accountID string,
	status int,
	auth string,
	types ...string,
) string {
	path := fmt.Sprintf(
		"/v1/accounts/%s/transactions",
		accountID,
	)
	if len(types) > 0 {
		values := url.Values{}
		for _, t := range types {
			values.Add("type", t)
		}
		path += "?" + values.Encode()
	}
	res, body := suite.DoRequest(
		"GET",
		path,
		nil,
		auth,
	)
	suite.Require().Equal(status, res.StatusCode, string(body))
	return string(body)
}
