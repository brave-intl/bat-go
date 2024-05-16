package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/libs/handlers"

	"github.com/brave-intl/bat-go/services/skus/handler"
	"github.com/brave-intl/bat-go/services/skus/model"
)

type mockTLV2Svc struct {
	FnUniqBatches func(ctx context.Context, orderID, itemID uuid.UUID) (int, int, error)
}

func (s *mockTLV2Svc) UniqBatches(ctx context.Context, orderID, itemID uuid.UUID) (int, int, error) {
	if s.FnUniqBatches == nil {
		return 10, 1, nil
	}

	return s.FnUniqBatches(ctx, orderID, itemID)
}

func TestCredential_CountBatches(t *testing.T) {
	type tcGiven struct {
		ctx context.Context
		svc *mockTLV2Svc
	}

	type tcExpected struct {
		lim  int
		nact int
		err  *handlers.AppError
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
				svc: &mockTLV2Svc{},
			},
			exp: tcExpected{
				err: handlers.ValidationError("request", map[string]interface{}{"orderID": "uuid: incorrect UUID length: invalid_id"}),
			},
		},

		{
			name: "context_cancelled",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"c0c0a000-0000-4000-a000-000000000000"},
					},
				}),
				svc: &mockTLV2Svc{
					FnUniqBatches: func(ctx context.Context, orderID, itemID uuid.UUID) (int, int, error) {
						return 0, 0, context.Canceled
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(context.Canceled, "cliend ended request", model.StatusClientClosedConn),
			},
		},

		{
			name: "order_not_found",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"c0c0a000-0000-4000-a000-000000000000"},
					},
				}),
				svc: &mockTLV2Svc{
					FnUniqBatches: func(ctx context.Context, orderID, itemID uuid.UUID) (int, int, error) {
						return 0, 0, model.ErrOrderNotFound
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrOrderNotFound, "order not found", http.StatusNotFound),
			},
		},

		{
			name: "order_not_found_no_items",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"c0c0a000-0000-4000-a000-000000000000"},
					},
				}),
				svc: &mockTLV2Svc{
					FnUniqBatches: func(ctx context.Context, orderID, itemID uuid.UUID) (int, int, error) {
						return 0, 0, model.ErrInvalidOrderNoItems
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrInvalidOrderNoItems, "order not found", http.StatusNotFound),
			},
		},

		{
			name: "order_not_found_item",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"c0c0a000-0000-4000-a000-000000000000"},
					},
				}),
				svc: &mockTLV2Svc{
					FnUniqBatches: func(ctx context.Context, orderID, itemID uuid.UUID) (int, int, error) {
						return 0, 0, model.ErrOrderItemNotFound
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrOrderItemNotFound, "order not found", http.StatusNotFound),
			},
		},

		{
			name: "order_not_paid",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"c0c0a000-0000-4000-a000-000000000000"},
					},
				}),
				svc: &mockTLV2Svc{
					FnUniqBatches: func(ctx context.Context, orderID, itemID uuid.UUID) (int, int, error) {
						return 0, 0, model.ErrOrderNotPaid
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrOrderNotPaid, "order not paid", http.StatusPaymentRequired),
			},
		},

		{
			name: "unsupported_cred_type",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"c0c0a000-0000-4000-a000-000000000000"},
					},
				}),
				svc: &mockTLV2Svc{
					FnUniqBatches: func(ctx context.Context, orderID, itemID uuid.UUID) (int, int, error) {
						return 0, 0, model.ErrUnsupportedCredType
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrUnsupportedCredType, "credential type not supported", http.StatusBadRequest),
			},
		},

		{
			name: "something_went_wrong",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"c0c0a000-0000-4000-a000-000000000000"},
					},
				}),
				svc: &mockTLV2Svc{
					FnUniqBatches: func(ctx context.Context, orderID, itemID uuid.UUID) (int, int, error) {
						return 0, 0, model.Error("any_error")
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrSomethingWentWrong, "something went wrong", http.StatusInternalServerError),
			},
		},

		{
			name: "success",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"c0c0a000-0000-4000-a000-000000000000"},
					},
				}),
				svc: &mockTLV2Svc{
					FnUniqBatches: func(ctx context.Context, orderID, itemID uuid.UUID) (int, int, error) {
						return 10, 1, nil
					},
				},
			},
			exp: tcExpected{
				lim:  10,
				nact: 1,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			h := handler.NewCred(tc.given.svc)

			req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
			req = req.WithContext(tc.given.ctx)

			rw := httptest.NewRecorder()
			rw.Header().Set("content-type", "application/json")

			act1 := h.CountBatches(rw, req)
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
			act2 := &struct {
				L int `json:"limit"`
				A int `json:"active"`
			}{}

			err := json.Unmarshal(resp, act2)
			must.Equal(t, nil, err)

			should.Equal(t, tc.exp.lim, act2.L)
			should.Equal(t, tc.exp.nact, act2.A)
		})
	}
}
