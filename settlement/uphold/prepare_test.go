package uphold

import (
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/settlement"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/shopspring/decimal"
)

// TestGroupSettlements tests GroupSettlements
func TestGroupSettlements(t *testing.T) {
	settlements, wantedSettlements := generateRandomSettlementsAndResultMap()
	result := GroupSettlements(&settlements)
	if !reflect.DeepEqual(result, wantedSettlements) {
		t.Fatalf("Wanted: %#v\nFound: %#v", wantedSettlements, result)
	}
}

// TestFlattenPaymentsByWalletProviderID tests FlattenPaymentsByWalletProviderID
func TestFlattenPaymentsByWalletProviderID(t *testing.T) {
	settlements, wantedSettlements := generateFixedSettlementsSliceAndResultsSlice()
	result := FlattenPaymentsByWalletProviderID(&settlements)
	if !reflect.DeepEqual(result, wantedSettlements) {
		t.Fatalf("Wanted: %#v\nFound: %#v", wantedSettlements, result)
	}
}

func generateRandomSettlementsAndResultMap() ([]settlement.Transaction, map[string][]settlement.Transaction) {
	var (
		settlements       []settlement.Transaction
		wantedSettlements = make(map[string][]settlement.Transaction)
	)
	altCurrency := altcurrency.BAT
	provderIds := []string{"ea524fe8-8a81-4191-a1bf-610f4b956816", "fc5cd231-22ac-403b-867c-9b8a080001b2", "060f9494-9b71-450b-89a7-ba745a9f36f7", "fe3e49fe-9c5c-4b08-b07c-97b3171c0b69"}
	// Push a random number of records for each provderId into the settlements slice
	// At the same time, construct the expected results map. This is done with random
	// numbers of settlements to mitigate programming to the test.
	for _, provderID := range provderIds {
		settlementInstance := settlement.Transaction{
			AltCurrency:      &altCurrency,
			Authority:        "test",
			Amount:           decimal.NewFromFloat(0.0),
			ExchangeFee:      decimal.NewFromFloat(0.0),
			FailureReason:    "test",
			Currency:         "test",
			Destination:      "test",
			Publisher:        "test",
			BATPlatformFee:   decimal.NewFromFloat(0.0),
			Probi:            decimal.NewFromFloat(0.0),
			ProviderID:       "test",
			WalletProvider:   "uphold",
			WalletProviderID: provderID,
			Channel:          "test",
			SignedTx:         "test",
			Status:           "test",
			SettlementID:     "test",
			TransferFee:      decimal.NewFromFloat(0.0),
			Type:             "test",
			ValidUntil:       time.Now(),
			DocumentID:       "test",
			Note:             "test",
		}

		for i := 0; i <= rand.Intn(5); i++ {
			settlements = append(settlements, settlementInstance)
			if wantedSettlements[provderID] != nil {
				wantedSettlements[provderID] = append(
					wantedSettlements[provderID],
					settlementInstance,
				)
			} else {
				wantedSettlements[provderID] = []settlement.Transaction{
					settlementInstance,
				}
			}
		}
	}
	return settlements, wantedSettlements
}

func generateFixedSettlementsSliceAndResultsSlice() ([]settlement.Transaction, []settlement.Transaction) {
	var (
		settlements       []settlement.Transaction
		wantedSettlements []settlement.Transaction
	)
	altCurrency := altcurrency.BAT
	provderIds := []string{"ea524fe8-8a81-4191-a1bf-610f4b956816", "fc5cd231-22ac-403b-867c-9b8a080001b2", "060f9494-9b71-450b-89a7-ba745a9f36f7", "fe3e49fe-9c5c-4b08-b07c-97b3171c0b69"}
	priceSet := []decimal.Decimal{
		decimal.NewFromFloat(1.0),
		decimal.NewFromFloat(0.2),
		decimal.NewFromFloat(0.5),
		decimal.NewFromFloat(4.1),
		decimal.NewFromFloat(9.3),
		decimal.NewFromFloat(24.10),
		decimal.NewFromFloat(7.7),
	}
	wantedSum := decimal.NewFromFloat(46.9)
	presetTime := time.Now()
	for _, provderID := range provderIds {
		settlementInstance := settlement.Transaction{
			AltCurrency:      &altCurrency,
			Authority:        "test",
			Amount:           decimal.NewFromFloat(0.0),
			ExchangeFee:      decimal.NewFromFloat(0.0),
			FailureReason:    "test",
			Currency:         "test",
			Destination:      "test",
			Publisher:        "test",
			BATPlatformFee:   decimal.NewFromFloat(0.0),
			Probi:            decimal.NewFromFloat(0.0),
			ProviderID:       "test",
			WalletProvider:   "uphold",
			WalletProviderID: provderID,
			Channel:          "test",
			SignedTx:         "test",
			Status:           "test",
			SettlementID:     "test",
			TransferFee:      decimal.NewFromFloat(0.0),
			Type:             "test",
			ValidUntil:       presetTime,
			DocumentID:       "test",
			Note:             "test",
		}
		for _, price := range priceSet {
			settlementInstance.Amount = price
			settlements = append(settlements, settlementInstance)
		}
		settlementInstance.Amount = wantedSum
		wantedSettlements = append(wantedSettlements, settlementInstance)
	}
	return settlements, wantedSettlements
}
