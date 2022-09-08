//go:build integration

package promotion

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/inputs"
	kafkautils "github.com/brave-intl/bat-go/libs/kafka"
	"github.com/brave-intl/bat-go/libs/test"
	"github.com/brave-intl/bat-go/services/wallet"
	uuid "github.com/satori/go.uuid"
	"github.com/segmentio/kafka-go"

	// re-using viper bind-env for wallet env variables
	_ "github.com/brave-intl/bat-go/services/wallet/cmd"
	"github.com/stretchr/testify/suite"
)

type ServiceTestSuite struct {
	suite.Suite
}

func (suite *ServiceTestSuite) SetupSuite() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	m, err := pg.NewMigrate()
	suite.Require().NoError(err, "Failed to create migrate instance")

	ver, dirty, _ := m.Version()
	if dirty {
		suite.Require().NoError(m.Force(int(ver)))
	}
	if ver > 0 {
		suite.Require().NoError(m.Down(), "Failed to migrate down cleanly")
	}

	suite.Require().NoError(pg.Migrate(), "Failed to fully migrate")
}

func (suite *ServiceTestSuite) SetupTest() {
	suite.CleanDB()
}

func (suite *ServiceTestSuite) TearDownTest() {
	suite.CleanDB()
}

func (suite *ServiceTestSuite) CleanDB() {
	tables := []string{"claim_creds", "claims", "wallets", "issuers", "promotions"}

	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	for _, table := range tables {
		_, err = pg.RawDB().Exec("delete from " + table)
		suite.Require().NoError(err, "Failed to get clean table")
	}
}

func TestServiceTestSuite(t *testing.T) {
	suite.Run(t, new(ServiceTestSuite))
}

func (suite *ServiceTestSuite) createService() (*Service, context.Context) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, appctx.ParametersMergeBucketCTXKey, test.RandomString())
	ctx = context.WithValue(ctx, appctx.DisabledWalletGeoCountriesCTXKey, test.RandomString())

	ctx, walletService := wallet.SetupService(ctx)
	promotionDB, promotionRODB, err := NewPostgres()
	suite.Require().NoError(err, "unable connect to promotion db")
	s, err := InitService(
		ctx,
		promotionDB,
		promotionRODB,
		walletService,
	)
	suite.Require().NoError(err)

	return s, ctx
}

func (suite *ServiceTestSuite) TestGetAvailablePromotions() {
	var nilPromotions *[]Promotion
	noPromotions := []Promotion{}
	service, ctx := suite.createService()

	walletID := new(inputs.ID)
	id := walletID.UUID()

	promotions, err := service.GetAvailablePromotions(ctx, id, "", true)
	suite.Require().NoError(err)
	suite.Require().Equal(&noPromotions, promotions)

	err = inputs.DecodeAndValidateString(
		ctx,
		walletID,
		"00000000-0000-0000-0000-000000000000",
	)
	suite.Require().NoError(err)

	id = walletID.UUID()
	promotions, err = service.GetAvailablePromotions(ctx, id, "", true)
	suite.Require().NoError(err)
	suite.Require().Equal(nilPromotions, promotions)
}

func (suite *ServiceTestSuite) TestInitAndRunNextDrainRetryJob() {
	// seed the failed drain jobs
	pg, _, err := NewPostgres()
	suite.Require().NoError(err)

	query := `INSERT INTO claim_drain (wallet_id, erred, errcode, status, batch_id, credentials, completed, total)
				VALUES ($1, $2, $3, $4, $5, '[{"t":"123"}]', FALSE, 1);`

	walletIDs := make([]uuid.UUID, 5, 5)
	for i := 0; i < 5; i++ {
		walletIDs[i] = uuid.NewV4()

		_, err = pg.RawDB().ExecContext(context.Background(), query, walletIDs[i].String(), true,
			"reputation-failed", "reputation-failed", uuid.NewV4().String())

		suite.Require().NoError(err, "should have inserted claim drain row")
	}

	// setup kafka topic and dialer
	SetAdminAttestationTopic(fmt.Sprintf("admin_attestation_events.%s.repsys.upstream", uuid.NewV4().String()))

	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	c := context.WithValue(context.Background(), appctx.KafkaBrokersCTXKey, kafkaBrokers)
	ctx, cancel := context.WithCancel(c)
	defer cancel()

	dialer, _, err := kafkautils.TLSDialer()
	suite.Require().NoError(err)
	conn, err := dialer.DialLeader(ctx, "tcp", strings.Split(kafkaBrokers, ",")[0], adminAttestationTopic, 0)
	suite.Require().NoError(err)

	err = conn.CreateTopics(kafka.TopicConfig{Topic: adminAttestationTopic, NumPartitions: 1, ReplicationFactor: 1})
	suite.Require().NoError(err)

	kafkaWriter, _, err := kafkautils.InitKafkaWriter(ctx, adminAttestationTopic)
	suite.Require().NoError(err)

	codecs, err := kafkautils.GenerateCodecs(map[string]string{
		adminAttestationTopic: adminAttestationEventSchema,
	})

	randomString := func() string {
		return uuid.NewV4().String()
	}

	msgCount := 5
	for i := 0; i < msgCount; i++ {
		msg := AdminAttestationEvent{
			WalletID:      walletIDs[i].String(),
			Service:       randomString(),
			Signal:        randomString(),
			Score:         rand.Int31n(10),
			Justification: randomString(),
			CreatedAt:     time.Now().String(),
		}

		textual, err := json.Marshal(msg)
		suite.Require().NoError(err)

		native, _, err := codecs[adminAttestationTopic].NativeFromTextual(textual)
		suite.Require().NoError(err)

		binary, err := codecs[adminAttestationTopic].BinaryFromNative(nil, native)
		suite.Require().NoError(err)

		err = kafkaWriter.WriteMessages(ctx, kafka.Message{
			Key:   []byte(walletIDs[i].String()),
			Value: binary,
		})
		suite.Require().NoError(err)
	}

	// start service
	go func(ctx context.Context) {
		service, _ := InitService(ctx, pg, nil, nil)
		service.RunNextDrainRetryJob(ctx)
	}(ctx)

	// assert drain job has been updated
	index := 0
	end := time.Now().Add(60 * time.Second) // max timeout
	var drainJob DrainJob
	for {
		if time.Now().After(end) {
			suite.Require().Fail("test failed due to timeout")
		}
		if index >= msgCount {
			break
		}
		// select the drain job and if erred is false check it has been fully updated
		err = pg.RawDB().Get(&drainJob, `SELECT * FROM claim_drain WHERE wallet_id = $1 LIMIT 1`, walletIDs[index])
		if drainJob.Erred == false {
			suite.Require().Equal(walletIDs[index], drainJob.WalletID)
			suite.Require().Equal(false, drainJob.Erred)
			suite.Require().Equal("reputation-failed", *drainJob.ErrCode)
			suite.Require().Equal("retry-bypass-cbr", *drainJob.Status)
			index += 1
		}
	}
}
