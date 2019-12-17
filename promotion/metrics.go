package promotion

import (
	"time"

	uuid "github.com/satori/go.uuid"
)

// DailyUniqueMetricCounts holds a single metrics data
type DailyUniqueMetricCounts struct {
	Date         time.Time `db:"date"`
	ActivityType string    `db:"activity_type"`
	Wallets      int       `db:"wallets"`
}

// CountActiveWallet count the active user
func (pg *Postgres) CountActiveWallet(walletID uuid.UUID) error {
	statement := `
	INSERT INTO daily_unique_metrics(date, activity_type, wallets)
	VALUES (NOW()::DATE, $2, hll_add(hll_empty(), hll_hash_text($1)))
	ON CONFLICT (date, activity_type)
		DO UPDATE
		SET
			wallets = hll_add(daily_unique_metrics.wallets, hll_hash_text($1))
		WHERE
				daily_unique_metrics.date = NOW()::DATE
		AND daily_unique_metrics.activity_type = $2`
	_, err := pg.DB.Exec(
		statement,
		walletID.String(),
		"active",
	)
	return err
}
