// +build integration

package eyeshade

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/maikelmclauflin/go-boom"
	uuid "github.com/satori/go.uuid"
)

func (suite *ControllersTestSuite) TestRouterStatic() {
	_, body := suite.DoRequest("GET", "/", nil, "")
	suite.Require().Equal(".", string(body))
}

func (suite *ControllersTestSuite) TestRouterDefunct() {
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

func (suite *ControllersTestSuite) TestGETAccountEarnings() {
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

func (suite *ControllersTestSuite) TestGETAccountSettlementEarnings() {
	options := models.AccountSettlementEarningsOptions{
		Ascending: true,
		Type:      "contributions",
		Limit:     5,
	}
	expecting := SetupMockGetAccountSettlementEarnings(
		suite.mockRO,
		options,
	)
	path := fmt.Sprintf("/v1/accounts/settlements/contributions/total?limit=%d", options.Limit)
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
	var unmarshalledBody []models.AccountSettlementEarnings
	err = json.Unmarshal(body, &unmarshalledBody)
	suite.Require().Len(unmarshalledBody, options.Limit)

	now := time.Now()
	startDate := now.Truncate(time.Second)
	untilDate := startDate.Add(time.Hour * 24 * 2)
	options = models.AccountSettlementEarningsOptions{
		Ascending: true,
		Type:      "contributions",
		Limit:     5,
		StartDate: &startDate,
		UntilDate: &untilDate,
	}

	expecting = SetupMockGetAccountSettlementEarnings(
		suite.mockRO,
		options,
	)
	path = fmt.Sprintf(
		"/v1/accounts/settlements/contributions/total?limit=%d&start=%s&until=%s",
		options.Limit,
		options.StartDate.Format(time.RFC3339),
		options.UntilDate.Format(time.RFC3339),
	)
	res, body = suite.DoRequest(
		"GET",
		path,
		nil,
		suite.tokens["publishers"],
	)
	suite.Require().Equal(http.StatusOK, res.StatusCode)
	marshalled, err = json.Marshal(expecting)
	suite.Require().NoError(err)
	suite.Require().JSONEq(string(marshalled), string(body))
}

func (suite *ControllersTestSuite) TestGETBalances() {
	accountIDs := []string{uuid.NewV4().String()}
	accounts := SetupMockGetBalances(
		suite.mockRO,
		accountIDs,
	)
	param := "account="
	path := fmt.Sprintf("/v1/accounts/balances?%s%s", param, strings.Join(accountIDs, "&"+param))
	res, body := suite.DoRequest(
		"GET",
		path,
		nil,
		suite.tokens["publishers"],
	)
	suite.Require().Equal(http.StatusOK, res.StatusCode, string(body))
	accountsMarshalled, err := json.Marshal(accounts)
	suite.Require().NoError(err)
	suite.Require().JSONEq(string(accountsMarshalled), string(body))
	var unmarshalledBody []models.AccountSettlementEarnings
	err = json.Unmarshal(body, &unmarshalledBody)
	suite.Require().Len(unmarshalledBody, len(accountIDs))

	accountIDs = []string{uuid.NewV4().String()}
	accounts = SetupMockGetBalances(
		suite.mockRO,
		accountIDs,
	)
	param = "account="
	path = fmt.Sprintf("/v1/accounts/balances?%s%s", param, strings.Join(accountIDs, "&"+param))
	res, body = suite.DoRequest(
		"GET",
		path,
		nil,
		suite.tokens["publishers"],
	)
	suite.Require().Equal(http.StatusOK, res.StatusCode, string(body))
	accountsMarshalled, err = json.Marshal(accounts)
	suite.Require().NoError(err)
	suite.Require().JSONEq(string(accountsMarshalled), string(body))
	unmarshalledBody = []models.AccountSettlementEarnings{}
	err = json.Unmarshal(body, &unmarshalledBody)
	suite.Require().Len(unmarshalledBody, len(accountIDs))
	// now := time.Now()
	// startDate := now.Truncate(time.Second)
	// untilDate := startDate.Add(time.Hour * 24 * 2)
	// options = AccountSettlementEarningsOptions{
	// 	Ascending: true,
	// 	Type:      "contributions",
	// 	Limit:     5,
	// 	StartDate: &startDate,
	// 	UntilDate: &untilDate,
	// }

	// expecting = SetupMockGetAccountSettlementEarnings(
	// 	suite.mockRO,
	// 	options,
	// )
	// path = fmt.Sprintf(
	// 	"/v1/accounts/settlements/contributions/total?limit=%d&start=%s&until=%s",
	// 	options.Limit,
	// 	options.StartDate.Format(time.RFC3339),
	// 	options.UntilDate.Format(time.RFC3339),
	// )
	// res, body = suite.DoRequest(
	// 	"GET",
	// 	path,
	// 	nil,
	// )
	// suite.Require().Equal(http.StatusOK, res.StatusCode)
	// marshalled, err = json.Marshal(expecting)
	// suite.Require().NoError(err)
	// suite.Require().JSONEq(string(marshalled), string(body))
}

func (suite *ControllersTestSuite) TestGETTransactionsByAccount() {
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
			actual := suite.DoGetTransactionsByAccount(
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

func (suite *ControllersTestSuite) DoGetTransactionsByAccount(
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
