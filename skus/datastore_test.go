package skus

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
)

func TestGetPagedMerchantTransactions(t *testing.T) {
	ctx := context.Background()
	// setup mock DB we will inject into our pg
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Errorf("failed to create a sql mock: %s", err)
	}
	defer func() {
		if err := mockDB.Close(); err != nil {
			if !strings.Contains(err.Error(), "all expectations were already fulfilled") {
				t.Errorf("failed to close the mock database: %s", err)
			}
		}
	}()
	// inject our mock db into our postgres
	pg := &Postgres{Postgres: grantserver.Postgres{DB: sqlx.NewDb(mockDB, "sqlmock")}}

	// setup inputs
	merchantID := uuid.NewV4()
	ctx, pagination, err := inputs.NewPagination(ctx, "/?page=2&items=50&order=id.asc&order=createdAt.desc", new(Transaction))
	if err != nil {
		t.Errorf("failed to create pagination: %s\n", err)
	}

	// setup expected mocks
	countRows := sqlmock.NewRows([]string{"total"}).AddRow(3)
	mock.ExpectQuery(`
			SELECT (.+) as total
			FROM transactions as t
				INNER JOIN orders as o ON o.id = t.order_id
			WHERE (.+)`).WithArgs(merchantID).WillReturnRows(countRows)

	transactionUUIDs := []uuid.UUID{uuid.NewV4(), uuid.NewV4(), uuid.NewV4()}
	orderUUIDs := []uuid.UUID{uuid.NewV4(), uuid.NewV4(), uuid.NewV4()}
	createdAt := []time.Time{time.Now(), time.Now().Add(time.Second * 5), time.Now().Add(time.Second * 10)}

	getRows := sqlmock.NewRows(
		[]string{"id", "order_id", "created_at", "updated_at",
			"external_transaction_id", "status", "currency", "kind", "amount"}).
		AddRow(transactionUUIDs[0], orderUUIDs[0], createdAt[0], createdAt[0], "", "pending", "BAT", "subscription", 10).
		AddRow(transactionUUIDs[1], orderUUIDs[1], createdAt[1], createdAt[1], "", "pending", "BAT", "subscription", 10).
		AddRow(transactionUUIDs[2], orderUUIDs[2], createdAt[2], createdAt[2], "", "pending", "BAT", "subscription", 10)

	mock.ExpectQuery(`
			SELECT (.+)
			FROM transactions as t
				INNER JOIN orders as o ON o.id = t.order_id
			WHERE o.merchant_id = (.+)
			 ORDER BY (.+) OFFSET (.+) FETCH NEXT (.+)`).WithArgs(merchantID).
		WillReturnRows(getRows)

	// call function under test with inputs
	transactions, c, err := pg.GetPagedMerchantTransactions(ctx, merchantID, pagination)

	// test assertions
	if err != nil {
		t.Errorf("failed to get paged merchant transactions: %s\n", err)
	}
	if len(*transactions) != 3 {
		t.Errorf("should have seen 3 transactions: %+v\n", transactions)
	}
	if c != 3 {
		t.Errorf("should have total count of 3 transactions: %d\n", c)
	}
}
