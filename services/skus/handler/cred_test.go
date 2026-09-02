package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/libs/handlers"

	"github.com/brave-intl/bat-go/services/skus/handler"
	"github.com/brave-intl/bat-go/services/skus/model"
)

type mockTLV2Svc struct {
	FnUniqBatches                      func(ctx context.Context, orderID, itemID uuid.UUID) (*model.BatchesStatus, error)
	FnListActiveBatches                func(ctx context.Context, orderID, itemID uuid.UUID) ([]model.TLV2ActiveBatch, error)
	FnDeleteBatches                    func(ctx context.Context, orderID, itemID uuid.UUID, seats int) error
	FnExtendLinkingLimit               func(ctx context.Context, orderID, itemID uuid.UUID, write model.ExtensionWrite) error
	FnExtendLinkingLimitWithReceipt    func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error
	FnCanExtendLinkingLimitWithReceipt func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error
}

func (s *mockTLV2Svc) ExtendLinkingLimitWithReceipt(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
	if s.FnExtendLinkingLimitWithReceipt == nil {
		return nil
	}

	return s.FnExtendLinkingLimitWithReceipt(ctx, orderID, req)
}

func (s *mockTLV2Svc) CanExtendLinkingLimitWithReceipt(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
	if s.FnCanExtendLinkingLimitWithReceipt == nil {
		return nil
	}

	return s.FnCanExtendLinkingLimitWithReceipt(ctx, orderID, req)
}

func (s *mockTLV2Svc) UniqBatches(ctx context.Context, orderID, itemID uuid.UUID) (*model.BatchesStatus, error) {
	if s.FnUniqBatches == nil {
		return &model.BatchesStatus{Limit: 10, Active: 1}, nil
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

func (s *mockTLV2Svc) ExtendLinkingLimit(ctx context.Context, orderID, itemID uuid.UUID, write model.ExtensionWrite) error {
	if s.FnExtendLinkingLimit == nil {
		return nil
	}

	return s.FnExtendLinkingLimit(ctx, orderID, itemID, write)
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
					FnUniqBatches: func(ctx context.Context, orderID, itemID uuid.UUID) (*model.BatchesStatus, error) {
						return nil, context.Canceled
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
					FnUniqBatches: func(ctx context.Context, orderID, itemID uuid.UUID) (*model.BatchesStatus, error) {
						return nil, model.ErrOrderNotFound
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
					FnUniqBatches: func(ctx context.Context, orderID, itemID uuid.UUID) (*model.BatchesStatus, error) {
						return nil, model.ErrInvalidOrderNoItems
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
					FnUniqBatches: func(ctx context.Context, orderID, itemID uuid.UUID) (*model.BatchesStatus, error) {
						return nil, model.ErrOrderItemNotFound
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
					FnUniqBatches: func(ctx context.Context, orderID, itemID uuid.UUID) (*model.BatchesStatus, error) {
						return nil, model.ErrOrderNotPaid
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
					FnUniqBatches: func(ctx context.Context, orderID, itemID uuid.UUID) (*model.BatchesStatus, error) {
						return nil, model.ErrUnsupportedCredType
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
					FnUniqBatches: func(ctx context.Context, orderID, itemID uuid.UUID) (*model.BatchesStatus, error) {
						return nil, model.Error("any_error")
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
					FnUniqBatches: func(ctx context.Context, orderID, itemID uuid.UUID) (*model.BatchesStatus, error) {
						return &model.BatchesStatus{Limit: 10, Active: 1, NumSelfExtensions: 2}, nil
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

	const validBody = `{"expected_last_self_extension_at":null,"new_limit":13}`

	type tcGiven struct {
		ctx        context.Context
		body       string
		bodyReader io.Reader
		svc        *mockTLV2Svc
	}

	type tcExpected struct {
		err *handlers.AppError
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	withCode := func(appErr *handlers.AppError, code string) *handlers.AppError {
		appErr.ErrorCode = code
		return appErr
	}

	tests := []testCase{
		{
			name: "invalid_orderID",
			given: tcGiven{
				ctx:  routeCtx("not-a-uuid", "ad0be000-0000-4000-a000-000000000000"),
				body: validBody,
				svc:  &mockTLV2Svc{},
			},
			exp: tcExpected{
				err: handlers.ValidationError("request", map[string]interface{}{"orderID": "uuid: incorrect UUID length: not-a-uuid"}),
			},
		},

		{
			name: "invalid_itemID",
			given: tcGiven{
				ctx:  routeCtx("c0c0a000-0000-4000-a000-000000000000", "not-a-uuid"),
				body: validBody,
				svc:  &mockTLV2Svc{},
			},
			exp: tcExpected{
				err: handlers.ValidationError("request", map[string]interface{}{"itemID": "uuid: incorrect UUID length: not-a-uuid"}),
			},
		},

		{
			name: "malformed_body",
			given: tcGiven{
				ctx:  routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				body: "not-json",
				svc:  &mockTLV2Svc{},
			},
		},

		{
			name: "io_read_failure",
			given: tcGiven{
				ctx:        routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				bodyReader: iotest.ErrReader(io.ErrUnexpectedEOF),
				svc:        &mockTLV2Svc{},
			},
			exp: tcExpected{
				err: withCode(handlers.WrapError(io.ErrUnexpectedEOF, "failed to read request body", http.StatusBadRequest), model.ExtensionCodeMalformedBody),
			},
		},

		{
			name: "context_canceled",
			given: tcGiven{
				ctx:  routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				body: validBody,
				svc: &mockTLV2Svc{
					FnExtendLinkingLimit: func(ctx context.Context, orderID, itemID uuid.UUID, _ model.ExtensionWrite) error {
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
				ctx:  routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				body: validBody,
				svc: &mockTLV2Svc{
					FnExtendLinkingLimit: func(ctx context.Context, orderID, itemID uuid.UUID, _ model.ExtensionWrite) error {
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
				ctx:  routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				body: validBody,
				svc: &mockTLV2Svc{
					FnExtendLinkingLimit: func(ctx context.Context, orderID, itemID uuid.UUID, _ model.ExtensionWrite) error {
						return model.ErrOrderNotFound
					},
				},
			},
			exp: tcExpected{
				err: withCode(handlers.WrapError(model.ErrOrderNotFound, "order not found", http.StatusNotFound), model.ExtensionCodeOrderNotFound),
			},
		},

		{
			name: "order_not_paid",
			given: tcGiven{
				ctx:  routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				body: validBody,
				svc: &mockTLV2Svc{
					FnExtendLinkingLimit: func(ctx context.Context, orderID, itemID uuid.UUID, _ model.ExtensionWrite) error {
						return model.ErrOrderNotPaid
					},
				},
			},
			exp: tcExpected{
				err: withCode(handlers.WrapError(model.ErrOrderNotPaid, "order not paid", http.StatusPaymentRequired), model.ExtensionCodeOrderNotPaid),
			},
		},

		{
			name: "unsupported_cred_type",
			given: tcGiven{
				ctx:  routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				body: validBody,
				svc: &mockTLV2Svc{
					FnExtendLinkingLimit: func(ctx context.Context, orderID, itemID uuid.UUID, _ model.ExtensionWrite) error {
						return model.ErrUnsupportedCredType
					},
				},
			},
			exp: tcExpected{
				err: withCode(handlers.WrapError(model.ErrUnsupportedCredType, "credential type not supported", http.StatusBadRequest), model.ExtensionCodeUnsupportedCredType),
			},
		},

		{
			name: "invalid_limit",
			given: tcGiven{
				ctx:  routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				body: validBody,
				svc: &mockTLV2Svc{
					FnExtendLinkingLimit: func(ctx context.Context, orderID, itemID uuid.UUID, _ model.ExtensionWrite) error {
						return model.ErrExtensionInvalidLimit
					},
				},
			},
			exp: tcExpected{
				err: withCode(handlers.WrapError(model.ErrExtensionInvalidLimit, "extension new limit invalid", http.StatusUnprocessableEntity), model.ExtensionCodeInvalidLimit),
			},
		},

		{
			name: "conflict",
			given: tcGiven{
				ctx:  routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				body: validBody,
				svc: &mockTLV2Svc{
					FnExtendLinkingLimit: func(ctx context.Context, orderID, itemID uuid.UUID, _ model.ExtensionWrite) error {
						return model.ErrExtensionConflict
					},
				},
			},
			exp: tcExpected{
				err: withCode(handlers.WrapError(model.ErrExtensionConflict, "extension version conflict", http.StatusConflict), model.ExtensionCodeConflict),
			},
		},

		{
			name: "internal_error",
			given: tcGiven{
				ctx:  routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				body: validBody,
				svc: &mockTLV2Svc{
					FnExtendLinkingLimit: func(ctx context.Context, orderID, itemID uuid.UUID, _ model.ExtensionWrite) error {
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
				ctx:  routeCtx("c0c0a000-0000-4000-a000-000000000000", "ad0be000-0000-4000-a000-000000000000"),
				body: validBody,
				svc: &mockTLV2Svc{
					FnExtendLinkingLimit: func(ctx context.Context, orderID, itemID uuid.UUID, write model.ExtensionWrite) error {
						must.Equal(t, "c0c0a000-0000-4000-a000-000000000000", orderID.String())
						must.Equal(t, "ad0be000-0000-4000-a000-000000000000", itemID.String())
						must.Equal(t, 13, write.NewLimit)
						must.Nil(t, write.ExpectedLastSelfExtensionAt)
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

			var body io.Reader = strings.NewReader(tc.given.body)
			if tc.given.bodyReader != nil {
				body = tc.given.bodyReader
			}

			req := httptest.NewRequest(http.MethodPost, "http://localhost", body)
			req = req.WithContext(tc.given.ctx)

			rw := httptest.NewRecorder()
			rw.Header().Set("content-type", "application/json")

			appErr := h.ExtendLinkingLimit(rw, req)

			if tc.name == "malformed_body" {
				must.NotNil(t, appErr)
				should.Equal(t, http.StatusBadRequest, appErr.Code)
				should.Equal(t, "failed to parse request body", appErr.Message)
				should.Equal(t, model.ExtensionCodeMalformedBody, appErr.ErrorCode)

				return
			}

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

func TestCred_ExtendLinkingLimitWithReceipt(t *testing.T) {
	type appErrorExp struct {
		code      int
		errCode   string
		message   string
		data      interface{}
		mustCause must.ErrorAssertionFunc
	}

	type tcGiven struct {
		ctx  context.Context
		body string
		svc  *mockTLV2Svc
	}

	type tcExpected struct {
		code int
		err  *appErrorExp
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "error_parse_order_id",
			given: tcGiven{
				ctx: context.Background(),
			},
			exp: tcExpected{
				code: http.StatusBadRequest,
				err: &appErrorExp{
					code:    http.StatusBadRequest,
					message: "Error validating request",
					data:    map[string]interface{}{"validationErrors": map[string]interface{}{"orderID": "uuid: incorrect UUID length: "}},
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.Nil(t, err)
					},
				},
			},
		},

		{
			name: "error_unmarshal_body",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
			},
			exp: tcExpected{
				code: http.StatusBadRequest,
				err: &appErrorExp{
					code:    http.StatusBadRequest,
					errCode: model.ExtensionCodeMalformedBody,
					message: "failed to parse request body",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.NotNil(t, err)
					},
				},
			},
		},

		{
			name: "error_client_ended_request",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return context.Canceled
					},
				},
			},
			exp: tcExpected{
				code: model.StatusClientClosedConn,
				err: &appErrorExp{
					code:    model.StatusClientClosedConn,
					message: "client ended request",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, context.Canceled)
					},
				},
			},
		},

		{
			name: "error_request_timed_out",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return context.DeadlineExceeded
					},
				},
			},
			exp: tcExpected{
				code: http.StatusGatewayTimeout,
				err: &appErrorExp{
					code:    http.StatusGatewayTimeout,
					message: "request timed out",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, context.DeadlineExceeded)
					},
				},
			},
		},

		{
			name: "error_order_not_found",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrOrderNotFound
					},
				},
			},
			exp: tcExpected{
				code: http.StatusNotFound,
				err: &appErrorExp{
					code:    http.StatusNotFound,
					errCode: model.ExtensionCodeOrderNotFound,
					message: "order not found",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrOrderNotFound)
					},
				},
			},
		},

		{
			name: "error_invalid_order_no_items",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrInvalidOrderNoItems
					},
				},
			},
			exp: tcExpected{
				code: http.StatusNotFound,
				err: &appErrorExp{
					code:    http.StatusNotFound,
					errCode: model.ExtensionCodeOrderNotFound,
					message: "order not found",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrInvalidOrderNoItems)
					},
				},
			},
		},

		{
			name: "error_order_item_not_found",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrOrderItemNotFound
					},
				},
			},
			exp: tcExpected{
				code: http.StatusNotFound,
				err: &appErrorExp{
					code:    http.StatusNotFound,
					errCode: model.ExtensionCodeOrderNotFound,
					message: "order not found",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrOrderItemNotFound)
					},
				},
			},
		},

		{
			name: "error_order_not_paid",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrOrderNotPaid
					},
				},
			},
			exp: tcExpected{
				code: http.StatusPaymentRequired,
				err: &appErrorExp{
					code:    http.StatusPaymentRequired,
					errCode: model.ExtensionCodeOrderNotPaid,
					message: "order not paid",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrOrderNotPaid)
					},
				},
			},
		},

		{
			name: "error_credential_type_not_supported",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrUnsupportedCredType
					},
				},
			},
			exp: tcExpected{
				code: http.StatusBadRequest,
				err: &appErrorExp{
					code:    http.StatusBadRequest,
					errCode: model.ExtensionCodeUnsupportedCredType,
					message: "credential type not supported",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrUnsupportedCredType)
					},
				},
			},
		},

		{
			name: "error_extension_invalid_limit",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrExtensionInvalidLimit
					},
				},
			},
			exp: tcExpected{
				code: http.StatusUnprocessableEntity,
				err: &appErrorExp{
					code:    http.StatusUnprocessableEntity,
					errCode: model.ExtensionCodeInvalidLimitX,
					message: "extension new limit invalid",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrExtensionInvalidLimit)
					},
				},
			},
		},

		{
			name: "error_extension_not_supported",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrNoExtensionPolicy
					},
				},
			},
			exp: tcExpected{
				code: http.StatusUnprocessableEntity,
				err: &appErrorExp{
					code:    http.StatusUnprocessableEntity,
					errCode: model.ExtensionNotSupported,
					message: "item does not support extension",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrNoExtensionPolicy)
					},
				},
			},
		},

		{
			name: "error_extension_rate_limited",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrExtensionRateLimited
					},
				},
			},
			exp: tcExpected{
				code: http.StatusTooManyRequests,
				err: &appErrorExp{
					code:    http.StatusTooManyRequests,
					errCode: model.ExtensionCodeRateLimited,
					message: "extension rate limited",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrExtensionRateLimited)
					},
				},
			},
		},

		{
			name: "error_max_extension_per_item_reached",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrExtensionMaxPerItem
					},
				},
			},
			exp: tcExpected{
				code: http.StatusUnprocessableEntity,
				err: &appErrorExp{
					code:    http.StatusUnprocessableEntity,
					errCode: model.ExtensionCodeMaxPerItem,
					message: "max extensions per item reached",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrExtensionMaxPerItem)
					},
				},
			},
		},

		{
			name: "error_extension_not_at_limit",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrExtensionNotAtLimit
					},
				},
			},
			exp: tcExpected{
				code: http.StatusUnprocessableEntity,
				err: &appErrorExp{
					code:    http.StatusUnprocessableEntity,
					errCode: model.ExtensionCodeNotAtLimit,
					message: "not at limit; extension not needed",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrExtensionNotAtLimit)
					},
				},
			},
		},

		{
			name: "error_receipt_valid_error",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return &model.ReceiptValidError{Err: model.Error("some_error")}
					},
				},
			},
			exp: tcExpected{
				code: http.StatusBadRequest,
				err: &appErrorExp{
					code:    http.StatusBadRequest,
					errCode: "validation_failed",
					message: "Error some_error",
					data:    map[string]interface{}{"validationErrors": map[string]interface{}{"receiptErrors": "some_error"}},
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.Nil(t, err)
					},
				},
			},
		},

		{
			name: "error_internal_server_error",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrSomethingWentWrong
					},
				},
			},
			exp: tcExpected{
				code: http.StatusInternalServerError,
				err: &appErrorExp{
					code:    http.StatusInternalServerError,
					message: "something went wrong",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrSomethingWentWrong)
					},
				},
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
				body: `{ "type": "some_type", "raw_receipt": "blob", "package": "package", "subscription_id": "subscription_id" }`,
				svc: &mockTLV2Svc{
					FnExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						if orderID.String() != "facade00-0000-4000-a000-000000000000" {
							return model.Error("unexpected_order_id")
						}

						if req.Type != "some_type" {
							return model.Error("unexpected_type")
						}

						if req.Blob != "blob" {
							return model.Error("unexpected_blob")
						}

						if req.Package != "package" {
							return model.Error("unexpected_package")
						}

						if req.SubscriptionID != "subscription_id" {
							return model.Error("unexpected_subscription_id")
						}

						return nil
					},
				},
			},
			exp: tcExpected{
				code: http.StatusOK,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ch := handler.NewCred(tc.given.svc)

			r := httptest.NewRequest(http.MethodPost, "http://localhost", bytes.NewBufferString(tc.given.body))
			r = r.WithContext(tc.given.ctx)

			rw := httptest.NewRecorder()

			aerr := ch.ExtendLinkingLimitWithReceipt(rw, r)

			if aerr != nil {
				aerr.ServeHTTP(rw, r)
				must.Equal(t, tc.exp.err.code, aerr.Code)
				must.Equal(t, tc.exp.err.errCode, aerr.ErrorCode)
				must.Contains(t, tc.exp.err.message, aerr.Message)
				must.Equal(t, tc.exp.err.data, aerr.Data)
				tc.exp.err.mustCause(t, aerr.Cause)
			}

			should.Equal(t, tc.exp.code, rw.Code)
		})
	}
}

func TestCred_CanExtendLinkingLimitWithReceipt(t *testing.T) {
	type appErrorExp struct {
		code      int
		errCode   string
		message   string
		data      interface{}
		mustCause must.ErrorAssertionFunc
	}

	type tcGiven struct {
		ctx  context.Context
		body string
		svc  *mockTLV2Svc
	}

	type tcExpected struct {
		code int
		err  *appErrorExp
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "error_parse_order_id",
			given: tcGiven{
				ctx: context.Background(),
			},
			exp: tcExpected{
				code: http.StatusBadRequest,
				err: &appErrorExp{
					code:    http.StatusBadRequest,
					message: "Error validating request",
					data:    map[string]interface{}{"validationErrors": map[string]interface{}{"orderID": "uuid: incorrect UUID length: "}},
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.Nil(t, err)
					},
				},
			},
		},

		{
			name: "error_unmarshal_body",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
			},
			exp: tcExpected{
				code: http.StatusBadRequest,
				err: &appErrorExp{
					code:    http.StatusBadRequest,
					errCode: model.ExtensionCodeMalformedBody,
					message: "failed to parse request body",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.NotNil(t, err)
					},
				},
			},
		},

		{
			name: "error_client_ended_request",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnCanExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return context.Canceled
					},
				},
			},
			exp: tcExpected{
				code: model.StatusClientClosedConn,
				err: &appErrorExp{
					code:    model.StatusClientClosedConn,
					message: "client ended request",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, context.Canceled)
					},
				},
			},
		},

		{
			name: "error_request_timed_out",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnCanExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return context.DeadlineExceeded
					},
				},
			},
			exp: tcExpected{
				code: http.StatusGatewayTimeout,
				err: &appErrorExp{
					code:    http.StatusGatewayTimeout,
					message: "request timed out",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, context.DeadlineExceeded)
					},
				},
			},
		},

		{
			name: "error_order_id_does_not_match_receipt_order",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnCanExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrNoMatchOrderReceipt
					},
				},
			},
			exp: tcExpected{
				code: http.StatusConflict,
				err: &appErrorExp{
					code:    http.StatusConflict,
					message: "order_id does not match receipt order",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrNoMatchOrderReceipt)
					},
				},
			},
		},

		{
			name: "error_order_not_found",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnCanExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrOrderNotFound
					},
				},
			},
			exp: tcExpected{
				code: http.StatusNotFound,
				err: &appErrorExp{
					code:    http.StatusNotFound,
					errCode: model.ExtensionCodeOrderNotFound,
					message: "order not found",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrOrderNotFound)
					},
				},
			},
		},

		{
			name: "error_invalid_order_no_items",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnCanExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrInvalidOrderNoItems
					},
				},
			},
			exp: tcExpected{
				code: http.StatusNotFound,
				err: &appErrorExp{
					code:    http.StatusNotFound,
					errCode: model.ExtensionCodeOrderNotFound,
					message: "order not found",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrInvalidOrderNoItems)
					},
				},
			},
		},

		{
			name: "error_order_item_not_found",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnCanExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrOrderItemNotFound
					},
				},
			},
			exp: tcExpected{
				code: http.StatusNotFound,
				err: &appErrorExp{
					code:    http.StatusNotFound,
					errCode: model.ExtensionCodeOrderNotFound,
					message: "order not found",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrOrderItemNotFound)
					},
				},
			},
		},

		{
			name: "error_order_not_paid",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnCanExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrOrderNotPaid
					},
				},
			},
			exp: tcExpected{
				code: http.StatusPaymentRequired,
				err: &appErrorExp{
					code:    http.StatusPaymentRequired,
					errCode: model.ExtensionCodeOrderNotPaid,
					message: "order not paid",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrOrderNotPaid)
					},
				},
			},
		},

		{
			name: "error_credential_type_not_supported",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnCanExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrUnsupportedCredType
					},
				},
			},
			exp: tcExpected{
				code: http.StatusBadRequest,
				err: &appErrorExp{
					code:    http.StatusBadRequest,
					errCode: model.ExtensionCodeUnsupportedCredType,
					message: "credential type not supported",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrUnsupportedCredType)
					},
				},
			},
		},

		{
			name: "error_extension_invalid_limit",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnCanExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrExtensionInvalidLimit
					},
				},
			},
			exp: tcExpected{
				code: http.StatusUnprocessableEntity,
				err: &appErrorExp{
					code:    http.StatusUnprocessableEntity,
					errCode: model.ExtensionCodeInvalidLimitX,
					message: "extension new limit invalid",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrExtensionInvalidLimit)
					},
				},
			},
		},

		{
			name: "error_extension_not_supported",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnCanExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrNoExtensionPolicy
					},
				},
			},
			exp: tcExpected{
				code: http.StatusUnprocessableEntity,
				err: &appErrorExp{
					code:    http.StatusUnprocessableEntity,
					errCode: model.ExtensionNotSupported,
					message: "item does not support extension",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrNoExtensionPolicy)
					},
				},
			},
		},

		{
			name: "error_extension_rate_limited",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnCanExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrExtensionRateLimited
					},
				},
			},
			exp: tcExpected{
				code: http.StatusTooManyRequests,
				err: &appErrorExp{
					code:    http.StatusTooManyRequests,
					errCode: model.ExtensionCodeRateLimited,
					message: "extension rate limited",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrExtensionRateLimited)
					},
				},
			},
		},

		{
			name: "error_max_extension_per_item_reached",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnCanExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrExtensionMaxPerItem
					},
				},
			},
			exp: tcExpected{
				code: http.StatusUnprocessableEntity,
				err: &appErrorExp{
					code:    http.StatusUnprocessableEntity,
					errCode: model.ExtensionCodeMaxPerItem,
					message: "max extensions per item reached",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrExtensionMaxPerItem)
					},
				},
			},
		},

		{
			name: "error_extension_not_at_limit",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnCanExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrExtensionNotAtLimit
					},
				},
			},
			exp: tcExpected{
				code: http.StatusUnprocessableEntity,
				err: &appErrorExp{
					code:    http.StatusUnprocessableEntity,
					errCode: model.ExtensionCodeNotAtLimit,
					message: "not at limit; extension not needed",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrExtensionNotAtLimit)
					},
				},
			},
		},

		{
			name: "error_receipt_valid_error",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnCanExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return &model.ReceiptValidError{Err: model.Error("some_error")}
					},
				},
			},
			exp: tcExpected{
				code: http.StatusBadRequest,
				err: &appErrorExp{
					code:    http.StatusBadRequest,
					errCode: "validation_failed",
					message: "Error some_error",
					data:    map[string]interface{}{"validationErrors": map[string]interface{}{"receiptErrors": "some_error"}},
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.Nil(t, err)
					},
				},
			},
		},

		{
			name: "error_internal_server_error",
			given: tcGiven{
				ctx: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
					URLParams: chi.RouteParams{
						Keys:   []string{"orderID"},
						Values: []string{"facade00-0000-4000-a000-000000000000"},
					},
				}),
				body: "{}",
				svc: &mockTLV2Svc{
					FnCanExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						return model.ErrSomethingWentWrong
					},
				},
			},
			exp: tcExpected{
				code: http.StatusInternalServerError,
				err: &appErrorExp{
					code:    http.StatusInternalServerError,
					message: "something went wrong",
					mustCause: func(t must.TestingT, err error, i ...interface{}) {
						must.ErrorIs(t, err, model.ErrSomethingWentWrong)
					},
				},
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
				body: `{ "type": "some_type", "raw_receipt": "blob", "package": "package", "subscription_id": "subscription_id" }`,
				svc: &mockTLV2Svc{
					FnCanExtendLinkingLimitWithReceipt: func(ctx context.Context, orderID uuid.UUID, req model.ReceiptRequest) error {
						if orderID.String() != "facade00-0000-4000-a000-000000000000" {
							return model.Error("unexpected_order_id")
						}

						if req.Type != "some_type" {
							return model.Error("unexpected_type")
						}

						if req.Blob != "blob" {
							return model.Error("unexpected_blob")
						}

						if req.Package != "package" {
							return model.Error("unexpected_package")
						}

						if req.SubscriptionID != "subscription_id" {
							return model.Error("unexpected_subscription_id")
						}

						return nil
					},
				},
			},
			exp: tcExpected{
				code: http.StatusOK,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ch := handler.NewCred(tc.given.svc)

			r := httptest.NewRequest(http.MethodGet, "http://localhost", bytes.NewBufferString(tc.given.body))
			r = r.WithContext(tc.given.ctx)

			rw := httptest.NewRecorder()

			aerr := ch.CanExtendLinkingLimitWithReceipt(rw, r)

			if aerr != nil {
				aerr.ServeHTTP(rw, r)
				must.Equal(t, tc.exp.err.code, aerr.Code)
				must.Equal(t, tc.exp.err.errCode, aerr.ErrorCode)
				must.Contains(t, tc.exp.err.message, aerr.Message)
				must.Equal(t, tc.exp.err.data, aerr.Data)
				tc.exp.err.mustCause(t, aerr.Cause)
			}

			should.Equal(t, tc.exp.code, rw.Code)
		})
	}
}
