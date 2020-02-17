package mint

import (
	"context"
	"time"

	"github.com/brave-intl/bat-go/datastore/grantserver"
	uuid "github.com/satori/go.uuid"
)

// ClobberedCreds holds data of claims that have been clobbered and when they were first reported
type ClobberedCreds struct {
	ID        uuid.UUID `db:"id"`
	CreatedAt time.Time `db:"created_at"`
}

// Action holds minimal necessary data for an action
type Action struct {
	ID        uuid.UUID  `db:"id"`
	Value     string     `db:"value"`
	ActionID  string     `db:"action_id"`
	ModalID   uuid.UUID  `db:"modal_id"`
	CreatedAt *time.Time `db:"created_at"`
}

// Datastore abstracts over the underlying datastore
type Datastore interface {
	// ActivatePromotion marks a particular promotion as active
	UpsertAction(ctx context.Context, action Action) error
	// RemoveModalActions removes all modal actions
	RemoveModalActions(ctx context.Context, modalID uuid.UUID) error
	// GetModalActions retrieves modal actions
	GetModalActions(ctx context.Context, modalID uuid.UUID) (*[]Action, error)
}

// ReadOnlyDatastore includes all database methods that can be made with a read only db connection
type ReadOnlyDatastore interface{}

// Postgres is a Datastore wrapper around a postgres database
type Postgres struct {
	grantserver.Postgres
}

// NewPostgres creates a new Postgres Datastore
func NewPostgres(databaseURL string, performMigration bool, dbStatsPrefix ...string) (*Postgres, error) {
	pg, err := grantserver.NewPostgres(databaseURL, performMigration, dbStatsPrefix...)
	if pg != nil {
		return &Postgres{*pg}, err
	}
	return nil, err
}

// UpsertAction upserts a modal action
func (pg *Postgres) UpsertAction(ctx context.Context, action Action) error {
	id := uuid.NewV5(action.ModalID, action.ActionID)
	_, err := pg.DB.ExecContext(ctx, `insert into mint_actions(id, value, action_id, modal_id)
values($1, $2, $3, $4) on conflict (id)
do update set value = $2`, id, action.Value, action.ActionID, action.ModalID)
	return err
}

// RemoveModalActions removes all actions matching a given modal id
func (pg *Postgres) RemoveModalActions(ctx context.Context, modalID uuid.UUID) error {
	_, err := pg.DB.ExecContext(ctx, `
delete from mint_actions where modal_id = $1`, modalID)
	return err
}

// GetModalActions retrieves all actions given a modal id
func (pg *Postgres) GetModalActions(ctx context.Context, modalID uuid.UUID) (*[]Action, error) {
	var actions []Action
	err := pg.DB.Select(&actions, `select * from mint_actions where modal_id = $1`, modalID)
	return &actions, err
}
