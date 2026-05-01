//go:build integration

package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/ptr"
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
					SKUVnt:         "sku_vnt_01_01",
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
					SKUVnt:         "sku_vnt_01_01",
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
					SKU:                       "sku_02_01",
					SKUVnt:                    "sku_vnt_02_01",
					Quantity:                  2,
					Price:                     mustDecimalFromString("3"),
					Currency:                  "USD",
					Subtotal:                  mustDecimalFromString("6"),
					CredentialType:            "something",
					MaxActiveBatchesTLV2Creds: ptr.To(10),
				},

				{
					SKU:                       "sku_02_02",
					SKUVnt:                    "sku_vnt_02_02",
					Quantity:                  3,
					Price:                     mustDecimalFromString("4"),
					Currency:                  "USD",
					Subtotal:                  mustDecimalFromString("12"),
					CredentialType:            "something",
					MaxActiveBatchesTLV2Creds: ptr.To(10),
				},
			},

			exp: []model.OrderItem{
				{
					SKU:                       "sku_02_01",
					SKUVnt:                    "sku_vnt_02_01",
					Quantity:                  2,
					Price:                     mustDecimalFromString("3"),
					Currency:                  "USD",
					Subtotal:                  mustDecimalFromString("6"),
					CredentialType:            "something",
					MaxActiveBatchesTLV2Creds: ptr.To(10),
				},

				{
					SKU:                       "sku_02_02",
					SKUVnt:                    "sku_vnt_02_02",
					Quantity:                  3,
					Price:                     mustDecimalFromString("4"),
					Currency:                  "USD",
					Subtotal:                  mustDecimalFromString("12"),
					CredentialType:            "something",
					MaxActiveBatchesTLV2Creds: ptr.To(10),
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
				should.Equal(t, tc.exp[j].SKUVnt, actual[j].SKUVnt)
				should.Equal(t, tc.exp[j].Quantity, actual[j].Quantity)
				should.Equal(t, tc.exp[j].Price.String(), actual[j].Price.String())
				should.Equal(t, tc.exp[j].Currency, actual[j].Currency)
				should.Equal(t, tc.exp[j].Subtotal.String(), actual[j].Subtotal.String())
				should.Equal(t, tc.exp[j].CredentialType, actual[j].CredentialType)
				should.Equal(t, tc.exp[j].MaxActiveBatchesTLV2Creds, actual[j].MaxActiveBatchesTLV2Creds)
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

func TestOrderItem_ApplyExtensionCAS(t *testing.T) {
	dbi, err := setupDBI()
	must.Equal(t, nil, err)

	defer func() {
		_, _ = dbi.Exec("TRUNCATE TABLE order_items, orders;")
	}()

	orepo := repository.NewOrder()
	iorepo := repository.NewOrderItem()

	// Helper: create an order with a single TLV2 item and return the item's id.
	insertItem := func(ctx context.Context, dbi sqlx.ExtContext) uuid.UUID {
		ord, err := createOrderForTest(ctx, dbi, orepo)
		must.NoError(t, err)

		items := []model.OrderItem{
			{
				OrderID:                   ord.ID,
				SKU:                       "sku_cas",
				SKUVnt:                    "sku_vnt_cas",
				Quantity:                  1,
				Price:                     mustDecimalFromString("1"),
				Currency:                  "USD",
				Subtotal:                  mustDecimalFromString("1"),
				CredentialType:            "time-limited-v2",
				MaxActiveBatchesTLV2Creds: ptr.To(10),
			},
		}

		inserted, err := iorepo.InsertMany(ctx, dbi, items...)
		must.NoError(t, err)
		must.Equal(t, 1, len(inserted))

		return inserted[0].ID
	}

	t.Run("first_extension_succeeds_with_nil_token", func(t *testing.T) {
		ctx := context.TODO()

		tx, err := dbi.BeginTxx(ctx, nil)
		must.NoError(t, err)
		t.Cleanup(func() { _ = tx.Rollback() })

		itemID := insertItem(ctx, tx)

		err = iorepo.ApplyExtensionCAS(ctx, tx, itemID, nil, 13)
		must.NoError(t, err)

		// Verify side-effects.
		var got struct {
			MaxActiveBatchesTLV2Creds *int       `db:"max_active_batches_tlv2_creds"`
			NumSelfExtensions         int        `db:"num_self_extensions"`
			LastSelfExtensionAt       *time.Time `db:"last_self_extension_at"`
		}
		err = tx.QueryRowxContext(ctx, `SELECT max_active_batches_tlv2_creds, num_self_extensions, last_self_extension_at FROM order_items WHERE id = $1`, itemID).StructScan(&got)
		must.NoError(t, err)
		should.Equal(t, 13, *got.MaxActiveBatchesTLV2Creds)
		should.Equal(t, 1, got.NumSelfExtensions)
		must.NotNil(t, got.LastSelfExtensionAt)
	})

	t.Run("conflict_when_token_is_stale", func(t *testing.T) {
		ctx := context.TODO()

		tx, err := dbi.BeginTxx(ctx, nil)
		must.NoError(t, err)
		t.Cleanup(func() { _ = tx.Rollback() })

		itemID := insertItem(ctx, tx)

		// First extension succeeds.
		must.NoError(t, iorepo.ApplyExtensionCAS(ctx, tx, itemID, nil, 13))

		// Second extension with a stale (nil) token fails — row was extended.
		err = iorepo.ApplyExtensionCAS(ctx, tx, itemID, nil, 16)
		must.ErrorIs(t, err, model.ErrExtensionConflict)
	})

	t.Run("subsequent_extension_succeeds_with_matching_token", func(t *testing.T) {
		ctx := context.TODO()

		tx, err := dbi.BeginTxx(ctx, nil)
		must.NoError(t, err)
		t.Cleanup(func() { _ = tx.Rollback() })

		itemID := insertItem(ctx, tx)

		must.NoError(t, iorepo.ApplyExtensionCAS(ctx, tx, itemID, nil, 13))

		var token *time.Time
		err = tx.QueryRowxContext(ctx, `SELECT last_self_extension_at FROM order_items WHERE id = $1`, itemID).Scan(&token)
		must.NoError(t, err)
		must.NotNil(t, token)

		err = iorepo.ApplyExtensionCAS(ctx, tx, itemID, token, 16)
		must.NoError(t, err)

		var got struct {
			MaxActiveBatchesTLV2Creds *int `db:"max_active_batches_tlv2_creds"`
			NumSelfExtensions         int  `db:"num_self_extensions"`
		}
		err = tx.QueryRowxContext(ctx, `SELECT max_active_batches_tlv2_creds, num_self_extensions FROM order_items WHERE id = $1`, itemID).StructScan(&got)
		must.NoError(t, err)
		should.Equal(t, 16, *got.MaxActiveBatchesTLV2Creds)
		should.Equal(t, 2, got.NumSelfExtensions)
	})

	t.Run("check_violation_above_ceiling", func(t *testing.T) {
		ctx := context.TODO()

		tx, err := dbi.BeginTxx(ctx, nil)
		must.NoError(t, err)
		t.Cleanup(func() { _ = tx.Rollback() })

		itemID := insertItem(ctx, tx)

		err = iorepo.ApplyExtensionCAS(ctx, tx, itemID, nil, 1001)
		must.Error(t, err)

		var pqErr *pq.Error
		must.True(t, errors.As(err, &pqErr), "expected *pq.Error, got %T (%v)", err, err)
		should.Equal(t, pq.ErrorCode("23514"), pqErr.Code)
	})
}
