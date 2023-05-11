package grant

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"os"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	srv "github.com/brave-intl/bat-go/libs/service"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/libs/wallet/provider/uphold"
	"github.com/brave-intl/bat-go/services/promotion"
	"github.com/brave-intl/bat-go/services/wallet"
	sentry "github.com/getsentry/sentry-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/shopspring/decimal"
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
	baseCtx                context.Context
	Datastore              Datastore
	RoDatastore            ReadOnlyDatastore
	wallet                 *wallet.Service
	promotion              *promotion.Service
	grantWalletBalanceDesc *prometheus.Desc
	jobs                   []srv.Job
}

// Jobs - Implement srv.JobService interface
func (s *Service) Jobs() []srv.Job {
	return s.jobs
}

// InitService initializes the grant service
func InitService(
	ctx context.Context,
	datastore Datastore,
	roDatastore ReadOnlyDatastore,
	walletService *wallet.Service,
	promotionService *promotion.Service,
) (*Service, error) {
	gs := &Service{
		baseCtx:     ctx,
		Datastore:   datastore,
		RoDatastore: roDatastore,
		wallet:      walletService,
		promotion:   promotionService,
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
		var info walletutils.Info
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
	if s.RoDatastore != nil {
		return s.RoDatastore
	}
	return s.Datastore
}

// Describe returns all descriptions of the collector.
// We implement this and the Collect function to fulfill the prometheus.Collector interface
func (s *Service) Describe(ch chan<- *prometheus.Desc) {
	ch <- s.grantWalletBalanceDesc
}

// Collect returns the current state of all metrics of the collector.
// We implement this and the Describe function to fulfill the prometheus.Collector interface
func (s *Service) Collect(ch chan<- prometheus.Metric) {
	balance, err := grantWallet.GetBalance(s.baseCtx, true)
	if err != nil {
		sentry.CaptureException(err)
		balance = grantWallet.LastBalance
		if balance == nil {
			balance = &walletutils.Balance{
				SpendableProbi: decimal.Zero,
			}
		}
	}

	spendable, _ := grantWallet.GetWalletInfo().AltCurrency.FromProbi(balance.SpendableProbi).Float64()

	ch <- prometheus.MustNewConstMetric(
		s.grantWalletBalanceDesc,
		prometheus.GaugeValue,
		spendable,
	)
}
