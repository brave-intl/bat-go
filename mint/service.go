package mint

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/promotion"
	"github.com/brave-intl/bat-go/utils/clients/slack"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Service holds values useful for service
type Service struct {
	SlackClient      slack.Client
	datastore        Datastore
	roDatastore      ReadOnlyDatastore
	PromotionService *promotion.Service
}

// Mint holds minimum mint data
type Mint struct {
	WalletIDs  []uuid.UUID
	Grants     int
	Amount     decimal.Decimal
	Bonus      decimal.Decimal
	Type       string
	Platform   string
	Legacy     bool
	ExpiryTime string
}

// InitService creates a mint for printing promotions and grants
func InitService(datastore Datastore, roDatastore ReadOnlyDatastore, promotionService *promotion.Service) (*Service, error) {
	// FOR DEV / STAGING ENVS ONLY
	if os.Getenv("ENV") != "local" {
		return nil, nil
	}
	slackClient, err := slack.New()
	if err != nil {
		return nil, err
	}
	s := Service{
		SlackClient:      slackClient,
		datastore:        datastore,
		roDatastore:      roDatastore,
		PromotionService: promotionService,
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
		WalletIDs:  []uuid.UUID{},
		Grants:     1,
		ExpiryTime: time.Now().Add(time.Hour * 24 * 30).Format(time.RFC3339),
		Amount:     decimal.NewFromFloat(10),
		Bonus:      decimal.NewFromFloat(0),
		Legacy:     false,
		Platform:   "desktop",
		Type:       "ads",
	}
	for _, input := range allActions {
		switch input.ActionID {
		case "wallet_id":
			if input.Value != "" {
				ids := []uuid.UUID{}
				list := strings.Split(input.Value, ",")
				for _, id := range list {
					id, err := uuid.FromString(strings.TrimSpace(id))
					if err != nil {
						return err
					}
					ids = append(ids, id)
				}
				mintPayload.WalletIDs = ids
			}
		case "grants":
			if input.Value != "" {
				grants, err := strconv.Atoi(input.Value)
				if err != nil {
					return err
				}
				mintPayload.Grants = grants
			}
		case "amount":
			if input.Value != "" {
				amount, err := decimal.NewFromString(input.Value)
				if err != nil {
					return err
				}
				mintPayload.Amount = amount
			}
		case "bonus":
			if input.Value != "" {
				bonus, err := decimal.NewFromString(input.Value)
				if err != nil {
					return err
				}
				mintPayload.Bonus = bonus
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
	// validate wallet assignments for types of promotions
	zeroMintWallets := len(mintPayload.WalletIDs) == 0
	if mintPayload.Type == "ads" && zeroMintWallets {
		return errors.New("an ads grant must be attributed to a wallet id")
	} else if mintPayload.Type == "ugp" && !zeroMintWallets {
		return errors.New("a ugp wallet must not be attributed to a wallet id")
	}

	grants := 1
	if mintPayload.Type == "ads" {
		grants = len(mintPayload.WalletIDs)
	}
	promoDatastore := service.PromotionService.Datastore()
	for i := 0; i < mintPayload.Grants; i++ {
		promotionData, err := promoDatastore.CreatePromotion(mintPayload.Type, grants, mintPayload.Amount, mintPayload.Platform)
		if err != nil {
			return err
		}

		err = promoDatastore.ActivatePromotion(promotionData)
		if err != nil {
			return err
		}

		_, err = service.PromotionService.CreateIssuer(ctx, promotionData.ID, "control")
		if err != nil {
			return err
		}

		// create claims if it is an ads promotion
		if mintPayload.Type != "ads" {
			continue
		}
		claimIDs := []string{}
		for _, id := range mintPayload.WalletIDs {
			claim, err := promoDatastore.CreateClaim(
				promotionData.ID,
				id.String(),
				mintPayload.Amount,
				mintPayload.Bonus,
			)
			if err != nil {
				return err
			}
			claimIDs = append(claimIDs, claim.ID.String())
		}
		if mintPayload.Legacy {
			_, err := promoDatastore.ExecContext(
				ctx,
				`update claims set legacy_claimed = true where claims.id = any('{$1}')`,
				strings.Join(claimIDs, ","),
			)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
