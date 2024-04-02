package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/program/associated_token_account"
	"github.com/blocto/solana-go-sdk/program/token"
	"github.com/blocto/solana-go-sdk/rpc"
	"github.com/blocto/solana-go-sdk/types"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
	"github.com/shopspring/decimal"
)

type chainIdempotencyData struct {
	BlockHash  string `json:"blockHash"`
	SlotTarget uint64 `json:"slotTarget"`
	Signature  string `json:"signature"`
}

const (
	CommitmentNotFound rpc.Commitment = "notfound"
	CommitmentUnknown rpc.Commitment = "unknown"
	SPLBATMintDecimals  uint8  = 8                                              // Mint decimals for Wormhole wrapped BAT on mainnet
	SPLBATMintAddress   string = "EPeUFDgHRxs9xxEPVaL6kfGQvCon7jmAWKVUHuux1Tpz" // Mint address for Wormhole wrapped BAT on mainnet
	tokenProgramAddress string = "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"
)

// SolanaMachine is an implementation of TxStateMachine for Solana on-chain payouts
// use-case.
//
// Including the baseStateMachine provides a default implementation of TxStateMachine.
type SolanaMachine struct {
	baseStateMachine
	// signingKey is the private key of the funding wallet encoded in base58 format.
	//
	// The key can be derived from a mnemonic like this:
	//
	// 	mnemonic := "neither lonely flavor argue grass remind eye tag avocado spot unusual intact"
	// 	seed := bip39.NewSeed(mnemonic, "") // (mnemonic, password)
	// 	path := `m/44'/501'/0'/0'`
	// 	derivedKey, _ := hdwallet.Derived(path, seed)
	// 	derivedKey.PrivateKey
	signingKey        string
	solanaRpcEndpoint string
	splMintAddress    string
	splMintDecimals   uint8
}

// Pay implements TxStateMachine for the Solana machine.
func (sm *SolanaMachine) Pay(ctx context.Context) (*paymentLib.AuthenticatedPaymentState, error) {
	err := ctx.Err()
	if errors.Is(err, context.DeadlineExceeded) {
		return sm.transaction, err
	}

	splMintPublicKey := common.PublicKeyFromString(sm.splMintAddress)

	// Skip if the state is already final
	if sm.transaction.Status == paymentLib.Paid || sm.transaction.Status == paymentLib.Failed {
		return sm.transaction, nil
	}

	client := client.NewClient(sm.solanaRpcEndpoint)

	idempotencyData, err := decodeOrFetchChainIdempotencyData(ctx, sm.transaction.ExternalIdempotency, client)
	if err != nil {
		return sm.transaction, fmt.Errorf("failed to decode or fetch idempotency data: %w", err)
	}

	// If the signature is present in the idempotency data we should check its status before
	// proceeding.
	if idempotencyData.Signature != "" {
		// TODO Handle the missing from chain case where the transaction was sent but can't
		// yet be found. Retries will make this work, but we should have a better error.
		status, err := checkStatus(ctx, idempotencyData.Signature, client)
		if err != nil {
			return sm.transaction, fmt.Errorf("failed to check transaction status: %w", err)
		}
		if status == rpc.CommitmentConfirmed || status == rpc.CommitmentFinalized {
			entry, err := sm.SetNextState(ctx, paymentLib.Paid)
			if err != nil {
				return sm.transaction, fmt.Errorf("failed to write next state: %w entry: %v", err, entry)
			}
			return sm.transaction, nil
		}
	}

	signer, err := types.AccountFromBase58(sm.signingKey)
	if err != nil {
		return sm.transaction, fmt.Errorf("failed to derive account from base58: %w", err)
	}
	payeeWallet := common.PublicKeyFromString(sm.transaction.To)

	// Convert the amount to base units
	amount := sm.transaction.Amount.Mul(
		decimal.NewFromFloat(math.Pow10(int(sm.splMintDecimals))),
	).BigInt().Uint64()

	instructions, err := makeInstructions(
		signer.PublicKey,
		payeeWallet,
		amount,
		splMintPublicKey,
	)
	if err != nil {
		entry, setStateErr := sm.SetNextState(ctx, paymentLib.Failed)
		if setStateErr != nil {
			return sm.transaction, fmt.Errorf("failed to write next state: %w", setStateErr)
		}

		return sm.transaction, fmt.Errorf("failed to create instructions: %w entry: %v", err, entry)
	}

	// Add idempotency data to the transaction before handling potential transaction errors to ensure
	// it gets persisted for all possible return cases without repetition.
	marshaledIdempotencyData, marshalErr := encodeChainIdempotencyData(idempotencyData)
	if marshalErr != nil {
		return sm.transaction, marshalErr
	}
	sm.transaction.ExternalIdempotency = marshaledIdempotencyData

	// Once idempotency data is persisted, set the state to pending
	entry, err := sm.SetNextState(ctx, paymentLib.Pending)
	if err != nil {
		return sm.transaction, fmt.Errorf("failed to write next state: %w entry: %v", err, entry)
	}

	// Check if idempotency data has expired before (re)submitting the transaction
	//
	// A transaction expires if it could not be committed to a block within 150 slots. Once expired,
	// it can be safely retried with a new blockhash.
	expired := hasBlockHeightExpired(ctx, idempotencyData.SlotTarget, client)
	if expired {
		// Reset idempotency data to ensure a new transaction is submitted in the next iteration
		sm.transaction.ExternalIdempotency = []byte{}
		return sm.transaction, nil
	}

	// Submit the transaction using the same blockhash. We rely on the state machine to retry this
	// until the transaction is either confirmed or the blockhash expires.
	//
	// Ref: https://solana.com/docs/core/transactions/retry#customizing-rebroadcast-logic
	signature, err := sendTransaction(ctx, signer, instructions, idempotencyData.BlockHash, client)
	if err != nil {
		// FIXME - do not fail the transaction for a recoverable error
		entry, setStateErr := sm.SetNextState(ctx, paymentLib.Failed)
		if setStateErr != nil {
			return sm.transaction, fmt.Errorf("failed to write next state: %w", setStateErr)
		}

		return sm.transaction, fmt.Errorf("failed to submit transaction: %w entry: %v", err, entry)
	}

	idempotencyData.Signature = signature
	marshaledIdempotencyData, marshalErr = encodeChainIdempotencyData(idempotencyData)
	if marshalErr != nil {
		return sm.transaction, marshalErr
	}
	sm.transaction.ExternalIdempotency = marshaledIdempotencyData

	return sm.transaction, nil
}

// Fail implements TxStateMachine for the Solana machine.
func (sm *SolanaMachine) Fail(ctx context.Context) (*paymentLib.AuthenticatedPaymentState, error) {
	return sm.SetNextState(ctx, paymentLib.Failed)
}

func makeInstructions(
	feePayer common.PublicKey,
	payeeWallet common.PublicKey,
	amount uint64,
	mint common.PublicKey,
) ([]types.Instruction, error) {
	toAta, _, err := common.FindAssociatedTokenAddress(payeeWallet, mint)
	if err != nil {
		return []types.Instruction{}, err
	}

	fromAta, _, err := common.FindAssociatedTokenAddress(feePayer, mint)
	if err != nil {
		return []types.Instruction{}, err
	}
	ataCreationParam := associated_token_account.CreateIdempotentParam{
		Funder:                 feePayer,
		Owner:                  payeeWallet,
		Mint:                   mint,
		AssociatedTokenAccount: toAta,
	}

	batTransferParam := token.TransferParam{
		From:    fromAta,
		To:      toAta,
		Auth:    feePayer,
		Signers: []common.PublicKey{},
		Amount:  amount,
	}

	return []types.Instruction{
		// Create an associated token account if it doesn't exist
		associated_token_account.CreateIdempotent(ataCreationParam),

		// Transfer BAT to the recipient
		token.Transfer(batTransferParam),
	}, nil
}

func sendTransaction(
	ctx context.Context,
	signer types.Account,
	instructions []types.Instruction,
	blockHash string,
	rpcClient *client.Client,
) (string, error) {
	txn, err := types.NewTransaction(types.NewTransactionParam{
		Message: types.NewMessage(types.NewMessageParam{
			FeePayer:        signer.PublicKey,
			RecentBlockhash: blockHash,
			Instructions:    instructions,
		}),
		Signers: []types.Account{signer},
	})
	// fixme - irrecoverable err
	if err != nil {
		return "", fmt.Errorf("failed to create tx, err: %v", err)
	}

	signature, err := rpcClient.SendTransactionWithConfig(ctx, txn, client.SendTransactionConfig{
		SkipPreflight: true,
	})
	// fixme - recoverable err
	if err != nil {
		return "", fmt.Errorf("failed to send tx, err: %v", err)
	}

	return signature, nil
}

func decodeChainIdempotencyData(data []byte) (chainIdempotencyData, error) {
	idempotencyData := chainIdempotencyData{}
	err := json.Unmarshal(data, &idempotencyData)
	if err != nil {
		return chainIdempotencyData{}, fmt.Errorf("failed to unmarshal idempotency data, err: %v", err)
	}

	return idempotencyData, nil
}

func encodeChainIdempotencyData(data chainIdempotencyData) ([]byte, error) {
	marshaledData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal idempotency data, err: %v", err)
	}

	return marshaledData, nil
}

func fetchChainIdempotencyData(ctx context.Context, client *client.Client) (chainIdempotencyData, error) {
	latestBlockhashResult, err := client.GetLatestBlockhashAndContext(ctx)
	if err != nil {
		return chainIdempotencyData{}, fmt.Errorf("get recent block hash error, err: %v", err)
	}

	return chainIdempotencyData{
		BlockHash:  latestBlockhashResult.Value.Blockhash,
		SlotTarget: latestBlockhashResult.Context.Slot + 150,
	}, nil
}

func decodeOrFetchChainIdempotencyData(
	ctx context.Context,
	data []byte,
	client *client.Client,
) (chainIdempotencyData, error) {
	var (
		idempotencyData chainIdempotencyData
		err             error
	)

	if len(data) > 0 {
		idempotencyData, err = decodeChainIdempotencyData(data)
		if err != nil {
			return idempotencyData, fmt.Errorf("failed to decode idempotency data: %w", err)
		}
	} else {
		idempotencyData, err = fetchChainIdempotencyData(ctx, client)
		if err != nil {
			return idempotencyData, fmt.Errorf("failed to fetch idempotency data: %w", err)
		}
	}


	return idempotencyData, nil
}

func checkStatus(ctx context.Context, signature string, client *client.Client) (rpc.Commitment, error) {
	sigStatus, err := client.GetSignatureStatus(ctx, signature)
	if err != nil {
		return CommitmentUnknown, fmt.Errorf("status check error: %w", err)
	}

	if sigStatus == nil {
		return CommitmentNotFound, nil
	}
	if sigStatus.ConfirmationStatus == nil {
		return CommitmentUnknown, fmt.Errorf("failed to establish commitment status: %v", sigStatus)
	}

	if sigStatus.Err != nil {
		parsedErr, ok := sigStatus.Err.(map[string]interface{})
		if !ok {
			return CommitmentUnknown, fmt.Errorf("status error: %w", err)
		}
		if errVal, ok := parsedErr["InstructionError"]; ok {
			return CommitmentUnknown, fmt.Errorf("instruction error: %v", errVal)
		}
	}
	return *sigStatus.ConfirmationStatus, nil
}

func hasBlockHeightExpired(ctx context.Context, blockHeight uint64, rpcClient *client.Client) bool {
	blockHeightResponse, err := rpcClient.RpcClient.GetBlockHeight(ctx)
	if err != nil {
		// Failing to get the block height is a recoverable error, so we return false
		return false
	}

	return blockHeightResponse.Result > blockHeight
}
