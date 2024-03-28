package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/program/associated_token_account"
	"github.com/blocto/solana-go-sdk/program/token"
	"github.com/blocto/solana-go-sdk/rpc"
	"github.com/blocto/solana-go-sdk/types"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
	"github.com/shopspring/decimal"
)

type TransferCode int

const (
	TransferSuccessCode TransferCode = iota
	TransferDroppedCode
	TransferFailedCode
)

func (tc TransferCode) String() string {
	return [...]string{
		"TransferSuccessCode",
		"TransferDroppedCode",
		"TransferFailedCode",
	}[tc]
}

type chainIdempotencyData struct {
	BlockHash string `json:"blockHash"`
	Signature string `json:"signature"`
}

const (
	batMintDecimals     int    = 8
	batMintAddress      string = "EPeUFDgHRxs9xxEPVaL6kfGQvCon7jmAWKVUHuux1Tpz"
	tokenProgramAddress string = "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"
)

var (
	batMintPublicKey = common.PublicKeyFromString(batMintAddress)
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
}

// Pay implements TxStateMachine for the Solana machine.
func (sm *SolanaMachine) Pay(ctx context.Context) (*paymentLib.AuthenticatedPaymentState, error) {
	err := ctx.Err()
	if errors.Is(err, context.DeadlineExceeded) {
		return sm.transaction, err
	}

	// Do nothing if the state is already final
	if sm.transaction.Status == paymentLib.Paid || sm.transaction.Status == paymentLib.Failed {
		return sm.transaction, nil
	}

	client := client.NewClient(sm.solanaRpcEndpoint)
	signer, _ := types.AccountFromBase58(sm.signingKey)
	payeeWallet := common.PublicKeyFromString(sm.transaction.To)

	// Convert the amount to base units
	amount := sm.transaction.Amount.Mul(
		decimal.NewFromFloat(math.Pow10(batMintDecimals)),
	).BigInt().Uint64()

	instructions, err := makeInstructions(
		ctx,
		signer.PublicKey,
		payeeWallet,
		amount,
		client,
	)
	if err != nil {
		entry, setStateErr := sm.SetNextState(ctx, paymentLib.Failed)
		if setStateErr != nil {
			return sm.transaction, fmt.Errorf("failed to write next state: %w", setStateErr)
		}

		return sm.transaction, fmt.Errorf("failed to create instructions: %w entry: %v", err, entry)
	}

	status, idempotencyData, err := sendAndConfirmTransaction(ctx, signer, instructions, client)

	// Add idempotency data to the transaction before handling potential transaction errors to ensure
	// it gets persisted all possible return cases without repetition.
	marshaledIdempotencyData, marshalErr := json.Marshal(idempotencyData)
	if marshalErr != nil {
		return nil, fmt.Errorf("failed to marshal idempotency data: %w", marshalErr)
	}
	sm.transaction.ExternalIdempotency = marshaledIdempotencyData

	// Handle transaction submission errors
	if err != nil {
		entry, setStateErr := sm.SetNextState(ctx, paymentLib.Failed)
		if setStateErr != nil {
			return sm.transaction, fmt.Errorf("failed to write next state: %w", setStateErr)
		}

		return sm.transaction, fmt.Errorf("failed to submit transaction: %w entry: %v", err, entry)
	}

	if status == TransferSuccessCode {
		entry, err := sm.SetNextState(ctx, paymentLib.Paid)
		if err != nil {
			return sm.transaction, fmt.Errorf("failed to write next state: %w entry: %v", err, entry)
		}

		return sm.transaction, nil
	}

	entry, err := sm.SetNextState(ctx, paymentLib.Failed)
	if err != nil {
		return sm.transaction, fmt.Errorf("failed to write next state: %w entry: %v", err, entry)
	}

	return sm.transaction, fmt.Errorf("failed to submit transaction, status: %v", status)
}

// Fail implements TxStateMachine for the Solana machine.
func (sm *SolanaMachine) Fail(ctx context.Context) (*paymentLib.AuthenticatedPaymentState, error) {
	return sm.SetNextState(ctx, paymentLib.Failed)
}

func hasAssociatedTokenAccount(ctx context.Context, wallet common.PublicKey, client *client.Client) (common.PublicKey, bool, error) {
	ata, _, err := common.FindAssociatedTokenAddress(wallet, batMintPublicKey)
	if err != nil {
		return common.PublicKey{}, false, err
	}

	result, err := client.GetAccountInfo(
		ctx,
		ata.ToBase58(),
	)
	if err != nil {
		return common.PublicKey{}, false, err
	}

	if result.Owner.ToBase58() == tokenProgramAddress {
		return ata, true, nil
	}

	return ata, false, nil
}

func getCreateAssociatedTokenAccountInstruction(
	ctx context.Context,
	owner common.PublicKey,
	feePayer common.PublicKey,
	client *client.Client,
) (types.Instruction, bool, error) {
	ata, hasAta, err := hasAssociatedTokenAccount(ctx, owner, client)
	if err != nil {
		return types.Instruction{}, false, err
	}

	if hasAta {
		return types.Instruction{}, true, nil
	}

	return associated_token_account.Create(associated_token_account.CreateParam{
		Funder:                 feePayer,
		Owner:                  owner,
		Mint:                   batMintPublicKey,
		AssociatedTokenAccount: ata,
	}), false, nil
}

func getTransferInstruction(
	from common.PublicKey,
	to common.PublicKey,
	amount uint64,
	feePayer common.PublicKey,
) types.Instruction {
	return token.Transfer(token.TransferParam{
		From:    from,
		To:      to,
		Auth:    feePayer,
		Signers: []common.PublicKey{},
		Amount:  amount,
	})
}

func makeInstructions(ctx context.Context, feePayer common.PublicKey, payeeWallet common.PublicKey, amount uint64, client *client.Client) ([]types.Instruction, error) {
	instructions := make([]types.Instruction, 0)

	// Create an associated token account if it doesn't exist
	ataInstruction, hasAta, err := getCreateAssociatedTokenAccountInstruction(ctx, payeeWallet, feePayer, client)
	if err != nil {
		return []types.Instruction{}, err
	}

	if !hasAta {
		instructions = append(instructions, ataInstruction)
	}

	to, _, err := common.FindAssociatedTokenAddress(payeeWallet, batMintPublicKey)
	if err != nil {
		return []types.Instruction{}, err
	}

	from, _, err := common.FindAssociatedTokenAddress(feePayer, batMintPublicKey)
	if err != nil {
		return []types.Instruction{}, err
	}

	// Transfer BAT to the recipient
	transferInstruction := getTransferInstruction(
		from,
		to,
		amount,
		feePayer,
	)
	instructions = append(instructions, transferInstruction)

	return instructions, nil
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
	if err != nil {
		return "", fmt.Errorf("failed to create tx, err: %v", err)
	}

	signature, err := rpcClient.SendTransactionWithConfig(ctx, txn, client.SendTransactionConfig{
		SkipPreflight: true,
	})
	if err != nil {
		return "", fmt.Errorf("failed to send tx, err: %v", err)
	}

	return signature, nil
}

func isBlockHeightExpired(ctx context.Context, lastValidBlockHeight uint64, rpcClient *client.Client) (bool, error) {
	blockHeightResponse, err := rpcClient.RpcClient.GetBlockHeight(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get block height, err: %v", err)
	}

	return blockHeightResponse.Result > lastValidBlockHeight, nil
}

// sendAndConfirmTransaction signs and submits a transaction, then waits for confirmation.
//
// Once the transaction is sent, this method continuously polls on status of the signature
// until the transaction blockhash has expired or the transaction is comfirmed/finalized.
// The submission is repeated on each iteration to handle cases where the transaction is
// randomly dropped from the mempool.
//
// Ref: https://solana.com/docs/core/transactions/retry#customizing-rebroadcast-logic
//
// The method returns a TransferCode indicating the status of the transaction:
//   - TransferSuccessCode: "comfirmed" commitment level achieved in the cluster.
//   - TransferDroppedCode: failed to achieve a commitment within 150 slots.
//   - TransferFailedCode:  submission failed for some other reason, typically an RPC error.
func sendAndConfirmTransaction(
	ctx context.Context,
	signer types.Account,
	instructions []types.Instruction,
	client *client.Client,
) (TransferCode, chainIdempotencyData, error) {
	latestBlockhashResult, err := client.GetLatestBlockhashAndContext(ctx)
	if err != nil {
		return TransferFailedCode, chainIdempotencyData{}, fmt.Errorf("get recent block hash error, err: %v", err)
	}

	lastValidBlockHeight := latestBlockhashResult.Context.Slot + 150
	idempotencyData := chainIdempotencyData{
		BlockHash: latestBlockhashResult.Value.Blockhash,
	}

	for {
		blockHashExpired, err := isBlockHeightExpired(ctx, lastValidBlockHeight, client)
		if err != nil {
			return TransferFailedCode, fmt.Errorf("blockhash expired error, err: %v", err)
		}

		if blockHashExpired {
			return TransferDroppedCode, nil
		}

		signature, err := sendTransaction(ctx, signer, instructions, latestBlockhashResult.Value.Blockhash, client)
		if err != nil {
			return TransferFailedCode, idempotencyData, fmt.Errorf("send transaction error, err: %v", err)
		}
		idempotencyData.Signature = signature

		sigStatus, err := client.GetSignatureStatus(ctx, signature)
		if err != nil {
			log.Printf("get signature status error, err: %v\n", err)
			continue
		}

		if sigStatus == nil || sigStatus.ConfirmationStatus == nil {
			log.Printf("confirmation status is nil\n")
			continue
		}

		if *sigStatus.ConfirmationStatus == rpc.CommitmentConfirmed || *sigStatus.ConfirmationStatus == rpc.CommitmentFinalized {
			return TransferSuccessCode, nil
		}

		blockHashExpired, err := isBlockHeightExpired(ctx, lastValidBlockHeight, client)
		if err != nil {
			return TransferFailedCode, idempotencyData, fmt.Errorf("blockhash expired error, err: %v", err)
		}

		if blockHashExpired {
			return TransferDroppedCode, idempotencyData, nil
		}

		time.Sleep(500 * time.Millisecond)
	}
}
