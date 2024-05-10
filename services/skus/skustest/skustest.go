// Package skustest provides utilities for testing skus. Do not import this into non-test code.
package skustest

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/segmentio/kafka-go"
	must "github.com/stretchr/testify/require"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/datastore"
	kafkautils "github.com/brave-intl/bat-go/libs/kafka"
)

func Migrate(t *testing.T) {
	pg, err := datastore.NewPostgres("", false, "skus_db")
	must.NoError(t, err)

	m, err := pg.NewMigrate()
	must.NoError(t, err)

	if ver, dirty, _ := m.Version(); dirty {
		must.NoError(t, m.Force(int(ver)))
	}

	must.NoError(t, pg.Migrate())
}

func CleanDB(t *testing.T, dbi *sqlx.DB) {
	_, err := dbi.Exec("TRUNCATE TABLE vote_drain, api_keys, transactions, signing_order_request_outbox, time_limited_v2_order_creds, order_items, order_creds, order_cred_issuers, orders")
	must.Equal(t, nil, err)
}

func SetupKafka(ctx context.Context, t *testing.T, topics ...string) context.Context {
	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	ctx = context.WithValue(ctx, appctx.KafkaBrokersCTXKey, kafkaBrokers)

	dialer, _, err := kafkautils.TLSDialer()
	must.NoError(t, err)

	for _, topic := range topics {
		conn, err := dialer.DialLeader(ctx, "tcp", strings.Split(kafkaBrokers, ",")[0], topic, 0)
		must.NoError(t, err)

		must.NoError(t, conn.CreateTopics(kafka.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1}))
	}

	return ctx
}
