package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	FnUniqBatches        func(ctx context.Context, orderID, itemID uuid.UUID) (int, int, error)
	FnListActiveBatches  func(ctx context.Context, orderID, itemID uuid.UUID) ([]model.TLV2ActiveBatch, error)
	FnDeleteBatches      func(ctx context.Context, orderID, itemID uuid.UUID, seats int) error
	FnExtendLinkingLimit func(ctx context.Context, orderID, itemID uuid.UUID) error
}

func (s *mockTLV2Svc) UniqBatches(ctx context.Context, orderID, itemID uuid.UUID) (int, int, error) {
	if s.FnUniqBatches == nil {
		return 10, 1, nil
	}

	return s.FnUniqBatches(ctx, orderID, itemID)
}

func (s *mockTLV2Svc) ListActiveBatches(ctx context.Context, orderID, itemID uuid.UUID) ([]model.TLV2ActiveBatch, error) {
	if s.FnListActiveBatches == nil {
		return nil, nil
	}

	return s.FnListActiveBatches(ctx, orderID, itemID)
}

func (s *mockTLV2Svc) DeleteBatches(ctx context.Context, orderID, itemID uuid.UUID, seats int) error {
	if s.FnDeleteBatches == nil {
		return nil
	}

	return s.FnDeleteBatches(ctx, orderID, itemID, seats)
}

func (s *mockTLV2Svc) ExtendLinkingLimit(ctx context.Context, orderID, itemID uuid.UUID) error {
	if s.FnExtendLinkingLimit == nil {
		return nil
	}

	return s.FnExtendLinkingLimit(ctx, orderID, itemID)
}

func TestCred_CountBatches(t *testing.T) {
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
				err: handlers.WrapError(context.Canceled, "client ended request", model.StatusClientClosedConn),
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

func TestCred_ListActiveBatches(t *testing.T) {
	orderCtx := func(orderID string) context.Context {
		return context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
			URLParams: chi.RouteParams{
				Keys:   []string{"orderID"},
				Values: []string{orderID},
			},
		})
	}

	type tcGiven struct {
		ctx    context.Context
		itemID string // query param; empty means omitted
		svc    *mockTLV2Svc
	}

	type tcExpected struct {
		batches []model.TLV2ActiveBatch
		err     *handlers.AppError
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
				ctx: orderCtx("not-a-uuid"),
				svc: &mockTLV2Svc{},
			},
			exp: tcExpected{
				err: handlers.ValidationError("request", map[string]interface{}{"orderID": "uuid: incorrect UUID length: not-a-uuid"}),
			},
		},

		{
			name: "invalid_item_id",
			given: tcGiven{
				ctx:    orderCtx("c0c0a000-0000-4000-a000-000000000000"),
				itemID: "not-a-uuid",
				svc:    &mockTLV2Svc{},
			},
			exp: tcExpected{
				err: handlers.ValidationError("request", map[string]interface{}{"item_id": "uuid: incorrect UUID length: not-a-uuid"}),
			},
		},

		{
			name: "order_not_found",
			given: tcGiven{
				ctx: orderCtx("c0c0a000-0000-4000-a000-000000000000"),
				svc: &mockTLV2Svc{
					FnListActiveBatches: func(ctx context.Context, orderID, itemID uuid.UUID) ([]model.TLV2ActiveBatch, error) {
						return nil, model.ErrOrderNotFound
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrOrderNotFound, "order not found", http.StatusNotFound),
			},
		},

		{
			name: "order_not_paid",
			given: tcGiven{
				ctx: orderCtx("c0c0a000-0000-4000-a000-000000000000"),
				svc: &mockTLV2Svc{
					FnListActiveBatches: func(ctx context.Context, orderID, itemID uuid.UUID) ([]model.TLV2ActiveBatch, error) {
						return nil, model.ErrOrderNotPaid
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrOrderNotPaid, "order not paid", http.StatusPaymentRequired),
			},
		},

		{
			name: "cred_type_not_supported",
			given: tcGiven{
				ctx: orderCtx("c0c0a000-0000-4000-a000-000000000000"),
				svc: &mockTLV2Svc{
					FnListActiveBatches: func(ctx context.Context, orderID, itemID uuid.UUID) ([]model.TLV2ActiveBatch, error) {
						return nil, model.ErrUnsupportedCredType
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrUnsupportedCredType, "credential type not supported", http.StatusBadRequest),
			},
		},

		{
			name: "context_canceled",
			given: tcGiven{
				ctx: orderCtx("c0c0a000-0000-4000-a000-000000000000"),
				svc: &mockTLV2Svc{
					FnListActiveBatches: func(ctx context.Context, orderID, itemID uuid.UUID) ([]model.TLV2ActiveBatch, error) {
						return nil, context.Canceled
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(context.Canceled, "client ended request", model.StatusClientClosedConn),
			},
		},

		{
			name: "deadline_exceeded",
			given: tcGiven{
				ctx: orderCtx("c0c0a000-0000-4000-a000-000000000000"),
				svc: &mockTLV2Svc{
					FnListActiveBatches: func(ctx context.Context, orderID, itemID uuid.UUID) ([]model.TLV2ActiveBatch, error) {
						return nil, context.DeadlineExceeded
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(context.DeadlineExceeded, "request timed out", http.StatusGatewayTimeout),
			},
		},

		{
			name: "internal_error",
			given: tcGiven{
				ctx: orderCtx("c0c0a000-0000-4000-a000-000000000000"),
				svc: &mockTLV2Svc{
					FnListActiveBatches: func(ctx context.Context, orderID, itemID uuid.UUID) ([]model.TLV2ActiveBatch, error) {
						return nil, model.Error("unexpected")
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrSomethingWentWrong, "something went wrong", http.StatusInternalServerError),
			},
		},

		{
			name: "success_empty_returns_array_not_null",
			given: tcGiven{
				ctx: orderCtx("c0c0a000-0000-4000-a000-000000000000"),
				svc: &mockTLV2Svc{
					FnListActiveBatches: func(ctx context.Context, orderID, itemID uuid.UUID) ([]model.TLV2ActiveBatch, error) {
						return nil, nil // service may return nil slice
					},
				},
			},
			exp: tcExpected{
				batches: []model.TLV2ActiveBatch{}, // handler must promote nil → []
			},
		},

		{
			name: "success_no_item_filter",
			given: tcGiven{
				ctx: orderCtx("c0c0a000-0000-4000-a000-000000000000"),
				svc: &mockTLV2Svc{
					FnListActiveBatches: func(ctx context.Context, orderID, itemID uuid.UUID) ([]model.TLV2ActiveBatch, error) {
						should.Equal(t, uuid.Nil, itemID)
						return []model.TLV2ActiveBatch{{RequestID: "req-01"}}, nil
					},
				},
			},
			exp: tcExpected{
				batches: []model.TLV2ActiveBatch{{RequestID: "req-01"}},
			},
		},

		{
			name: "success_with_item_filter",
			given: tcGiven{
				ctx:    orderCtx("c0c0a000-0000-4000-a000-000000000000"),
				itemID: "ad0be000-0000-4000-a000-000000000000",
				svc: &mockTLV2Svc{
					FnListActiveBatches: func(ctx context.Context, orderID, itemID uuid.UUID) ([]model.TLV2ActiveBatch, error) {
						should.Equal(t, "ad0be000-0000-4000-a000-000000000000", itemID.String())
						return []model.TLV2ActiveBatch{{RequestID: "req-02"}}, nil
					},
				},
			},
			exp: tcExpected{
				batches: []model.TLV2ActiveBatch{{RequestID: "req-02"}},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			h := handler.NewCred(tc.given.svc)

			target := "http://localhost"
			if tc.given.itemID != "" {
				target += "?item_id=" + tc.given.itemID
			}

			req := httptest.NewRequest(http.MethodGet, target, nil)
			req = req.WithContext(tc.given.ctx)

			rw := httptest.NewRecorder()
			rw.Header().Set("content-type", "application/json")

			appErr := h.ListActiveBatches(rw, req)
			must.Equal(t, tc.exp.err, appErr)

			if tc.exp.err != nil {
				appErr.ServeHTTP(rw, req)
				exp, err := json.Marshal(tc.exp.err)
				must.Equal(t, nil, err)
				should.Equal(t, exp, bytes.TrimSpace(rw.Body.Bytes()))
				return
			}

			resp := &struct {
				Batches []model.TLV2ActiveBatch `json:"batches"`
			}{}
			must.Equal(t, nil, json.Unmarshal(rw.Body.Bytes(), resp))
			should.Equal(t, tc.exp.batches, resp.Batches)
		})
	}
}

func TestCred_DeleteBatches(t *testing.T) {
	orderCtx := func(orderID string) context.Context {
		return context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
			URLParams: chi.RouteParams{
				Keys:   []string{"orderID"},
				Values: []string{orderID},
			},
		})
	}

	type tcGiven struct {
		ctx  context.Context
		body string
		svc  *mockTLV2Svc
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
				ctx:  orderCtx("not-a-uuid"),
				body: `{"seats":1}`,
				svc:  &mockTLV2Svc{},
			},
			exp: tcExpected{
				err: handlers.ValidationError("request", map[string]interface{}{"orderID": "uuid: incorrect UUID length: not-a-uuid"}),
			},
		},

		{
			name: "invalid_json_body",
			given: tcGiven{
				ctx:  orderCtx("c0c0a000-0000-4000-a000-000000000000"),
				body: `{not json}`,
				svc:  &mockTLV2Svc{},
			},
			exp: tcExpected{
				err: handlers.WrapError(
					json.NewDecoder(strings.NewReader(`{not json}`)).Decode(&struct{}{}),
					"failed to parse request body",
					http.StatusBadRequest,
				),
			},
		},

		{
			name: "seats_zero",
			given: tcGiven{
				ctx:  orderCtx("c0c0a000-0000-4000-a000-000000000000"),
				body: `{"seats":0}`,
				svc:  &mockTLV2Svc{},
			},
			exp: tcExpected{
				err: handlers.ValidationError("request", map[string]interface{}{"seats": "must be a positive integer"}),
			},
		},

		{
			name: "invalid_item_id",
			given: tcGiven{
				ctx:  orderCtx("c0c0a000-0000-4000-a000-000000000000"),
				body: `{"seats":1,"item_id":"not-a-uuid"}`,
				svc:  &mockTLV2Svc{},
			},
			exp: tcExpected{
				err: handlers.ValidationError("request", map[string]interface{}{"item_id": "uuid: incorrect UUID length: not-a-uuid"}),
			},
		},

		{
			name: "order_not_found",
			given: tcGiven{
				ctx:  orderCtx("c0c0a000-0000-4000-a000-000000000000"),
				body: `{"seats":1}`,
				svc: &mockTLV2Svc{
					FnDeleteBatches: func(ctx context.Context, orderID, itemID uuid.UUID, seats int) error {
						return model.ErrOrderNotFound
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrOrderNotFound, "order not found", http.StatusNotFound),
			},
		},

		{
			name: "order_not_paid",
			given: tcGiven{
				ctx:  orderCtx("c0c0a000-0000-4000-a000-000000000000"),
				body: `{"seats":1}`,
				svc: &mockTLV2Svc{
					FnDeleteBatches: func(ctx context.Context, orderID, itemID uuid.UUID, seats int) error {
						return model.ErrOrderNotPaid
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrOrderNotPaid, "order not paid", http.StatusPaymentRequired),
			},
		},

		{
			name: "cred_type_not_supported",
			given: tcGiven{
				ctx:  orderCtx("c0c0a000-0000-4000-a000-000000000000"),
				body: `{"seats":1}`,
				svc: &mockTLV2Svc{
					FnDeleteBatches: func(ctx context.Context, orderID, itemID uuid.UUID, seats int) error {
						return model.ErrUnsupportedCredType
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrUnsupportedCredType, "credential type not supported", http.StatusBadRequest),
			},
		},

		{
			name: "deadline_exceeded",
			given: tcGiven{
				ctx:  orderCtx("c0c0a000-0000-4000-a000-000000000000"),
				body: `{"seats":1}`,
				svc: &mockTLV2Svc{
					FnDeleteBatches: func(ctx context.Context, orderID, itemID uuid.UUID, seats int) error {
						return context.DeadlineExceeded
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(context.DeadlineExceeded, "request timed out", http.StatusGatewayTimeout),
			},
		},

		{
			name: "internal_error",
			given: tcGiven{
				ctx:  orderCtx("c0c0a000-0000-4000-a000-000000000000"),
				body: `{"seats":1}`,
				svc: &mockTLV2Svc{
					FnDeleteBatches: func(ctx context.Context, orderID, itemID uuid.UUID, seats int) error {
						return model.Error("unexpected")
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrSomethingWentWrong, "something went wrong", http.StatusInternalServerError),
			},
		},

		{
			name: "seats_exceeds_batch_count",
			given: tcGiven{
				ctx:  orderCtx("c0c0a000-0000-4000-a000-000000000000"),
				body: `{"seats":5}`,
				svc: &mockTLV2Svc{
					FnDeleteBatches: func(ctx context.Context, orderID, itemID uuid.UUID, seats int) error {
						return model.ErrBatchSeatsExceeded
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrBatchSeatsExceeded, "seats exceeds active batch count", http.StatusBadRequest),
			},
		},

		{
			name: "success",
			given: tcGiven{
				ctx:  orderCtx("c0c0a000-0000-4000-a000-000000000000"),
				body: `{"seats":2,"item_id":"ad0be000-0000-4000-a000-000000000000"}`,
				svc: &mockTLV2Svc{
					FnDeleteBatches: func(ctx context.Context, orderID, itemID uuid.UUID, seats int) error {
						should.Equal(t, 2, seats)
						should.Equal(t, "ad0be000-0000-4000-a000-000000000000", itemID.String())
						return nil
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			h := handler.NewCred(tc.given.svc)

			req := httptest.NewRequest(http.MethodDelete, "http://localhost", strings.NewReader(tc.given.body))
			req = req.WithContext(tc.given.ctx)

			rw := httptest.NewRecorder()
			rw.Header().Set("content-type", "application/json")

			appErr := h.DeleteBatches(rw, req)
			must.Equal(t, tc.exp.err, appErr)

			if tc.exp.err != nil {
				appErr.ServeHTTP(rw, req)
				exp, err := json.Marshal(tc.exp.err)
				must.Equal(t, nil, err)
				should.Equal(t, exp, bytes.TrimSpace(rw.Body.Bytes()))
				return
			}

			should.Equal(t, http.StatusOK, rw.Code)
		})
	}
}

func TestCred_ExtendLinkingLimit(t *testing.T) {
	routeCtx := func(orderID, itemID string) context.Context {
		return context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
			URLParams: chi.RouteParams{
				Keys:   []string{"orderID", "itemID"},
				Values: []string{orderID, itemID},
			},
		})
	}

	type tcGiven struct {
		ctx context.Context
		svc *mockTLV2Svc
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
				ctx: routeCtx("not-a-uuid", "ad0be000-0000-4000-a000-000000000000"),
				svc: &mockTLV2Svc{},
			},
			exp: tcExpected{
				err: handlers.ValidationError("request", map[string]interface{}{"orderID": "uuid: incorrect UUID length: not-a-uuid"}),
			},
		},

		{
			name: "invalid_itemID",
			given: tcGiven{
				ctx: routeCtx("c0c0a000-0000-4000-a000-000000000000", "not-a-uuid"),
				svc: &mockTLV2Svc{},
			},
			exp: tcExpected{
				err: handlers.ValidationError("request", map[string]interface{}{"itemID": "uuid: incorrect UUID length: not-a-uuid"}),
			},
		},

		{
			name: "order_forbidden",
			given: tcGiven{
				ctx: routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				svc: &mockTLV2Svc{
					FnExtendLinkingLimit: func(ctx context.Context, orderID, itemID uuid.UUID) error {
						return model.ErrOrderForbidden
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrOrderForbidden, "order access forbidden", http.StatusForbidden),
			},
		},

		{
			name: "context_canceled",
			given: tcGiven{
				ctx: routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				svc: &mockTLV2Svc{
					FnExtendLinkingLimit: func(ctx context.Context, orderID, itemID uuid.UUID) error {
						return context.Canceled
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(context.Canceled, "client ended request", model.StatusClientClosedConn),
			},
		},

		{
			name: "deadline_exceeded",
			given: tcGiven{
				ctx: routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				svc: &mockTLV2Svc{
					FnExtendLinkingLimit: func(ctx context.Context, orderID, itemID uuid.UUID) error {
						return context.DeadlineExceeded
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(context.DeadlineExceeded, "request timed out", http.StatusGatewayTimeout),
			},
		},

		{
			name: "order_not_found",
			given: tcGiven{
				ctx: routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				svc: &mockTLV2Svc{
					FnExtendLinkingLimit: func(ctx context.Context, orderID, itemID uuid.UUID) error {
						return model.ErrOrderNotFound
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrOrderNotFound, "order not found", http.StatusNotFound),
			},
		},

		{
			name: "order_not_paid",
			given: tcGiven{
				ctx: routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				svc: &mockTLV2Svc{
					FnExtendLinkingLimit: func(ctx context.Context, orderID, itemID uuid.UUID) error {
						return model.ErrOrderNotPaid
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
				ctx: routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				svc: &mockTLV2Svc{
					FnExtendLinkingLimit: func(ctx context.Context, orderID, itemID uuid.UUID) error {
						return model.ErrUnsupportedCredType
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrUnsupportedCredType, "credential type not supported", http.StatusBadRequest),
			},
		},

		{
			name: "slots_already_available",
			given: tcGiven{
				ctx: routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				svc: &mockTLV2Svc{
					FnExtendLinkingLimit: func(ctx context.Context, orderID, itemID uuid.UUID) error {
						return model.ErrExtensionSlotsAvailable
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrExtensionSlotsAvailable, "slots already available", http.StatusBadRequest),
			},
		},

		{
			name: "extension_cap_reached",
			given: tcGiven{
				ctx: routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				svc: &mockTLV2Svc{
					FnExtendLinkingLimit: func(ctx context.Context, orderID, itemID uuid.UUID) error {
						return model.ErrExtensionCapReached
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrExtensionCapReached, "extension cap reached", http.StatusForbidden),
			},
		},

		{
			name: "rate_limited",
			given: tcGiven{
				ctx: routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				svc: &mockTLV2Svc{
					FnExtendLinkingLimit: func(ctx context.Context, orderID, itemID uuid.UUID) error {
						return model.ErrExtensionRateLimited
					},
				},
			},
			exp: tcExpected{
				err: handlers.WrapError(model.ErrExtensionRateLimited, "extension rate limited", http.StatusTooManyRequests),
			},
		},

		{
			name: "internal_error",
			given: tcGiven{
				ctx: routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				svc: &mockTLV2Svc{
					FnExtendLinkingLimit: func(ctx context.Context, orderID, itemID uuid.UUID) error {
						return model.Error("unexpected")
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
				ctx: routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				svc: &mockTLV2Svc{
					FnExtendLinkingLimit: func(ctx context.Context, orderID, itemID uuid.UUID) error {
						must.Equal(t, "c0c0a000-0000-4000-a000-000000000000", orderID.String())
						must.Equal(t, "ad0be000-0000-4000-a000-000000000000", itemID.String())
						return nil
					},
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			h := handler.NewCred(tc.given.svc)

			req := httptest.NewRequest(http.MethodPost, "http://localhost", nil)
			req = req.WithContext(tc.given.ctx)

			rw := httptest.NewRecorder()
			rw.Header().Set("content-type", "application/json")

			appErr := h.ExtendLinkingLimit(rw, req)
			must.Equal(t, tc.exp.err, appErr)

			if tc.exp.err != nil {
				appErr.ServeHTTP(rw, req)
				exp, err := json.Marshal(tc.exp.err)
				must.Equal(t, nil, err)
				should.Equal(t, exp, bytes.TrimSpace(rw.Body.Bytes()))
				return
			}

			should.Equal(t, http.StatusOK, rw.Code)
		})
	}
}
