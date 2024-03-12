package payments

import (
	"context"
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

	status, err := sendAndConfirmTransaction(ctx, signer, instructions, client)
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

func getAssociatedTokenAccount(wallet common.PublicKey) (common.PublicKey, error) {
	ataPubKey, _, err := common.FindAssociatedTokenAddress(wallet, batMintPublicKey)
	if err != nil {
		return common.PublicKey{}, err
	}

	return ataPubKey, nil
}

func hasAssociatedTokenAccount(ctx context.Context, wallet common.PublicKey, client *client.Client) (common.PublicKey, error) {
	ataPubKey, err := getAssociatedTokenAccount(wallet)
	if err != nil {
		return common.PublicKey{}, err
	}

	result, err := client.GetAccountInfo(
		ctx,
		ataPubKey.ToBase58(),
	)
	if err != nil {
		return common.PublicKey{}, err
	}

	if result.Owner.ToBase58() == tokenProgramAddress {
		return ataPubKey, nil
	}

	return common.PublicKey{}, nil
}

func getCreateAssociatedTokenAccountInstruction(
	ctx context.Context,
	owner common.PublicKey,
	feePayer common.PublicKey,
	client *client.Client,
) (types.Instruction, error) {
	ata, err := hasAssociatedTokenAccount(ctx, owner, client)
	if err != nil {
		return types.Instruction{}, nil
	}

	return associated_token_account.Create(associated_token_account.CreateParam{
		Funder:                 feePayer,
		Owner:                  owner,
		Mint:                   batMintPublicKey,
		AssociatedTokenAccount: ata,
	}), nil
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
	ataInstruction, err := getCreateAssociatedTokenAccountInstruction(ctx, payeeWallet, feePayer, client)
	if err == nil {
		instructions = append(instructions, ataInstruction)
	}

	to, err := getAssociatedTokenAccount(payeeWallet)
	if err != nil {
		return []types.Instruction{}, err
	}

	from, err := getAssociatedTokenAccount(feePayer)
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
// until the transaction blockhash has expired or the transaction is finalized. The submission
// is repeated on each iteration to handle cases where the transaction is randomly dropped from
// the mempool.
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
) (TransferCode, error) {
	latestBlockhashResult, err := client.GetLatestBlockhashAndContext(ctx)
	if err != nil {
		return TransferFailedCode, fmt.Errorf("get recent block hash error, err: %v", err)
	}

	lastValidBlockHeight := latestBlockhashResult.Context.Slot + 150

	for {
		signature, err := sendTransaction(ctx, signer, instructions, latestBlockhashResult.Value.Blockhash, client)
		if err != nil {
			return TransferFailedCode, fmt.Errorf("send transaction error, err: %v", err)
		}

		sigStatus, err := client.GetSignatureStatus(ctx, signature)
		if err != nil {
			log.Printf("get signature status error, err: %v\n", err)
			continue
		}

		if sigStatus == nil || sigStatus.ConfirmationStatus == nil {
			log.Printf("confirmation status is nil\n")
			continue
		}

		if *sigStatus.ConfirmationStatus == rpc.CommitmentConfirmed {
			return TransferSuccessCode, nil
		}

		blockHashExpired, err := isBlockHeightExpired(ctx, lastValidBlockHeight, client)
		if err != nil {
			return TransferFailedCode, fmt.Errorf("blockhash expired error, err: %v", err)
		}

		if blockHashExpired {
			return TransferDroppedCode, nil
		}

		time.Sleep(500 * time.Millisecond)
	}
}
