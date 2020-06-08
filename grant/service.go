package grant

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"os"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	srv "github.com/brave-intl/bat-go/utils/service"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	sentry "github.com/getsentry/sentry-go"
	"github.com/prometheus/client_golang/prometheus"
)

const localEnv = "local"

var (
	grantWalletPublicKeyHex  = os.Getenv("GRANT_WALLET_PUBLIC_KEY")
	grantWalletPrivateKeyHex = os.Getenv("GRANT_WALLET_PRIVATE_KEY")
	grantWalletCardID        = os.Getenv("GRANT_WALLET_CARD_ID")
	grantWallet              *uphold.Wallet
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

// InitService initializes the grant service
func InitService(ctx context.Context, datastore Datastore, roDatastore ReadOnlyDatastore) (*Service, error) {
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

		grantWallet, err = uphold.New(ctx, info, privKey, pubKey)
		if err != nil {
			return nil, err
		}
	} else if os.Getenv("ENV") != localEnv {
		return nil, errors.New("GRANT_WALLET_CARD_ID must be set in production")
	}

	if datastore != nil {
		prometheus.MustRegister(gs)
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
