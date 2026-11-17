package skus

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/libs/datastore"

	"github.com/brave-intl/bat-go/services/skus/model"
	"github.com/brave-intl/bat-go/services/skus/storage/repository"
)

// fakeReceiptValidator stubs the vendor receipt validation seam.
type fakeReceiptValidator struct {
	rcpt model.ReceiptData
	err  error
}

func (f *fakeReceiptValidator) validateApple(ctx context.Context, req model.ReceiptRequest) (model.ReceiptData, error) {
	return f.rcpt, f.err
}

func (f *fakeReceiptValidator) validateGoogle(ctx context.Context, req model.ReceiptRequest) (model.ReceiptData, error) {
	return f.rcpt, f.err
}

func (f *fakeReceiptValidator) fetchSubPlayStore(ctx context.Context, pkgName, subID, token string) (*playStoreSubPurchase, error) {
	return nil, errors.New("not implemented")
}

// TestCreateOrderWithReceiptSerialization pins the serialization contract of
// createOrderWithReceipt: the existence check and creation for a receipt run
// under a per-receipt advisory lock, so concurrent submissions of the same
// receipt cannot create duplicate paid orders (H1 4111604 class).
func TestCreateOrderWithReceiptSerialization(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	must.NoError(t, err)
	defer func() { _ = mockDB.Close() }()

	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	orderID := uuid.NewV4()

	orderRow := sqlmock.NewRows([]string{
		"id", "created_at", "currency", "updated_at", "total_price",
		"merchant_id", "location", "status", "allowed_payment_methods",
		"metadata", "valid_for", "last_paid_at", "expires_at", "trial_days",
	}).AddRow(
		orderID.String(), time.Now(), "usd", time.Now(), "5.00",
		"brave.com", nil, "paid", []byte("{stripe}"),
		[]byte(`{"externalID":"ext-1"}`), nil, time.Now(), expiresAt, nil,
	)

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT (.+) FROM orders WHERE metadata->>'externalID' = \$1`).
		WillReturnRows(orderRow)
	// The found order's paid-status bookkeeping runs on the same transaction.
	mock.ExpectExec("UPDATE orders SET status").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE orders SET updated_at").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE orders SET last_paid_at").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO order_payment_history").
		WillReturnResult(sqlmock.NewResult(1, 1))
	// The serialization transaction commits, releasing the advisory lock.
	mock.ExpectCommit()

	svc := &Service{
		Datastore: &Postgres{
			Postgres: datastore.Postgres{DB: sqlx.NewDb(mockDB, "sqlmock")},
		},
		orderRepo:   repository.NewOrder(),
		payHistRepo: repository.NewOrderPayHistory(),
		vendorReceiptValid: &fakeReceiptValidator{
			rcpt: model.ReceiptData{
				ExtID:     "ext-1",
				ProductID: "brave.leo.monthly",
				ExpiresAt: expiresAt,
			},
		},
	}

	ord, err := svc.createOrderWithReceipt(context.Background(), model.ReceiptRequest{
		Type: model.VendorApple,
		Blob: "raw-receipt",
	})
	must.ErrorIs(t, err, model.ErrOrderExistsForReceipt)
	must.NotNil(t, ord)
	must.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateOrderWithReceiptCreate pins the creation branch of
// createOrderWithReceipt: the advisory lock is acquired before the existence
// check, the order and its payment bookkeeping are created, and the
// serialization transaction commits only after the order is durable.
func TestCreateOrderWithReceiptCreate(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	must.NoError(t, err)
	defer func() { _ = mockDB.Close() }()

	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	orderID := uuid.NewV4()
	itemID := uuid.NewV4()

	orderRow := sqlmock.NewRows([]string{
		"id", "created_at", "currency", "updated_at", "total_price",
		"merchant_id", "location", "status", "allowed_payment_methods", "valid_for",
	}).AddRow(
		orderID.String(), time.Now(), "usd", time.Now(), "14.99",
		"brave.com", nil, "paid", []byte("{stripe}"), nil,
	)

	itemRow := sqlmock.NewRows([]string{
		"id", "order_id", "sku", "sku_variant", "created_at", "updated_at",
		"currency", "quantity", "price", "location", "description",
		"credential_type", "subtotal", "metadata", "valid_for",
		"max_active_batches_tlv2_creds", "num_self_extensions", "last_self_extension_at",
	}).AddRow(
		itemID.String(), orderID.String(), "brave-leo-premium", "brave-leo-premium",
		time.Now(), time.Now(), "usd", 1, "14.99", nil, "Premium access to Leo",
		"time-limited", "14.99", []byte("{}"), nil, nil, 0, nil,
	)

	// Serialization transaction: advisory lock, then the existence check.
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT (.+) FROM orders WHERE metadata->>'externalID' = \$1`).
		WillReturnError(sql.ErrNoRows)

	// Everything runs on the same transaction under the advisory lock: the item
	// credential type is time-limited, so no issuer creation happens.
	mock.ExpectQuery("INSERT INTO orders").WillReturnRows(orderRow)
	mock.ExpectQuery("INSERT INTO order_items").WillReturnRows(itemRow)
	mock.ExpectExec("UPDATE orders").WillReturnResult(sqlmock.NewResult(1, 1))

	// Paid status, expiry and payment history.
	mock.ExpectExec("UPDATE orders SET status").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE orders SET updated_at").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE orders SET last_paid_at").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO order_payment_history").WillReturnResult(sqlmock.NewResult(1, 1))

	// Mobile metadata: externalID, paymentProcessor and vendor keys.
	mock.ExpectExec("UPDATE orders").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE orders").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE orders").WillReturnResult(sqlmock.NewResult(1, 1))

	// The serialization transaction commits last, releasing the advisory lock
	// only after the order is durable.
	mock.ExpectCommit()

	svc := &Service{
		Datastore: &Postgres{
			Postgres: datastore.Postgres{DB: sqlx.NewDb(mockDB, "sqlmock")},
		},
		orderRepo:     repository.NewOrder(),
		orderItemRepo: repository.NewOrderItem(),
		payHistRepo:   repository.NewOrderPayHistory(),
		vendorReceiptValid: &fakeReceiptValidator{
			rcpt: model.ReceiptData{
				Type:      model.VendorGoogle,
				ExtID:     "ext-2",
				ProductID: "brave.leo.monthly",
				ExpiresAt: expiresAt,
			},
		},
		newItemReqSet: map[string]model.OrderItemRequestNew{
			"brave-leo-premium": {
				Quantity:                1,
				SKU:                     "brave-leo-premium",
				SKUVnt:                  "brave-leo-premium",
				Description:             "Premium access to Leo",
				CredentialType:          "time-limited",
				CredentialValidDuration: "P1M",
				Price:                   decimal.RequireFromString("14.99"),
			},
		},
		payProcCfg: newPaymentProcessorConfig("development"),
	}

	ord, err := svc.createOrderWithReceipt(context.Background(), model.ReceiptRequest{
		Type: model.VendorGoogle,
		Blob: "raw-receipt",
	})
	must.NoError(t, err)
	must.NotNil(t, ord)
	must.Equal(t, orderID.String(), ord.ID.String())
	must.NoError(t, mock.ExpectationsWereMet())
}
