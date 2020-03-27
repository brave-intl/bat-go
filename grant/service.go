package grant

import (
	"encoding/hex"
	"errors"
	"os"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	srv "github.com/brave-intl/bat-go/utils/service"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	"github.com/getsentry/sentry-go"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ed25519"
)

const (
	lowerTxLimit = 0.25
	upperTxLimit = 120.0
	localEnv     = "local"
)

var (
	// GrantSignatorPublicKeyHex is the hex encoded public key of the keypair used to sign grants
	GrantSignatorPublicKeyHex    = os.Getenv("GRANT_SIGNATOR_PUBLIC_KEY")
	grantWalletPublicKeyHex      = os.Getenv("GRANT_WALLET_PUBLIC_KEY")
	grantWalletPrivateKeyHex     = os.Getenv("GRANT_WALLET_PRIVATE_KEY")
	grantWalletCardID            = os.Getenv("GRANT_WALLET_CARD_ID")
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

// Service contains datastore as well as prometheus metrics
type Service struct {
	datastore              Datastore
	roDatastore            ReadOnlyDatastore
	grantWalletBalanceDesc *prometheus.Desc
	jobs                   []srv.Job
}

// Jobs - Implement srv.JobService interface
func (s *Service) Jobs() []srv.Job {
	return s.jobs
}

// NewService is created
func NewService() (*Service, error) {
	roDB := os.Getenv("RO_DATABASE_URL")

	var grantRoPg ReadOnlyDatastore
	grantPg, err := NewPostgres("", true)
	if err != nil {
		return nil, errors.Wrap(err, "Must be able to init postgres connection to start")
	}
	if len(roDB) > 0 {
		grantRoPg, err = NewPostgres(roDB, false)
		if err != nil {
			return nil, errors.Wrap(err, "Could not start reader postgres connection")
		}
	}

	return InitService(grantPg, grantRoPg)
}

// InitService initializes the grant service
func InitService(datastore Datastore, roDatastore ReadOnlyDatastore) (*Service, error) {
	gs := &Service{
		datastore:   datastore,
		roDatastore: roDatastore,
		grantWalletBalanceDesc: prometheus.NewDesc(
			"grant_wallet_balance",
			"A gauge of the grant wallet remaining balance.",
			[]string{},
			prometheus.Labels{},
		),
	}

	// setup runnable jobs
	gs.jobs = []srv.Job{}

	if os.Getenv("ENV") != localEnv && !refreshBalance {
		return nil, errors.New("refreshBalance must be true in production")
	}
	if os.Getenv("ENV") != localEnv && !testSubmit {
		return nil, errors.New("testSubmit must be true in production")
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
			return nil, errorutils.Wrap(err, "grantWalletPublicKeyHex is invalid")
		}
		privKey, err = hex.DecodeString(grantWalletPrivateKeyHex)
		if err != nil {
			return nil, errorutils.Wrap(err, "grantWalletPrivateKeyHex is invalid")
		}

		grantWallet, err = uphold.New(info, privKey, pubKey)
		if err != nil {
			return nil, err
		}
	} else if os.Getenv("ENV") != localEnv {
		return nil, errors.New("GRANT_WALLET_CARD_ID must be set in production")
	}

	if registerGrantInstrumentation {
		if datastore != nil {
			prometheus.MustRegister(gs)
		}

		prometheus.MustRegister(claimedGrantsCounter)
		prometheus.MustRegister(redeemedGrantsCounter)
		registerGrantInstrumentation = false
	}

	return gs, nil
}

// ReadableDatastore returns a read only datastore if available, otherwise a normal datastore
func (s *Service) ReadableDatastore() ReadOnlyDatastore {
	if s.roDatastore != nil {
		return s.roDatastore
	}
	return s.datastore
}

// Describe returns all descriptions of the collector.
// We implement this and the Collect function to fulfill the prometheus.Collector interface
func (s *Service) Describe(ch chan<- *prometheus.Desc) {
	ch <- s.grantWalletBalanceDesc
}

// Collect returns the current state of all metrics of the collector.
// We implement this and the Describe function to fulfill the prometheus.Collector interface
func (s *Service) Collect(ch chan<- prometheus.Metric) {
	balance, err := grantWallet.GetBalance(true)
	if err != nil {
		sentry.CaptureException(err)
		return
	}

	spendable, _ := grantWallet.GetWalletInfo().AltCurrency.FromProbi(balance.SpendableProbi).Float64()

	ch <- prometheus.MustNewConstMetric(
		s.grantWalletBalanceDesc,
		prometheus.GaugeValue,
		spendable,
	)
}
