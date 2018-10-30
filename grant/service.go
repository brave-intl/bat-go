package grant

import (
	"encoding/hex"
	"errors"
	"os"
	"strings"

	"github.com/brave-intl/bat-go/datastore"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/closers"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	"github.com/garyburd/redigo/redis"
	raven "github.com/getsentry/raven-go"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ed25519"
)

const (
	lowerTxLimit        = 1
	upperTxLimit        = 120
	ninetyDaysInSeconds = 60 * 60 * 24 * 90
	productionEnv       = "production"
)

var (
	// SettlementDestination is the address of the settlement wallet
	SettlementDestination = os.Getenv("BAT_SETTLEMENT_ADDRESS")
	// GrantSignatorPublicKeyHex is the hex encoded public key of the keypair used to sign grants
	GrantSignatorPublicKeyHex    = os.Getenv("GRANT_SIGNATOR_PUBLIC_KEY")
	grantWalletPublicKeyHex      = os.Getenv("GRANT_WALLET_PUBLIC_KEY")
	grantWalletPrivateKeyHex     = os.Getenv("GRANT_WALLET_PRIVATE_KEY")
	grantWalletCardID            = os.Getenv("GRANT_WALLET_CARD_ID")
	grantPublicKey               ed25519.PublicKey
	grantWallet                  *uphold.Wallet
	refreshBalance               = true  // for testing we can disable balance refresh
	testSubmit                   = true  // for testing we can disable testing tx submit
	registerGrantInstrumentation = true  // for testing we can disable grant claim / redeem instrumentation registration
	safeMode                     = false // if set true disables grant redemption
	claimedGrantsCounter         = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "claimed_grants_total",
			Help: "Number of grants claimed since start.",
		},
		[]string{},
	)
	redeemedGrantsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redeemed_grants_total",
			Help: "Number of grants redeemed since start.",
		},
		[]string{"promotionId"},
	)
)

// InitGrantService initializes the grant service
func InitGrantService(pool *redis.Pool) error {
	var err error
	grantPublicKey, err = hex.DecodeString(GrantSignatorPublicKeyHex)
	if err != nil {
		return err
	}

	if os.Getenv("ENV") == productionEnv && !refreshBalance {
		return errors.New("refreshBalance must be true in production")
	}
	if os.Getenv("ENV") == productionEnv && !testSubmit {
		return errors.New("testSubmit must be true in production")
	}

	if len(grantWalletCardID) > 0 {
		var info wallet.Info
		info.Provider = "uphold"
		info.ProviderID = grantWalletCardID
		{
			tmp := altcurrency.BAT
			info.AltCurrency = &tmp
		}

		var pubKey httpsignature.Ed25519PubKey
		var privKey ed25519.PrivateKey
		var err error

		pubKey, err = hex.DecodeString(grantWalletPublicKeyHex)
		if err != nil {
			return err
		}
		privKey, err = hex.DecodeString(grantWalletPrivateKeyHex)
		if err != nil {
			return err
		}

		grantWallet, err = uphold.New(info, privKey, pubKey)
		if err != nil {
			return err
		}
	} else if os.Getenv("ENV") == productionEnv {
		return errors.New("GRANT_WALLET_CARD_ID must be set in production")
	}

	if registerGrantInstrumentation {
		if pool != nil {
			gs := &grantService{
				pool: pool,
				outstandingGrantCountDesc: prometheus.NewDesc(
					"outstanding_grants_total",
					"Outstanding grants that have been claimed and have not expired.",
					[]string{},
					prometheus.Labels{},
				),
				completedGrantCountDesc: prometheus.NewDesc(
					"completed_grants_total",
					"Completed grants that have been redeemed.",
					[]string{"promotionId"},
					prometheus.Labels{},
				),
			}
			prometheus.MustRegister(gs)
		}

		prometheus.MustRegister(claimedGrantsCounter)
		prometheus.MustRegister(redeemedGrantsCounter)
	}

	return nil
}

type grantService struct {
	pool                      *redis.Pool
	outstandingGrantCountDesc *prometheus.Desc
	completedGrantCountDesc   *prometheus.Desc
}

// Describe returns all descriptions of the collector.
// We implement this and the Collect function to fulfill the prometheus.Collector interface
func (gs *grantService) Describe(ch chan<- *prometheus.Desc) {
	ch <- gs.outstandingGrantCountDesc
	ch <- gs.completedGrantCountDesc
}

// Collect returns the current state of all metrics of the collector.
// We implement this and the Describe function to fulfill the prometheus.Collector interface
func (gs *grantService) Collect(ch chan<- prometheus.Metric) {
	conn := gs.pool.Get()
	defer closers.Panic(conn)

	kv := datastore.GetRedisKv(&conn)
	ogCount, err := kv.Count("grant:*")
	if err != nil {
		raven.CaptureError(err, map[string]string{})
		return
	}
	ch <- prometheus.MustNewConstMetric(
		gs.outstandingGrantCountDesc,
		prometheus.GaugeValue,
		float64(ogCount),
	)
	promotions, err := kv.Keys("promotion:*:grants")
	if err != nil {
		raven.CaptureError(err, map[string]string{})
		return
	}
	for i := 0; i < len(promotions); i++ {
		promotionSet := datastore.GetRedisSet(&conn, promotions[i])
		promotionID := strings.TrimSuffix(strings.TrimPrefix(promotions[i], "promotion:"), ":grants")
		completedCount, err := promotionSet.Cardinality()
		if err != nil {
			raven.CaptureError(err, map[string]string{})
			return
		}

		ch <- prometheus.MustNewConstMetric(
			gs.completedGrantCountDesc,
			prometheus.GaugeValue,
			float64(completedCount),
			promotionID,
		)
	}
}
