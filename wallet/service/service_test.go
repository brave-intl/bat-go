// +build integration

package service

import (
	"context"
	"testing"

	mockledger "github.com/brave-intl/bat-go/utils/clients/ledger/mock"
	gomock "github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
)

func TestGetOrCreateWallet(t *testing.T) {
	pg, err := NewPostgres("", false)
	if err != nil {
		t.Fatal(err)
	}

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	walletID := uuid.NewV4()

	mockLedger := mockledger.NewMockClient(mockCtrl)
	mockLedger.EXPECT().GetWallet(gomock.Any(), gomock.Eq(walletID)).Return(nil, nil)

	var service Service
	service.Datastore = pg
	service.LedgerClient = mockLedger

	wallet, err := service.GetOrCreateWallet(context.Background(), walletID)

	if wallet != nil {
		t.Fatal("Expected no wallet to be returned")
	}
	if err != nil {
		t.Fatal(err)
	}
}
