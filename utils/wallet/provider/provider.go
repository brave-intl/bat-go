package provider

import (
	"context"
	"fmt"

	"github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
)

// GetWallet returns the wallet corresponding to the passed wallet info
func GetWallet(ctx context.Context, info wallet.Info) (wallet.Wallet, error) {
	switch info.Provider {
	case "uphold":
		uW, err := uphold.FromWalletInfo(ctx, info)
		if err != nil {
			return uW, err
		}
		// TODO once we can retrieve public key info from uphold
		// err = uW.UpdatePublicKey()
		return uW, err
	default:
		return nil, fmt.Errorf("No such supported wallet provider %s", info.Provider)
	}
}
