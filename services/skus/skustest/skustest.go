// Package skustest provides utilities for testing skus. Do not import this into non-test code.
package skustest

import (
	"context"
	"os"
	"strings"
	"testing"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/datastore"
	kafkautils "github.com/brave-intl/bat-go/libs/kafka"
	"github.com/jmoiron/sqlx"
	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
)

var tables = []string{"vote_drain", "api_keys", "transactions", "signing_order_request_outbox",
	"time_limited_v2_order_creds", "order_creds", "order_cred_issuers", "order_items", "orders"}

// Migrate - perform a migration for skus
func Migrate(t *testing.T) {
	postgres, err := datastore.NewPostgres("", false, "skus_db")
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

// SetupKafka is a test helper to setup kafka brokers and topic
func SetupKafka(ctx context.Context, t *testing.T, topics ...string) context.Context {
	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	ctx = context.WithValue(ctx, appctx.KafkaBrokersCTXKey, kafkaBrokers)

	dialer, _, err := kafkautils.TLSDialer()
	assert.NoError(t, err)

	for _, topic := range topics {
		conn, err := dialer.DialLeader(ctx, "tcp", strings.Split(kafkaBrokers, ",")[0], topic, 0)
		assert.NoError(t, err)

		err = conn.CreateTopics(kafka.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1})
		assert.NoError(t, err)
	}

	return ctx
}
