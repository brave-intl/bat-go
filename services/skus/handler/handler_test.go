package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/libs/datastore"
	"github.com/brave-intl/bat-go/libs/handlers"

	"github.com/brave-intl/bat-go/services/skus/handler"
	"github.com/brave-intl/bat-go/services/skus/model"
)

func TestMain(m *testing.M) {
	zerolog.SetGlobalLevel(zerolog.Disabled)
	os.Exit(m.Run())
}

type mockOrderService struct {
	fnCreateOrderFromRequest func(ctx context.Context, req handler.CreateOrderRequest) (*model.Order, error)
}

func (s *mockOrderService) CreateOrderFromRequest(ctx context.Context, req handler.CreateOrderRequest) (*model.Order, error) {
	if s.fnCreateOrderFromRequest == nil {
		return &model.Order{Items: []model.OrderItem{{}}}, nil
	}

	return s.fnCreateOrderFromRequest(ctx, req)
}

func TestOrder_Create(t *testing.T) {
	type tcGiven struct {
		svc  *mockOrderService
		body string
	}

	type tcExpected struct {
		err    *handlers.AppError
		result *model.Order
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "invalid_data",
			given: tcGiven{
				svc: &mockOrderService{},
				body: `{
					"email": "you@example.com",
					"items": []
				}`,
			},
			exp: tcExpected{
				err: handlers.ValidationError(
					"Error validating request body",
					map[string]interface{}{
						"items": "array must contain at least one item",
					},
				),
			},
		},

		{
			name: "invalid_sku",
			given: tcGiven{
				svc: &mockOrderService{
					fnCreateOrderFromRequest: func(ctx context.Context, req handler.CreateOrderRequest) (*model.Order, error) {
						return nil, model.ErrInvalidSKU
					},
				},
				body: `{
					"email": "you@example.com",
					"items": [
						{
							"sku": "invalid_sku",
							"quantity": 1
						}
					]
				}`,
			},
			exp: tcExpected{
				err: handlers.ValidationError(model.ErrInvalidSKU.Error(), nil),
			},
		},

		{
			name: "some_error",
			given: tcGiven{
				svc: &mockOrderService{
					fnCreateOrderFromRequest: func(ctx context.Context, req handler.CreateOrderRequest) (*model.Order, error) {
						return nil, model.Error("some_error")
					},
				},
				body: `{
					"email": "you@example.com",
					"items": [
						{
							"sku": "invalid_sku",
							"quantity": 1
						}
					]
				}`,
			},
			exp: tcExpected{
				err: handlers.WrapError(
					model.Error("some_error"),
					"Error creating the order in the database",
					http.StatusInternalServerError,
				),
			},
		},

		{
			name: "success",
			given: tcGiven{
				svc: &mockOrderService{
					fnCreateOrderFromRequest: func(ctx context.Context, req handler.CreateOrderRequest) (*model.Order, error) {
						result := &model.Order{
							Location: datastore.NullString{
								NullString: sql.NullString{
									Valid:  true,
									String: "somewhere",
								},
							},
							Items: []model.OrderItem{
								{
									SKU:      "some_sku",
									Quantity: 1,
									Price:    mustDecimalFromString("2"),
									Subtotal: mustDecimalFromString("2"),
									Location: datastore.NullString{
										NullString: sql.NullString{
											Valid:  true,
											String: "somewhere",
										},
									},
									Description: datastore.NullString{
										NullString: sql.NullString{
											Valid:  true,
											String: "something",
										},
									},
								},
							},
							TotalPrice: mustDecimalFromString("2"),
						}

						return result, nil
					},
				},
				body: `{
					"email": "you@example.com",
					"items": [
						{
							"sku": "some_sku",
							"quantity": 1
						}
					]
				}`,
			},
			exp: tcExpected{
				result: &model.Order{
					Location: datastore.NullString{
						NullString: sql.NullString{
							Valid:  true,
							String: "somewhere",
						},
					},
					Items: []model.OrderItem{
						{
							SKU:      "some_sku",
							Quantity: 1,
							Price:    mustDecimalFromString("2"),
							Subtotal: mustDecimalFromString("2"),
							Location: datastore.NullString{
								NullString: sql.NullString{
									Valid:  true,
									String: "somewhere",
								},
							},
							Description: datastore.NullString{
								NullString: sql.NullString{
									Valid:  true,
									String: "something",
								},
							},
						},
					},
					TotalPrice: mustDecimalFromString("2"),
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			h := handler.NewOrder(tc.given.svc)

			body := bytes.NewBufferString(tc.given.body)

			req := httptest.NewRequest(http.MethodPost, "http://localhost", body)

			rw := httptest.NewRecorder()
			rw.Header().Set("content-type", "application/json")

			act1 := h.Create(rw, req)
			must.Equal(t, tc.exp.err, act1)

			if tc.exp.err != nil {
				act1.ServeHTTP(rw, req)
				resp := rw.Body.Bytes()

				act2 := &handlers.AppError{}
				err := json.Unmarshal(resp, act2)
				must.Equal(t, nil, err)

				// Cause is excluded from JSON.
				tc.exp.err.Cause = nil

				should.Equal(t, tc.exp.err, act2)

				return
			}

			resp := rw.Body.Bytes()
			act2 := &model.Order{}

			err := json.Unmarshal(resp, act2)
			must.Equal(t, nil, err)

			should.Equal(t, tc.exp.result, act2)
		})
	}
}

func mustDecimalFromString(v string) decimal.Decimal {
	result, err := decimal.NewFromString(v)
	if err != nil {
		panic(err)
	}

	return result
}
