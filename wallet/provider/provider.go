package provider

import (
	"errors"
	"fmt"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
)

func GetWallet(info wallet.WalletInfo) (wallet.Wallet, error) {
	switch info.Provider {
	case "uphold":
		uW, err := uphold.FromWalletInfo(info)
		if err != nil {
			return uW, err
		}
		// TODO once we can retrieve public key info from uphold
		// err = uW.UpdatePublicKey()
		return uW, err
	default:
		return nil, errors.New(fmt.Sprintf("No such supported wallet provider %s", info.Provider))
	}
}
