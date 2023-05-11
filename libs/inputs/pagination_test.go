package inputs

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

type transaction struct {
	ID                    uuid.UUID       `json:"id" db:"id"`
	OrderID               uuid.UUID       `json:"orderId" db:"order_id"`
	CreatedAt             time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt             time.Time       `json:"updatedAt" db:"updated_at"`
	ExternalTransactionID string          `json:"external_transaction_id" db:"external_transaction_id"`
	Status                string          `json:"status" db:"status"`
	Currency              string          `json:"currency" db:"currency"`
	Kind                  string          `json:"kind" db:"kind"`
	Amount                decimal.Decimal `json:"amount" db:"amount"`
}

func TestNewPagination(t *testing.T) {
	var ctx = context.Background()
	ctx, p, err := NewPagination(ctx, "?order=createdAt.asc&order=orderId.desc", new(transaction))
	if err != nil {
		t.Error("failed to create a new pagination: ", err)
		return
	}
	var orderBy = p.GetOrderBy(ctx)
	if !strings.Contains(orderBy, "created_at  ASC") ||
		!strings.Contains(orderBy, "order_id  DESC") {
		t.Logf("P: %+v\n", p.GetOrderBy(ctx))
		t.Error("order by statement not what was expected")
		return
	}
	// test some invalid pagination
	_, _, err = NewPagination(context.Background(), "?order=created_at.asc&order=orderId.BLAH", new(transaction))
	if err == nil {
		t.Error("new pagination should have failed", err)
		return
	}
	fmt.Println(err.Error())
	if !strings.Contains(err.Error(), "created_at") || !strings.Contains(err.Error(), "BLAH") {
		t.Error("new pagination should have complained about created_at and BLAH")
		return
	}
}
