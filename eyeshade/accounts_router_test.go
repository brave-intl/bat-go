// +build integration

package eyeshade

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	uuid "github.com/satori/go.uuid"
)

func (suite *ControllersTestSuite) TestStaticRouter() {
	_, body := suite.DoRequest("GET", "/", nil)
	suite.Require().Equal("ack.", string(body))
}

func (suite *ControllersTestSuite) TestDefunctRouter() {
	re := regexp.MustCompile(`\{.+\}`)
	for _, route := range defunctRoutes {
		path := re.ReplaceAllString(route.Path, uuid.NewV4().String())
		_, body := suite.DoRequest(route.Method, path, nil)
		var defunctResponse DefunctResponse
		err := json.Unmarshal(body, &defunctResponse)
		suite.Require().NoError(err)
		suite.Require().Equal(DefunctResponse{
			StatusCode: http.StatusGone,
			Message:    "Gone",
			Error:      "Gone",
		}, defunctResponse)
	}
}

func (suite *ControllersTestSuite) TestGetAccountEarnings() {
	options := AccountEarningsOptions{
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
	)
	suite.Require().Equal(http.StatusOK, res.StatusCode)
	marshalled, err := json.Marshal(expecting)
	suite.Require().NoError(err)
	suite.Require().JSONEq(string(marshalled), string(body))
}

func (suite *ControllersTestSuite) TestGetAccountSettlementEarnings() {
	options := AccountSettlementEarningsOptions{
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
	)
	suite.Require().Equal(http.StatusOK, res.StatusCode)
	marshalled, err := json.Marshal(expecting)
	suite.Require().NoError(err)
	suite.Require().JSONEq(string(marshalled), string(body))
	var unmarshalledBody []AccountSettlementEarnings
	err = json.Unmarshal(body, &unmarshalledBody)
	suite.Require().Len(unmarshalledBody, options.Limit)

	now := time.Now()
	startDate := now.Truncate(time.Second)
	untilDate := startDate.Add(time.Hour * 24 * 2)
	options = AccountSettlementEarningsOptions{
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
	)
	suite.Require().Equal(http.StatusOK, res.StatusCode)
	marshalled, err = json.Marshal(expecting)
	suite.Require().NoError(err)
	suite.Require().JSONEq(string(marshalled), string(body))
}

func (suite *ControllersTestSuite) TestGetBalances() {
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
	)
	suite.Require().Equal(http.StatusOK, res.StatusCode, string(body))
	accountsMarshalled, err := json.Marshal(accounts)
	suite.Require().NoError(err)
	suite.Require().JSONEq(string(accountsMarshalled), string(body))
	var unmarshalledBody []AccountSettlementEarnings
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
	)
	suite.Require().Equal(http.StatusOK, res.StatusCode, string(body))
	accountsMarshalled, err = json.Marshal(accounts)
	suite.Require().NoError(err)
	suite.Require().JSONEq(string(accountsMarshalled), string(body))
	unmarshalledBody = []AccountSettlementEarnings{}
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
