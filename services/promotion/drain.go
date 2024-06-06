package promotion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

var (
	errReputationServiceFailure = errors.New("failed to call reputation service")
	errWalletNotReputable       = errors.New("wallet is not reputable")
	errWalletDrainLimitExceeded = errors.New("wallet drain limit exceeded")
	withdrawalLimitHit          = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name:        "withdrawalLimitHit",
			Help:        "A counter for when a drain hits the withdrawal limit",
			ConstLabels: prometheus.Labels{"service": "wallet"},
		})
)

// MintGrant create a new grant for the wallet specified with the total specified
func (service *Service) MintGrant(ctx context.Context, walletID uuid.UUID, total decimal.Decimal, promotions ...uuid.UUID) error {
	// setup a logger
	logger := logging.Logger(ctx, "promotion.MintGrant")

	// for all of the promotion ids (limit of 4 wallets can be linked)
	// attempt to create a claim.  If we run into a unique key constraint, this means that
	// we have already created a claim for this wallet id/ promotion
	var attempts int
	for _, pID := range promotions {
		logger.Debug().Msg("MintGrant: creating the claim to destination")
		// create a new claim for the wallet deposit account for total
		// this is a legacy claimed claim
		_, err := service.Datastore.CreateClaim(pID, walletID.String(), total, decimal.Zero, true)
		if err != nil {
			var pgErr *pq.Error
			if errors.As(err, &pgErr) {
				// unique constraint error (wallet id and promotion id combo exists)
				// use one of the other 4 promotions instead
				if pgErr.Code == "23505" {
					attempts++
					continue
				}
			}
			logger.Error().Err(err).Msg("MintGrant: failed to create a new claim to destination")
			return err
		}
		break
	}
	if attempts >= len(promotions) {
		return errors.New("limit of draining 4 wallets to brave wallet exceeded")
	}
	return nil
}

// FetchAdminAttestationWalletID - retrieves walletID from topic
func (service *Service) FetchAdminAttestationWalletID(ctx context.Context) (*uuid.UUID, error) {
	message, err := service.kafkaAdminAttestationReader.ReadMessage(ctx)
	if err != nil {
		return nil, fmt.Errorf("read message: error reading kafka message %w", err)
	}

	codec, ok := service.codecs[adminAttestationTopic]
	if !ok {
		return nil, fmt.Errorf("read message: could not find codec %s", adminAttestationTopic)
	}

	native, _, err := codec.NativeFromBinary(message.Value)
	if err != nil {
		return nil, fmt.Errorf("read message: error could not decode naitve from binary %w", err)
	}

	textual, err := codec.TextualFromNative(nil, native)
	if err != nil {
		return nil, fmt.Errorf("read message: error could not decode textual from native %w", err)
	}

	var adminAttestationEvent AdminAttestationEvent
	err = json.Unmarshal(textual, &adminAttestationEvent)
	if err != nil {
		return nil, fmt.Errorf("read message: error could not decode json from textual %w", err)
	}

	walletID := uuid.FromStringOrNil(adminAttestationEvent.WalletID)
	if walletID == uuid.Nil {
		return nil, fmt.Errorf("read message: error could not decode walletID %s", adminAttestationEvent.WalletID)
	}

	return &walletID, nil
}
