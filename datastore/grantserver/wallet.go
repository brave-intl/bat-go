package grantserver

import (
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/wallet"
	uuid "github.com/satori/go.uuid"
)

// UpsertWallet upserts the given wallet
func (pg *Postgres) UpsertWallet(wallet *wallet.Info) error {
	statement := `
	insert into wallets (id, provider, provider_id, public_key, provider_linking_id, anonymous_address)
	values ($1, $2, $3, $4, $5, $6)
	on conflict (id) do
	update set
		provider_linking_id = $5,
		anonymous_address = $6
	returning *`
<<<<<<< HEAD
	_, err := pg.RawDB().Exec(statement, wallet.ID, wallet.Provider, wallet.ProviderID, wallet.PublicKey, wallet.PayoutAddress)
=======
	_, err := pg.DB.Exec(statement, wallet.ID, wallet.Provider, wallet.ProviderID, wallet.PublicKey, wallet.ProviderLinkingID, wallet.AnonymousAddress)
>>>>>>> add wallet endpoints
	if err != nil {
		return err
	}

	return nil
}

// GetWallet by ID
func (pg *Postgres) GetWallet(ID uuid.UUID) (*wallet.Info, error) {
	statement := "select * from wallets where id = $1"
	wallets := []wallet.Info{}
	err := pg.RawDB().Select(&wallets, statement, ID)
	if err != nil {
		return nil, err
	}

	if len(wallets) > 0 {
		// FIXME currently assumes BAT
		{
			tmp := altcurrency.BAT
			wallets[0].AltCurrency = &tmp
		}
		return &wallets[0], nil
	}

	return nil, nil
}
