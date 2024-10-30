package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/services/rewards"
	"github.com/brave-intl/bat-go/services/rewards/model"
)

func TestService_GetCards(t *testing.T) {
	type card struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		URL         string `json:"url"`
		Thumbnail   string `json:"thumbnail"`
	}

	type cards struct {
		CommunityCard  []card `json:"community-card"`
		MerchStoreCard []card `json:"merch-store-card"`
	}

	type tcGiven struct {
		cardSvc cardService
	}

	type tcExpected struct {
		code  int
		cards cards
		err   *handlers.AppError
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "error_cards_as_bytes",
			given: tcGiven{
				cardSvc: &mockCardService{
					fnGetCards: func(ctx context.Context) (rewards.CardBytes, error) {
						return nil, model.Error("error")
					},
				},
			},
			exp: tcExpected{
				code: http.StatusInternalServerError,
				err: &handlers.AppError{
					Message: errSomethingWentWrong.Error(),
					Code:    http.StatusInternalServerError,
					Cause:   errSomethingWentWrong,
				},
			},
		},

		{
			name: "success",
			given: tcGiven{
				cardSvc: &mockCardService{
					fnGetCards: func(ctx context.Context) (rewards.CardBytes, error) {
						cards := rewards.CardBytes(`{ "community-card": [{"title": "<string>", "description": "<string>", "url": "<string>", "thumbnail": "<string>"}], "merch-store-card": [{"title": "<string>", "description": "<string>", "url": "<string>", "thumbnail": "<string>"}] }`)

						return cards, nil
					},
				},
			},
			exp: tcExpected{
				code: http.StatusOK,
				cards: cards{
					CommunityCard: []card{
						{
							Title:       "<string>",
							Description: "<string>",
							URL:         "<string>",
							Thumbnail:   "<string>",
						},
					},
					MerchStoreCard: []card{
						{
							Title:       "<string>",
							Description: "<string>",
							URL:         "<string>",
							Thumbnail:   "<string>",
						},
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/cards", nil)

			rw := httptest.NewRecorder()

			ch := NewCardsHandler(tc.given.cardSvc)

			actual := ch.GetCardsHandler(rw, r)

			if actual != nil {
				actual.ServeHTTP(rw, r)
				assert.Equal(t, tc.exp.code, rw.Code)
				assert.Equal(t, tc.exp.err.Code, actual.Code)
				assert.Equal(t, tc.exp.err.Cause, actual.Cause)
				assert.Contains(t, actual.Message, tc.exp.err.Message)
				return
			}

			assert.Equal(t, tc.exp.code, rw.Code)

			var body cards
			err := json.Unmarshal(rw.Body.Bytes(), &body)
			require.NoError(t, err)

			assert.Equal(t, tc.exp.cards, body)
		})
	}
}

type mockCardService struct {
	fnGetCards func(ctx context.Context) (rewards.CardBytes, error)
}

func (m *mockCardService) GetCardsAsBytes(ctx context.Context) (rewards.CardBytes, error) {
	if m.fnGetCards == nil {
		return rewards.CardBytes{}, nil
	}

	return m.fnGetCards(ctx)
}
