package mint

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/brave-intl/bat-go/promotion"
	"github.com/brave-intl/bat-go/utils/clients/slack"
	"github.com/getsentry/raven-go"
	"github.com/rs/zerolog/log"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Service holds values useful for service
type Service struct {
	SlackClient slack.Client
	datastore   Datastore
	roDatastore ReadOnlyDatastore
}

// Mint holds minimum mint data
type Mint struct {
	WalletID   uuid.UUID
	Amount     decimal.Decimal
	Type       string
	Platform   string
	Legacy     bool
	ExpiryTime string
}

// InitService creates a mint for printing promotions and grants
func InitService(datastore Datastore, roDatastore ReadOnlyDatastore) (*Service, error) {
	// FOR DEV / STAGING ENVS ONLY
	if os.Getenv("ENV") == "production" {
		return nil, nil
	}
	slackClient, err := slack.New()
	if err != nil {
		return nil, err
	}
	s := Service{
		SlackClient: slackClient,
		datastore:   datastore,
		roDatastore: roDatastore,
	}
	return &s, nil
}

// Datastore returns the datastore
func (service *Service) Datastore() Datastore {
	return service.datastore
}

// ReadableDatastore returns a read only datastore if available, otherwise a normal datastore
func (service *Service) ReadableDatastore() ReadOnlyDatastore {
	if service.roDatastore != nil {
		return service.roDatastore
	}
	return service.Datastore()
}

// HandleBlockAction holds the logic for transferring data from interactive content's requests to the database
func (service *Service) HandleBlockAction(ctx context.Context, payload slack.ActionsRequest) error {
	view := payload.View
	action := payload.Actions[0]
	value := ""
	switch action.Type {
	case "static_select":
		value = action.SelectedOption.Value
	case "radio_buttons":
		value = action.SelectedOption.Value
	case "datepicker":
		value = action.SelectedDate
	default:
	}
	return service.Datastore().UpsertAction(ctx, Action{
		ActionID: action.ActionID,
		Value:    value,
		ModalID:  view.CallbackID,
	})
}

// HandleViewSubmission handles the view being submitted
func (service *Service) HandleViewSubmission(ctx context.Context, payload slack.ActionsRequest) error {
	state := payload.View.State.Values
	actions := []Action{}
	modalID := payload.View.CallbackID
	for k := range state {
		actionState := state[k]
		for actionKey := range actionState {
			actions = append(actions, Action{
				ActionID: actionKey,
				Value:    actionState[actionKey].Value,
				ModalID:  modalID,
			})
		}
	}
	moreActions, err := service.Datastore().GetModalActions(ctx, modalID)
	if err != nil {
		return err
	}
	allActions := append(actions, (*moreActions)...)
	mintPayload := Mint{
		WalletID:   uuid.Nil,
		ExpiryTime: time.Now().Add(time.Hour * 24 * 30).Format(time.RFC3339),
		Amount:     decimal.NewFromFloat(10),
		Legacy:     false,
		Platform:   "desktop",
		Type:       "ads",
	}
	for _, input := range allActions {
		switch input.ActionID {
		case "wallet_id":
			if input.Value != "" {
				id, err := uuid.FromString(input.Value)
				if err != nil {
					return err
				}
				mintPayload.WalletID = id
			}
		case "amount":
			amount, err := decimal.NewFromString(input.Value)
			if input.Value != "" {
				if err != nil {
					return err
				}
				mintPayload.Amount = amount
			}
		case "type":
			mintPayload.Type = input.Value
		case "platform":
			desktopPlatforms := [...]string{"linux", "osx", "windows"}
			platform := input.Value
			if input.Value == "" {
				return errors.New("platform must be chosen")
			}
			for _, desktopPlatform := range desktopPlatforms {
				if platform == desktopPlatform {
					platform = "desktop"
				}
			}
			mintPayload.Platform = input.Value
		case "legacy":
			mintPayload.Legacy = input.Value == "yes"
		case "expiry_time":
			if input.Value != "" {
				mintPayload.ExpiryTime = input.Value
			}
		}
	}
	if mintPayload.Type == "ads" && uuid.Equal(uuid.Nil, mintPayload.WalletID) {
		return errors.New("an ads grant must be attributed to a wallet id")
	}

	var roPromoPG promotion.ReadOnlyDatastore
	promoPG, err := promotion.NewPostgres("", true, "promotion_db")
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		log.Panic().Err(err).Msg("Must be able to init postgres connection to start")
	}

	promoService, err := promotion.InitService(promoPG, roPromoPG)
	if err != nil {
		return err
	}
	promotion, err := promoPG.CreatePromotion(mintPayload.Type, 1, mintPayload.Amount, mintPayload.Platform)
	if err != nil {
		return err
	}

	err = promoPG.ActivatePromotion(promotion)
	if err != nil {
		return err
	}

	_, err = promoService.CreateIssuer(ctx, promotion.ID, "control")
	if err != nil {
		return err
	}

	if mintPayload.Type == "ads" {
		claim, err := promoPG.CreateClaim(
			promotion.ID,
			mintPayload.WalletID.String(),
			mintPayload.Amount,
			decimal.NewFromFloat(0),
		)
		if err != nil {
			return err
		}
		if mintPayload.Legacy {
			_, err := promoPG.DB.ExecContext(ctx, `update claims set legacy_claimed = true where claims.id = $1`, claim.ID)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
