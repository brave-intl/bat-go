// Package wallettest provides utilities for testing wallets. Do not import this into non-test code.
package wallettest

import (
	"github.com/brave-intl/bat-go/libs/datastore"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"testing"
)

var tables = []string{"claim_creds", "claims", "wallets", "issuers", "promotions"}

// Migrate - perform a migration for skus
func Migrate(t *testing.T) {
	postgres, err := datastore.NewPostgres("", false, "wallet_db")
	assert.NoError(t, err)

	migrate, err := postgres.NewMigrate()
	assert.NoError(t, err)

	version, dirty, _ := migrate.Version()
	if dirty {
		assert.NoError(t, migrate.Force(int(version)))
	}

	if version > 0 {
		assert.NoError(t, migrate.Down())
	}

	err = postgres.Migrate()
	assert.NoError(t, err)
}

// CleanDB - clean up the test db fixtures
func CleanDB(t *testing.T, datastore *sqlx.DB) {
	for _, table := range tables {
		_, err := datastore.Exec("delete from " + table)
		assert.NoError(t, err)
	}
}
