package service

import (
	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/brave-intl/bat-go/wallet"
	uuid "github.com/satori/go.uuid"
)

// Datastore abstracts over the underlying datastore
type Datastore interface {
	// UpsertWallet upserts the given wallet
	UpsertWallet(wallet *wallet.Info) error
	// GetWallet by ID
	GetWallet(id uuid.UUID) (*wallet.Info, error)
}

// ReadOnlyDatastore includes all database methods that can be made with a read only db connection
type ReadOnlyDatastore interface {
	// GetWallet by ID
	GetWallet(id uuid.UUID) (*wallet.Info, error)
}

// Postgres is a Datastore wrapper around a postgres database
type Postgres struct {
	grantserver.Postgres
}

// NewPostgres creates a new Postgres Datastore
func NewPostgres(databaseURL string, performMigration bool, dbStatsPrefix ...string) (Datastore, error) {
	pg, err := grantserver.NewPostgres(databaseURL, performMigration, dbStatsPrefix...)
	if pg != nil {
		return &DatastoreWithPrometheus{
			base: &Postgres{*pg}, instanceName: "wallet_datastore",
		}, err
	}
	return nil, err
}

// NewROPostgres creates a new Postgres RO Datastore
func NewROPostgres(databaseURL string, performMigration bool, dbStatsPrefix ...string) (ReadOnlyDatastore, error) {
	pg, err := grantserver.NewPostgres(databaseURL, performMigration, dbStatsPrefix...)
	if pg != nil {
		return &ReadOnlyDatastoreWithPrometheus{
			base: &Postgres{*pg}, instanceName: "wallet_ro_datastore",
		}, err
	}
	return nil, err
}
