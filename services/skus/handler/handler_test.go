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

	"github.com/go-chi/chi"
	"github.com/rs/zerolog"
	uuid "github.com/satori/go.uuid"
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
	fnCreateOrderFromRequest func(ctx context.Context, req model.CreateOrderRequest) (*model.Order, error)
	fnCreateOrder            func(ctx context.Context, req *model.CreateOrderRequestNew) (*model.Order, error)
	fnCancelOrder            func(ctx context.Context, id uuid.UUID) error
}

func (s *mockOrderService) CreateOrderFromRequest(ctx context.Context, req model.CreateOrderRequest) (*model.Order, error) {
	if s.fnCreateOrderFromRequest == nil {
		return &model.Order{Items: []model.OrderItem{{}}}, nil
	}

	return s.fnCreateOrderFromRequest(ctx, req)
}

func (s *mockOrderService) CreateOrder(ctx context.Context, req *model.CreateOrderRequestNew) (*model.Order, error) {
	if s.fnCreateOrder == nil {
		return &model.Order{Items: []model.OrderItem{{}}}, nil
	}

	return s.fnCreateOrder(ctx, req)
}

func (s *mockOrderService) CancelOrder(ctx context.Context, id uuid.UUID) error {
	if s.fnCancelOrder == nil {
		return nil
	}

	return s.fnCancelOrder(ctx, id)
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
					"request body",
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
					fnCreateOrderFromRequest: func(ctx context.Context, req model.CreateOrderRequest) (*model.Order, error) {
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
					fnCreateOrderFromRequest: func(ctx context.Context, req model.CreateOrderRequest) (*model.Order, error) {
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
					fnCreateOrderFromRequest: func(ctx context.Context, req model.CreateOrderRequest) (*model.Order, error) {
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
									SKUVnt:   "some_sku",
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
							SKUVnt:   "some_sku",
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

func TestOrder_CreateNew(t *testing.T) {
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
			name: "invalid_email",
			given: tcGiven{
				svc: &mockOrderService{},
				body: `{
					"email": "you_example.com",
					"currency": "USD",
					"stripe_metadata": {
						"success_uri": "https://example.com/success",
						"cancel_uri": "https://example.com/cancel"
					},
					"payment_methods": ["stripe"],
					"items": [
						{
							"quantity": 1,
							"sku": "sku",
							"sku_variant": "sku_vnt",
							"location": "location",
							"description": "description",
							"credential_type": "credential_type",
							"credential_valid_duration": "P1M",
							"stripe_metadata": {
								"product_id": "product_id",
								"item_id": "item_id"
							}
						}
					]
				}`,
			},
			exp: tcExpected{
				err: &handlers.AppError{
					Message: "Validation failed",
					Code:    http.StatusBadRequest,
					Data: map[string]interface{}{"validationErrors": map[string]string{
						"Email": "Key: 'CreateOrderRequestNew.Email' Error:Field validation for 'Email' failed on the 'email' tag",
					}},
				},
			},
		},

		{
			name: "some_error",
			given: tcGiven{
				svc: &mockOrderService{
					fnCreateOrder: func(ctx context.Context, req *model.CreateOrderRequestNew) (*model.Order, error) {
						return nil, model.Error("some_error")
					},
				},
				body: `{
					"email": "you@example.com",
					"currency": "USD",
					"stripe_metadata": {
						"success_uri": "https://example.com/success",
						"cancel_uri": "https://example.com/cancel"
					},
					"payment_methods": ["stripe"],
					"items": [
						{
							"quantity": 1,
							"sku": "sku",
							"sku_variant": "sku_vnt",
							"location": "location",
							"description": "description",
							"credential_type": "credential_type",
							"credential_valid_duration": "P1M",
							"stripe_metadata": {
								"product_id": "product_id",
								"item_id": "item_id"
							}
						}
					]
				}`,
			},
			exp: tcExpected{
				err: handlers.WrapError(
					model.Error("something went wrong"),
					"Couldn't finish creating order",
					http.StatusInternalServerError,
				),
			},
		},

		{
			name: "success",
			given: tcGiven{
				svc: &mockOrderService{
					fnCreateOrder: func(ctx context.Context, req *model.CreateOrderRequestNew) (*model.Order, error) {
						result := &model.Order{
							Location: datastore.NullString{
								NullString: sql.NullString{
									Valid:  true,
									String: "location",
								},
							},
							Items: []model.OrderItem{
								{
									SKU:      "sku",
									SKUVnt:   "sku_vnt",
									Quantity: 1,
									Price:    mustDecimalFromString("1"),
									Subtotal: mustDecimalFromString("1"),
									Location: datastore.NullString{
										NullString: sql.NullString{
											Valid:  true,
											String: "location",
										},
									},
									Description: datastore.NullString{
										NullString: sql.NullString{
											Valid:  true,
											String: "description",
										},
									},
									CredentialType: "credential_type",
									ValidForISO:    ptrTo("P1M"),
									Metadata: datastore.Metadata{
										"stripe_product_id": "product_id",
										"stripe_item_id":    "item_id",
									},
								},
							},
							TotalPrice: mustDecimalFromString("1"),
						}

						return result, nil
					},
				},
				body: `{
					"email": "you@example.com",
					"currency": "USD",
					"stripe_metadata": {
						"success_uri": "https://example.com/success",
						"cancel_uri": "https://example.com/cancel"
					},
					"payment_methods": ["stripe"],
					"items": [
						{
							"quantity": 1,
							"sku": "sku",
							"sku_variant": "sku_vnt",
							"location": "location",
							"description": "description",
							"credential_type": "credential_type",
							"credential_valid_duration": "P1M",
							"stripe_metadata": {
								"product_id": "product_id",
								"item_id": "item_id"
							}
						}
					]
				}`,
			},
			exp: tcExpected{
				result: &model.Order{
					Location: datastore.NullString{
						NullString: sql.NullString{
							Valid:  true,
							String: "location",
						},
					},
					Items: []model.OrderItem{
						{
							SKU:      "sku",
							SKUVnt:   "sku_vnt",
							Quantity: 1,
							Price:    mustDecimalFromString("1"),
							Subtotal: mustDecimalFromString("1"),
							Location: datastore.NullString{
								NullString: sql.NullString{
									Valid:  true,
									String: "location",
								},
							},
							Description: datastore.NullString{
								NullString: sql.NullString{
									Valid:  true,
									String: "description",
								},
							},
							CredentialType: "credential_type",
							ValidForISO:    ptrTo("P1M"),
							Metadata: datastore.Metadata{
								"stripe_product_id": "product_id",
								"stripe_item_id":    "item_id",
							},
						},
					},
					TotalPrice: mustDecimalFromString("1"),
				},
			},
		},

		{
			name: "success_dicounts_metadata",
			given: tcGiven{
				svc: &mockOrderService{
					fnCreateOrder: func(ctx context.Context, req *model.CreateOrderRequestNew) (*model.Order, error) {
						if len(req.Discounts) != 1 {
							return nil, model.Error("unexpected_discounts")
						}

						if req.Discounts[0] != "coup_id_01" {
							return nil, model.Error("unexpected_discount_value")
						}

						if val, ok := req.Metadata["key_01"]; !ok || val != "val_01" {
							return nil, model.Error("unexpected_metadata_val")
						}

						result := &model.Order{
							Location: datastore.NullString{
								NullString: sql.NullString{
									Valid:  true,
									String: "location",
								},
							},
							Items: []model.OrderItem{
								{
									SKU:      "sku",
									SKUVnt:   "sku_vnt",
									Quantity: 1,
									Price:    mustDecimalFromString("1"),
									Subtotal: mustDecimalFromString("1"),
									Location: datastore.NullString{
										NullString: sql.NullString{
											Valid:  true,
											String: "location",
										},
									},
									Description: datastore.NullString{
										NullString: sql.NullString{
											Valid:  true,
											String: "description",
										},
									},
									CredentialType: "credential_type",
									ValidForISO:    ptrTo("P1M"),
									Metadata: datastore.Metadata{
										"stripe_product_id": "product_id",
										"stripe_item_id":    "item_id",
									},
								},
							},
							TotalPrice: mustDecimalFromString("1"),
						}

						return result, nil
					},
				},
				body: `{
					"email": "you@example.com",
					"currency": "USD",
					"stripe_metadata": {
						"success_uri": "https://example.com/success",
						"cancel_uri": "https://example.com/cancel"
					},
					"payment_methods": ["stripe"],
					"discounts": ["coup_id_01"],
					"items": [
						{
							"quantity": 1,
							"sku": "sku",
							"sku_variant": "sku_vnt",
							"location": "location",
							"description": "description",
							"credential_type": "credential_type",
							"credential_valid_duration": "P1M",
							"stripe_metadata": {
								"product_id": "product_id",
								"item_id": "item_id"
							}
						}
					],
					"metadata": {
						"key_01": "val_01"
					}
				}`,
			},
			exp: tcExpected{
				result: &model.Order{
					Location: datastore.NullString{
						NullString: sql.NullString{
							Valid:  true,
							String: "location",
						},
					},
					Items: []model.OrderItem{
						{
							SKU:      "sku",
							SKUVnt:   "sku_vnt",
							Quantity: 1,
							Price:    mustDecimalFromString("1"),
							Subtotal: mustDecimalFromString("1"),
							Location: datastore.NullString{
								NullString: sql.NullString{
									Valid:  true,
									String: "location",
								},
							},
							Description: datastore.NullString{
								NullString: sql.NullString{
									Valid:  true,
									String: "description",
								},
							},
							CredentialType: "credential_type",
							ValidForISO:    ptrTo("P1M"),
							Metadata: datastore.Metadata{
								"stripe_product_id": "product_id",
								"stripe_item_id":    "item_id",
							},
						},
					},
					TotalPrice: mustDecimalFromString("1"),
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

			act1 := h.CreateNew(rw, req)
			must.Equal(t, tc.exp.err, act1)

			if tc.exp.err != nil {
				act1.ServeHTTP(rw, req)
				resp := rw.Body.Bytes()

				exp, err := json.Marshal(tc.exp.err)
				must.Equal(t, nil, err)

				should.Equal(t, exp, bytes.TrimSpace(resp))
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

func TestOrder_Cancel(t *testing.T) {
	type tcGiven struct {
		ctx context.Context
		svc *mockOrderService
		oid uuid.UUID
	}

	type tcExpected struct {
		err *handlers.AppError
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "invalid_orderID",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"invalid_id"},
					},
				}),
				svc: &mockOrderService{},
				oid: uuid.Nil,
			},
			exp: tcExpected{
				err: handlers.ValidationError("request", map[string]interface{}{"orderID": model.ErrInvalidUUID}),
			},
		},

		{
			name: "context_cancelled",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				svc: &mockOrderService{
					fnCancelOrder: func(ctx context.Context, id uuid.UUID) error {
						return context.Canceled
					},
				},
				oid: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrSomethingWentWrong, "client ended request", model.StatusClientClosedConn),
			},
		},

		{
			name: "something_went_wrong",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				svc: &mockOrderService{
					fnCancelOrder: func(ctx context.Context, id uuid.UUID) error {
						return model.Error("any_error")
					},
				},
				oid: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrSomethingWentWrong, "could not cancel order", http.StatusInternalServerError),
			},
		},

		{
			name: "success",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				svc: &mockOrderService{},
				oid: uuid.Must(uuid.FromString("facade00-0000-4000-a000-000000000000")),
			},
			exp: tcExpected{},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			h := handler.NewOrder(tc.given.svc)

			uri := "http://localhost/v1/orders-new/" + tc.given.oid.String()
			req := httptest.NewRequest(http.MethodDelete, uri, nil)
			req = req.WithContext(tc.given.ctx)

			rw := httptest.NewRecorder()
			rw.Header().Set("content-type", "application/json")

			actual1 := h.Cancel(rw, req)
			must.Equal(t, tc.exp.err, actual1)

			if tc.exp.err != nil {
				actual1.ServeHTTP(rw, req)
				resp := rw.Body.Bytes()

				exp, err := json.Marshal(tc.exp.err)
				must.NoError(t, err)

				should.Equal(t, exp, bytes.TrimSpace(resp))
				return
			}
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

func ptrTo[T any](v T) *T {
	return &v
}
