package subscriptions

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type DataStore interface {
	createRoom(r Room) error
	increaseMau()
}

type Postgres struct {
	*sqlx.DB
}

type Stat struct {
	Name      string       `db:"name"`
	CreatedAt sql.NullTime `db:"created_at"`
	Count     int          `db:"count"`
}

func (pg *Postgres) createRoom(r Room) error {
	createdRoom := Room{}
	err := pg.Get(&createdRoom, `
		INSERT INTO rooms (name, tier)
		VALUES ($1, $2)
		RETURNING *
	`, r.Name, r.Tier)
	return err
}

func (pg *Postgres) increaseMau() {
	repoStat := Stat{}
	year, month, _ := time.Now().Date()
	name := fmt.Sprintf("mau_%d_%d", year, month)
	err := pg.Get(&repoStat, `
		INSERT INTO stats(name, count, created_at)
		VALUES($1, $2, NOW())
		ON CONFLICT(name) DO UPDATE SET count = stats.count + 1
		RETURNING *;
	`, name, 1)
	if err != nil {
		fmt.Println(err.Error())
	}
}
