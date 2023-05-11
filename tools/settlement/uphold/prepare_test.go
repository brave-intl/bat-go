package uphold

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/shopspring/decimal"
	"gotest.tools/assert"
)

// TestGroupSettlements tests GroupSettlements
func TestGroupSettlements(t *testing.T) {
	settlements, wantedSettlements := generateRandomSettlementsAndResultMap()
	result := GroupSettlements(&settlements)
	if !reflect.DeepEqual(result, wantedSettlements) {
		t.Fatalf("wanted: %#v\nfound: %#v", wantedSettlements, result)
	}
}

// TestFlattenPaymentsByWalletProviderID tests FlattenPaymentsByWalletProviderID
func TestFlattenPaymentsByWalletProviderID(t *testing.T) {
	settlements, wantedSettlements := generateFixedSettlementsSliceAndResultsSlice()
	results := FlattenPaymentsByWalletProviderID(&settlements)
	foundMatches := 0
	for _, result := range results {
		for _, wantedSettlement := range wantedSettlements {
			fmt.Printf("wanted WalletProviderID: %s\nfound WalletProviderID: %s\n", wantedSettlement.WalletProviderID, result.WalletProviderID)
			if wantedSettlement.WalletProviderID == result.WalletProviderID {
				fmt.Printf("wanted Amount: %s\nfound Amount: %s\n", wantedSettlement.Probi, result.Probi)
				if wantedSettlement.Probi.Equal(result.Probi) {
					foundMatches++
				}
			}
		}
	}
	assert.Equal(t, foundMatches, len(wantedSettlements))
}

func generateRandomSettlementsAndResultMap() ([]custodian.Transaction, map[string][]custodian.Transaction) {
	var (
		settlements       []custodian.Transaction
		wantedSettlements = make(map[string][]custodian.Transaction)
	)
	altCurrency := altcurrency.BAT
	provderIds := []string{"ea524fe8-8a81-4191-a1bf-610f4b956816", "fc5cd231-22ac-403b-867c-9b8a080001b2", "060f9494-9b71-450b-89a7-ba745a9f36f7", "fe3e49fe-9c5c-4b08-b07c-97b3171c0b69"}
	// Push a random number of records for each provderId into the settlements slice
	// At the same time, construct the expected results map. This is done with random
	// numbers of settlements to mitigate programming to the test.
	for _, provderID := range provderIds {
		settlementInstance := custodian.Transaction{
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
				wantedSettlements[provderID] = []custodian.Transaction{
					settlementInstance,
				}
			}
		}
	}
	return settlements, wantedSettlements
}

func generateFixedSettlementsSliceAndResultsSlice() ([]custodian.Transaction, []custodian.Transaction) {
	var (
		settlements       []custodian.Transaction
		wantedSettlements []custodian.Transaction
	)
	altCurrency := altcurrency.BAT
	provderIds := []string{"ea524fe8-8a81-4191-a1bf-610f4b956816", "fc5cd231-22ac-403b-867c-9b8a080001b2", "060f9494-9b71-450b-89a7-ba745a9f36f7", "fe3e49fe-9c5c-4b08-b07c-97b3171c0b69"}

	probi1, _ := decimal.NewFromString("1000000000000000000")
	probi2, _ := decimal.NewFromString("200000000000000000")
	probi3, _ := decimal.NewFromString("500000000000000000")
	probi4, _ := decimal.NewFromString("4100000000000000000")
	probi5, _ := decimal.NewFromString("9300000000000000000")
	probi6, _ := decimal.NewFromString("24100000000000000000")
	probi7, _ := decimal.NewFromString("7700000000000000000")
	probi8, _ := decimal.NewFromString("1000000000000000345")
	priceSet := []decimal.Decimal{
		probi1, probi2, probi3, probi4, probi5, probi6, probi7, probi8,
	}
	wantedSum, _ := decimal.NewFromString("47900000000000000345")
	presetTime := time.Now()
	for _, provderID := range provderIds {
		settlementInstance := custodian.Transaction{
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
			settlementInstance.Probi = price
			settlements = append(settlements, settlementInstance)
		}
		settlementInstance.Probi = wantedSum
		wantedSettlements = append(wantedSettlements, settlementInstance)
	}
	return settlements, wantedSettlements
}
