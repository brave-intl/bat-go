package eyeshade

import (
	"context"

	"github.com/brave-intl/bat-go/eyeshade/countries"
	"github.com/brave-intl/bat-go/eyeshade/datastore"
	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/brave-intl/bat-go/utils/inputs"
)

// Datastore returns a read only datastore if available
// otherwise a normal datastore
func (service *Service) Datastore(ro ...bool) datastore.Datastore {
	if len(ro) > 0 && ro[0] && service.roDatastore != nil {
		return service.roDatastore
	}
	return service.datastore
}

// GetAccountEarnings uses the readonly connection if available to get the account earnings
func (service *Service) GetAccountEarnings(
	ctx context.Context,
	options models.AccountEarningsOptions,
) (*[]models.AccountEarnings, error) {
	return service.Datastore(true).
		GetAccountEarnings(
			ctx,
			options,
		)
}

// GetAccountSettlementEarnings uses the readonly connection if available to get the account earnings
func (service *Service) GetAccountSettlementEarnings(
	ctx context.Context,
	options models.AccountSettlementEarningsOptions,
) (*[]models.AccountSettlementEarnings, error) {
	return service.Datastore(true).
		GetAccountSettlementEarnings(
			ctx,
			options,
		)
}

// GetBalances uses the readonly connection if available to get the account earnings
func (service *Service) GetBalances(
	ctx context.Context,
	accountIDs []string,
	includePending bool,
) (*[]models.Balance, error) {
	d := service.Datastore(true)
	balances, err := d.GetBalances(
		ctx,
		accountIDs,
	)
	if err != nil {
		return nil, err
	}
	if includePending {
		pendingVotes, err := d.GetPending(
			ctx,
			accountIDs,
		)
		if err != nil {
			return nil, err
		}
		return models.MergePendingTransactions(*pendingVotes, *balances), nil
	}
	return balances, nil
}

// GetTransactionsByAccount uses the readonly connection if available to get the account transactions
func (service *Service) GetTransactionsByAccount(
	ctx context.Context,
	accountID string,
	txTypes []string,
) (*[]models.CreatorsTransaction, error) {
	transactions, err := service.Datastore(true).
		GetTransactionsByAccount(
			ctx,
			accountID,
			txTypes,
		)
	if err != nil {
		return nil, err
	}
	return models.TransactionsToCreatorsTransactions(
		accountID,
		transactions,
	), nil
}

// GetReferralGroups gets the referral groups that match the input parameters
func (service *Service) GetReferralGroups(
	ctx context.Context,
	resolve bool,
	activeAt inputs.Time,
	fields ...string,
) (*[]countries.ReferralGroup, error) {
	groups, err := service.Datastore(true).
		GetReferralGroups(ctx, activeAt)
	if err != nil {
		return nil, err
	}
	if resolve {
		groups = countries.Resolve(*groups)
	}
	for _, group := range *groups {
		group.SetKeys(fields) // will only render these keys when serializing
	}
	return groups, nil
}

// GetGrantStats gets the grnat stats that match the input parameters
func (service *Service) GetGrantStats(
	ctx context.Context,
	grantStatOptions models.GrantStatOptions,
) (*models.GrantStat, error) {
	return service.Datastore(true).
		GetGrantStats(ctx, grantStatOptions)
}

// GetSettlementStats gets the grnat stats that match the input parameters
func (service *Service) GetSettlementStats(
	ctx context.Context,
	settlementStatOptions models.SettlementStatOptions,
) (*models.SettlementStat, error) {
	return service.Datastore(true).
		GetSettlementStats(ctx, settlementStatOptions)
}

// InsertConvertableTransactions gets the grnat stats that match the input parameters
func (service *Service) InsertConvertableTransactions(
	ctx context.Context,
	txs []models.ConvertableTransaction,
) error {
	return service.Datastore().
		InsertConvertableTransactions(ctx, txs)
}
