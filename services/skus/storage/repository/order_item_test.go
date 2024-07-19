//go:build integration

package repository_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/libs/datastore"

	"github.com/brave-intl/bat-go/services/skus/model"
	"github.com/brave-intl/bat-go/services/skus/storage/repository"
)

func TestOrderItem_InsertMany(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE order_items, orders;")
	}()

	type testCase struct {
		name  string
		given []model.OrderItem
		exp   []model.OrderItem
	}

	tests := []testCase{
		{
			name: "empty_input",
			exp:  []model.OrderItem{},
		},

		{
			name: "one_item",
			given: []model.OrderItem{
				{
					SKU:            "sku_01_01",
					Quantity:       1,
					Price:          mustDecimalFromString("2"),
					Currency:       "USD",
					Subtotal:       mustDecimalFromString("2"),
					CredentialType: "something",
				},
			},

			exp: []model.OrderItem{
				{
					SKU:            "sku_01_01",
					Quantity:       1,
					Price:          mustDecimalFromString("2"),
					Currency:       "USD",
					Subtotal:       mustDecimalFromString("2"),
					CredentialType: "something",
				},
			},
		},

		{
			name: "two_items",
			given: []model.OrderItem{
				{
					SKU:            "sku_02_01",
					Quantity:       2,
					Price:          mustDecimalFromString("3"),
					Currency:       "USD",
					Subtotal:       mustDecimalFromString("6"),
					CredentialType: "something",
				},

				{
					SKU:            "sku_02_02",
					Quantity:       3,
					Price:          mustDecimalFromString("4"),
					Currency:       "USD",
					Subtotal:       mustDecimalFromString("12"),
					CredentialType: "something",
				},
			},

			exp: []model.OrderItem{
				{
					SKU:            "sku_02_01",
					Quantity:       2,
					Price:          mustDecimalFromString("3"),
					Currency:       "USD",
					Subtotal:       mustDecimalFromString("6"),
					CredentialType: "something",
				},

				{
					SKU:            "sku_02_02",
					Quantity:       3,
					Price:          mustDecimalFromString("4"),
					Currency:       "USD",
					Subtotal:       mustDecimalFromString("12"),
					CredentialType: "something",
				},
			},
		},
	}

	orepo := repository.NewOrder()
	iorepo := repository.NewOrderItem()

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.TODO()

			tx, err := dbi.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadUncommitted})
			must.Equal(t, nil, err)

			t.Cleanup(func() { _ = tx.Rollback() })

			order, err := createOrderForTest(ctx, tx, orepo)
			must.Equal(t, nil, err)

			model.OrderItemList(tc.given).SetOrderID(order.ID)

			actual, err := iorepo.InsertMany(ctx, tx, tc.given...)
			must.Equal(t, nil, err)

			must.Equal(t, len(tc.exp), len(actual))

			// Check each item manually as ids are generated.
			for j := range tc.exp {
				should.NotEqual(t, uuid.Nil, actual[j].ID)
				should.Equal(t, order.ID, actual[j].OrderID)
				should.Equal(t, tc.exp[j].SKU, actual[j].SKU)
				should.Equal(t, tc.exp[j].Quantity, actual[j].Quantity)
				should.Equal(t, tc.exp[j].Price.String(), actual[j].Price.String())
				should.Equal(t, tc.exp[j].Currency, actual[j].Currency)
				should.Equal(t, tc.exp[j].Subtotal.String(), actual[j].Subtotal.String())
				should.Equal(t, tc.exp[j].CredentialType, actual[j].CredentialType)
			}
		})
	}
}

func setupDBI() (*sqlx.DB, error) {
	pg, err := datastore.NewPostgres("", false, "")
	if err != nil {
		return nil, err
	}

	mg, err := pg.NewMigrate()
	if err != nil {
		return nil, err
	}

	if ver, dirty, _ := mg.Version(); dirty {
		if err := mg.Force(int(ver)); err != nil {
			return nil, err
		}
	}

	if err := pg.Migrate(); err != nil {
		return nil, err
	}

	return pg.RawDB(), nil
}

type orderCreator interface {
	Create(ctx context.Context, dbi sqlx.QueryerContext, req *model.OrderNew) (*model.Order, error)
}

func createOrderForTest(ctx context.Context, dbi sqlx.QueryerContext, repo orderCreator) (*model.Order, error) {
	price, err := decimal.NewFromString("187")
	if err != nil {
		return nil, err
	}

	req := &model.OrderNew{
		MerchantID: "brave.com",
		Currency:   "USD",
		Status:     "pending",
		Location: sql.NullString{
			Valid:  true,
			String: "somelocation",
		},
		TotalPrice:            price,
		AllowedPaymentMethods: pq.StringArray{"stripe"},
	}

	result, err := repo.Create(ctx, dbi, req)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func mustDecimalFromString(v string) decimal.Decimal {
	result, err := decimal.NewFromString(v)
	if err != nil {
		panic(err)
	}

	return result
}
