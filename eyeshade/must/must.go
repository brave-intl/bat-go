package must

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// Marshal requires the ability to marshal a struct into json
func Marshal(
	assertions *require.Assertions,
	structure interface{},
) string {
	marshalled, err := json.Marshal(structure)
	assertions.NoError(err)
	return string(marshalled)
}

// CreateContributions creates contributions with random data
func CreateContributions(count int) []models.Contribution {
	contributions := []models.Contribution{}
	types := models.ContributionTypes.All()
	promotionLimit := 3
	promotionIDs := RandomIDs(promotionLimit)
	for i := 0; i < count; i++ {
		for _, t := range types {
			contributions = append(contributions, models.Contribution{
				ID:            uuid.NewV4().String(),
				Type:          t,
				Channel:       models.Channel("brave.com"),
				CreatedAt:     time.Now().UTC(),
				BaseVoteValue: models.VoteValue,
				VoteTally:     RandomNumber(1, 9),
				FundingSource: promotionIDs[RandomNumber(promotionLimit)],
			})
		}
	}
	return contributions
}

// CreateSettlements creates settlements with random data
// given a count and a settlement type (see models.TransactionTypes)
func CreateSettlements(count int, txType string) []models.Settlement {
	settlements := []models.Settlement{}
	for i := 0; i < count; i++ {
		bat := decimal.NewFromFloat(5)
		fees := bat.Mul(models.ContributionFee)
		batSubFees := bat.Sub(fees)
		settlements = append(settlements, models.Settlement{
			AltCurrency:  altcurrency.BAT,
			Probi:        altcurrency.BAT.ToProbi(batSubFees),
			Fees:         altcurrency.BAT.ToProbi(fees),
			Fee:          decimal.Zero,
			Commission:   decimal.Zero,
			Amount:       bat,
			Currency:     altcurrency.BAT.String(),
			Owner:        fmt.Sprintf("publishers#uuid:%s", uuid.NewV4().String()),
			Channel:      models.Channel("brave.com"),
			Hash:         uuid.NewV4().String(),
			Type:         txType,
			SettlementID: uuid.NewV4().String(),
			DocumentID:   uuid.NewV4().String(),
			Address:      uuid.NewV4().String(),
		})
	}
	return settlements
}

// Unmarshal must be able to unmarshal a json string
// into a given structure
// structure must be a pointer to access value
func Unmarshal(
	assertions *require.Assertions,
	bytes []byte,
	structure interface{},
) {
	assertions.NoError(json.Unmarshal(bytes, structure))
}

// RandomDecimal creates a random decimal amount
func RandomDecimal() decimal.Decimal {
	return decimal.NewFromFloat(
		float64(rand.Intn(100)),
	).Div(
		decimal.NewFromFloat(10),
	)
}

// CreateReferrals creates referrals given a count and group id
func CreateReferrals(count int, countryGroupID uuid.UUID) []models.Referral {
	referrals := []models.Referral{}
	for i := 0; i < count; i++ {
		now := time.Now()
		referrals = append(referrals, models.Referral{
			TransactionID:      uuid.NewV4().String(),
			Channel:            models.Channel("brave.com"),
			Owner:              fmt.Sprintf("publishers#uuid:%s", uuid.NewV4().String()),
			FinalizedTimestamp: now,
			ReferralCode:       "ABC123",
			DownloadID:         uuid.NewV4().String(),
			DownloadTimestamp:  now.AddDate(0, 0, -30),
			CountryGroupID:     countryGroupID.String(),
			Platform:           "osx",
		})
	}
	return referrals
}

// CreateSuggestions creates suggestions given a count
// all different types are sent back
// if only one is required, they can be passed as optional paramters
func CreateSuggestions(
	count int,
	params ...[]string,
) []models.Suggestion {
	suggestions := []models.Suggestion{}
	promotionLimit := 5
	promotionIDs := RandomIDs(promotionLimit)
	suggestionTypes := models.ContributionTypeList
	fundingTypes := models.FundingTypeList
	if len(params) > 0 {
		suggestionTypes = params[0]
		if len(params) > 1 {
			fundingTypes = params[1]
		}
	}
	for i := 0; i < count; i++ {
		now := time.Now()
		fundings := []models.Funding{}
		total := decimal.Zero
		rand.Seed(int64(i))
		for _, suggestionType := range suggestionTypes {
			for j := 0; j < 5; j++ {
				random := rand.Int()
				promotionIDIndex := random % promotionLimit
				amount := decimal.NewFromFloat(float64(random%10 + 1)).Mul(models.VoteValue)
				total = total.Add(amount)
				for _, fundingType := range fundingTypes {
					fundings = append(fundings, models.Funding{
						Type:      fundingType,
						Amount:    amount,
						Cohort:    "control",
						Promotion: promotionIDs[promotionIDIndex].String(),
					})
				}
			}
			suggestions = append(suggestions, models.Suggestion{
				ID:          uuid.NewV4().String(),
				Type:        suggestionType,
				Channel:     models.Channel("brave.com"),
				CreatedAt:   now,
				TotalAmount: total,
				OrderID:     uuid.NewV4().String(),
				Funding:     fundings,
			})
		}
	}
	return suggestions
}

// RandomNumber creates random numbers for tests
func RandomNumber(params ...int) int {
	var max, min int
	random := rand.Int()
	if len(params) > 1 {
		max = params[1]
		min = params[0]
	} else if len(params) == 1 {
		max = params[0]
	} else {
		return random
	}
	delta := max - min
	leftover := random % delta
	return leftover + min
}

// RandomIDs generates random ids
func RandomIDs(count int, flags ...bool) []uuid.UUID {
	ids := []uuid.UUID{}
	for i := 0; i < count; i++ {
		ids = append(ids, uuid.NewV4())
	}
	return ids
}

// UUIDsToString turns a list of uuids to strings
func UUIDsToString(uuids ...uuid.UUID) []string {
	list := []string{}
	for _, id := range uuids {
		list = append(list, id.String())
	}
	return list
}

// WaitFor the utility to call functions on a periotic interval
func WaitFor(
	errChan chan error,
	handler func(chan error, bool) (bool, error),
) {
	for {
		<-time.After(time.Millisecond * 100)
		finished, err := handler(errChan, false)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			errChan <- err
			return
		}
		if finished {
			_, err := handler(errChan, true)
			if err != nil {
				errChan <- err
			}
			return
		}
	}
}
