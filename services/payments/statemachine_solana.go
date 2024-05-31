package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"

	solanaClient "github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/program/associated_token_account"
	"github.com/blocto/solana-go-sdk/program/compute_budget"
	"github.com/blocto/solana-go-sdk/program/token"
	"github.com/blocto/solana-go-sdk/rpc"
	"github.com/blocto/solana-go-sdk/types"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
	"github.com/mr-tron/base58"
	"github.com/shopspring/decimal"
)

type chainIdempotencyData struct {
	BlockHash   string            `json:"blockHash"`
	SlotTarget  uint64            `json:"slotTarget"`
	Transaction types.Transaction `json:"transaction"`
}

type TxnCommitmentStatus string

const (
	SPLBATMintDecimals   uint8               = 8                                              // Mint decimals for Wormhole wrapped BAT on mainnet
	SPLBATMintAddress    string              = "EPeUFDgHRxs9xxEPVaL6kfGQvCon7jmAWKVUHuux1Tpz" // Mint address for Wormhole wrapped BAT on mainnet
	TxnProcessed         TxnCommitmentStatus = "processed"
	TxnConfirmed         TxnCommitmentStatus = "confirmed"
	TxnFinalized         TxnCommitmentStatus = "finalized"
	TxnNotFound          TxnCommitmentStatus = "notfound"
	TxnUnknown           TxnCommitmentStatus = "unknown"
	TxnInstructionFailed TxnCommitmentStatus = "instructionfailed"
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
	signingKey      []byte
	solanaRpcClient solanaClient.Client
	splMintAddress  string
	splMintDecimals uint8
}

// IsAuthorized overrides the base state machine and checks whether the state machine has 1 or more
// authorization, returning true if so
func (sm *SolanaMachine) IsAuthorized(ctx context.Context) bool {
	return len(sm.getTransaction().Authorizations) >= 1
}

func (sm *SolanaMachine) Authorize(ctx context.Context) (*paymentLib.AuthenticatedPaymentState, error) {
	var err error
	if !sm.IsAuthorized(ctx) {
		return sm.transaction, &InsufficientAuthorizationsError{}
	}

	// In the event of extra calls to Authorize (i.e. a third authorization when only two are needed)
	// ExternalIdempotency will already be generated. Do not generate again. Do not error.
	if sm.transaction.ExternalIdempotency != nil {
		return sm.transaction, nil
	}

	// If the base Authorize method indicates we can proceed, generate, sign, and persist the
	// transaction
	latestBlockhashResult, err := sm.solanaRpcClient.GetLatestBlockhashAndContextWithConfig(
		ctx,
		// Defaults to Finalized, which decreases our available time to retry. Prefer Confirmed
		solanaClient.GetLatestBlockhashConfig{
			Commitment: rpc.CommitmentProcessed,
		},
	)
	if err != nil {
		return sm.transaction, fmt.Errorf("get recent block hash error, err: %w with result: %#v", err, latestBlockhashResult)
	}
	blockHash := latestBlockhashResult.Value.Blockhash
	slotTarget := latestBlockhashResult.Value.LatestValidBlockHeight + 150

	var signer types.Account
	signer, err = types.AccountFromSeed(sm.signingKey)
	if err != nil {
		return sm.transaction, fmt.Errorf("failed to derive account from base58: %w", err)
	}

	if signer.PublicKey.ToBase58() != sm.transaction.From {
		return sm.transaction, fmt.Errorf(
			"transaction 'From' address does not match the derived account: want=%s got=%s",
			signer.PublicKey.ToBase58(),
			sm.transaction.From,
		)
	}

	instructions, err := sm.makeInstructions(
		ctx,
		signer.PublicKey,
		common.PublicKeyFromString(sm.transaction.To),
		// Convert the amount to base units
		sm.transaction.Amount.Mul(
			decimal.NewFromFloat(math.Pow10(int(sm.splMintDecimals))),
		).BigInt().Uint64(),
		common.PublicKeyFromString(sm.splMintAddress),
	)
	if err != nil {
		entry, setStateErr := sm.SetNextState(ctx, paymentLib.Failed)
		if setStateErr != nil {
			return sm.transaction, fmt.Errorf("failed to write next state: %w", setStateErr)
		}

		return sm.transaction, fmt.Errorf("failed to create instructions: %w entry: %v", err, entry)
	}

	txn, err := types.NewTransaction(types.NewTransactionParam{
		Message: types.NewMessage(types.NewMessageParam{
			FeePayer:        signer.PublicKey,
			RecentBlockhash: blockHash,
			Instructions:    instructions,
		}),
		Signers: []types.Account{signer},
	})
	// Failure to generate a transaction means that we have a permanent error, as it is deterministic
	// based on fixed inputs. Fail the transaction.
	if err != nil {
		entry, setStateErr := sm.SetNextState(ctx, paymentLib.Failed)
		if setStateErr != nil {
			return sm.transaction, fmt.Errorf(
				"failed to write next state: %w after instruction generation failed: %w",
				setStateErr,
				err,
			)
		}

		return sm.transaction, fmt.Errorf("failed to create instructions: %w entry: %v", err, entry)
	}

	idempotencyData := chainIdempotencyData{
		BlockHash:   blockHash,
		SlotTarget:  slotTarget,
		Transaction: txn,
	}
	marshaledData, err := json.Marshal(idempotencyData)
	if err != nil {
		return sm.transaction, fmt.Errorf("failed to marshal idempotency data, err: %v", err)
	}

	sm.transaction.ExternalIdempotency = marshaledData

	return sm.SetNextState(ctx, paymentLib.Authorized)
}

// Pay implements TxStateMachine for the Solana machine.
func (sm *SolanaMachine) Pay(ctx context.Context) (*paymentLib.AuthenticatedPaymentState, error) {
	err := ctx.Err()
	if errors.Is(err, context.DeadlineExceeded) {
		return sm.transaction, err
	}

	// Skip if the state is already final
	if sm.transaction.Status == paymentLib.Paid || sm.transaction.Status == paymentLib.Failed {
		return sm.transaction, nil
	}

	// If ExternalIdempotency is missing we have nothing to submit. This indicates a serious uncaught
	// prior error and should Fail immediately.
	if sm.transaction.ExternalIdempotency == nil {
		entry, err := sm.SetNextState(ctx, paymentLib.Failed)
		if err != nil {
			return sm.transaction, fmt.Errorf("failed to write next state: %w entry: %v", err, entry)
		}
		return sm.transaction, fmt.Errorf("external idempotency data was unexpectedly empty")
	}

	idempotencyData := chainIdempotencyData{}
	err = json.Unmarshal(sm.transaction.ExternalIdempotency, &idempotencyData)
	if err != nil {
		return sm.transaction, fmt.Errorf("failed to unmarshal idempotency data, err: %v", err)
	}
	if len(idempotencyData.Transaction.Signatures) != 1 {
		_, setStateErr := sm.SetNextState(ctx, paymentLib.Failed)
		if setStateErr != nil {
			return sm.transaction, fmt.Errorf("failed to write next state: %w", setStateErr)
		}
		return sm.transaction, fmt.Errorf(
			"unexpected number of transaction signatures: %s",
			idempotencyData.Transaction.Signatures,
		)
	}

	b58Signature := base58.Encode(idempotencyData.Transaction.Signatures[0])
	// solanaError stores temporary errors that need to be recorded as the LastError in QLDB but
	// that are expected and do not need to be acted upon. It will only be returned if no other
	// unexpected errors arise from processing.
	var solanaError error
	status, err := checkStatus(ctx, b58Signature, &sm.solanaRpcClient)
	if err != nil {
		// Some errors are expected and we just want to record them. Check if the status check
		// returned such an error and prepare to return it after we attempt to make progress.
		if errors.Is(err, SolanaTransactionNotConfirmedError) ||
			errors.As(err, SolanaTransactionNotFoundError) ||
			errors.As(err, SolanaTransactionUnknownError) {
			solanaError = paymentLib.ProcessingErrorFromError(err, true)
		} else {
			return sm.transaction, fmt.Errorf("failed to check transaction status: %w", err)
		}
	}
	if status == TxnConfirmed || status == TxnFinalized {
		entry, err := sm.SetNextState(ctx, paymentLib.Paid)
		if err != nil {
			return sm.transaction, fmt.Errorf("failed to write next state: %w entry: %v", err, entry)
		}
		return sm.transaction, solanaError
	}

	// Check if idempotency data has expired before (re)submitting the transaction
	//
	// A transaction expires if it could not be committed to a block within 150 slots. Once expired,
	// it can be safely retried with a new blockhash. However, for the initial implementation we will
	// fail transactions that are dropped.
	blockHeightResponse, err := sm.solanaRpcClient.RpcClient.GetBlockHeightWithConfig(
		ctx,
		rpc.GetBlockHeightConfig{
			Commitment: rpc.CommitmentProcessed,
		},
	)
	if err != nil {
		// Failing to get the block height is a recoverable error, so return without state change
		return sm.transaction, fmt.Errorf("failed to get block height: %w", err)
	}
	if blockHeightResponse.Result > idempotencyData.SlotTarget {
		_, setStateErr := sm.SetNextState(ctx, paymentLib.Failed)
		if setStateErr != nil {
			return sm.transaction, fmt.Errorf("failed to write next state: %w", setStateErr)
		}
		return sm.transaction, errors.New("slot target exceeded")
	}

	// Submit the transaction using the same blockhash. We rely on the state machine to retry this
	// until the transaction is either confirmed or the blockhash expires.
	//
	// Ref: https://solana.com/docs/core/transactions/retry#customizing-rebroadcast-logic
	signature, err := sm.solanaRpcClient.SendTransactionWithConfig(
		ctx,
		idempotencyData.Transaction,
		solanaClient.SendTransactionConfig{
			MaxRetries:          100,
			PreflightCommitment: rpc.CommitmentProcessed,
			SkipPreflight:       true,
		},
	)
	if err != nil {
		// Introspect the RPC error looking for specific error codes
		var mapErr map[string]interface{}
		unmarshalErr := json.Unmarshal([]byte(err.Error()), &mapErr)
		if unmarshalErr != nil {
			return sm.transaction, fmt.Errorf("failed to submit transaction: %w", unmarshalErr)
		}
		data, ok := mapErr["data"].(map[string]interface{})
		if !ok {
			return sm.transaction, fmt.Errorf("failed to assert error data to map: %v", mapErr["data"])
		}
		if data["err"] == "BlockhashNotFound" {
			_, setStateErr := sm.SetNextState(ctx, paymentLib.Failed)
			if setStateErr != nil {
				return sm.transaction, fmt.Errorf("failed to write next state: %w", setStateErr)
			}
			return sm.transaction, fmt.Errorf("block hash expired: %w", err)
		}
		return sm.transaction, fmt.Errorf("failed to submit transaction: %w", err)
	}

	if signature != b58Signature {
		_, setStateErr := sm.SetNextState(ctx, paymentLib.Failed)
		if setStateErr != nil {
			return sm.transaction, fmt.Errorf("failed to write next state: %w", setStateErr)
		}
		return sm.transaction, fmt.Errorf(
			"submitted signature did not match idempotency data: expected %s but got %s",
			b58Signature,
			signature,
		)
	}

	// Once transaction is submitted set the state to pending
	entry, err := sm.SetNextState(ctx, paymentLib.Pending)
	if err != nil {
		return sm.transaction, fmt.Errorf("failed to write next state: %w entry: %v", err, entry)
	}

	return sm.transaction, solanaError
}

// Fail implements TxStateMachine for the Solana machine.
func (sm *SolanaMachine) Fail(ctx context.Context) (*paymentLib.AuthenticatedPaymentState, error) {
	return sm.SetNextState(ctx, paymentLib.Failed)
}

func (sm *SolanaMachine) makeInstructions(
	ctx context.Context,
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

	computeUnitPrice := 100
	if envCUPrice, err := strconv.Atoi(os.Getenv("SOLANA_COMPUTE_UNIT_PRICE")); err == nil {
		computeUnitPrice = envCUPrice
	}

	computeUnitLimit := 50000
	if envCULimit, err := strconv.Atoi(os.Getenv("SOLANA_COMPUTE_UNIT_LIMIT")); err == nil {
		computeUnitLimit = envCULimit
	}

	priceParam := compute_budget.SetComputeUnitPriceParam{MicroLamports: uint64(computeUnitPrice)}
	budgetParam := compute_budget.SetComputeUnitLimitParam{Units: uint32(computeUnitLimit)}

	return []types.Instruction{
		// Set the transaction budget
		compute_budget.SetComputeUnitLimit(budgetParam),

		// Set the transaction price parameter
		compute_budget.SetComputeUnitPrice(priceParam),

		// Create an associated token account if it doesn't exist
		associated_token_account.CreateIdempotent(ataCreationParam),

		// Transfer BAT to the recipient
		token.Transfer(batTransferParam),
	}, nil
}

func checkStatus(
	ctx context.Context,
	signature string,
	client *solanaClient.Client,
) (TxnCommitmentStatus, error) {
	sigStatus, err := client.GetSignatureStatus(ctx, signature)
	if err != nil {
		return TxnUnknown, fmt.Errorf("status check error: %w", err)
	}

	if sigStatus == nil {
		return TxnNotFound, SolanaTransactionNotFoundError
	}
	if sigStatus.ConfirmationStatus == nil {
		return TxnUnknown, SolanaTransactionUnknownError
	}

	if sigStatus.Err != nil {
		parsedErr, ok := sigStatus.Err.(map[string]interface{})
		if !ok {
			return TxnUnknown, fmt.Errorf("status error: %w", err)
		}
		if errVal, ok := parsedErr["InstructionError"]; ok {
			return TxnInstructionFailed, fmt.Errorf("instruction error: %v", errVal)
		}
	}

	switch *sigStatus.ConfirmationStatus {
	case rpc.CommitmentProcessed:
		return TxnProcessed, SolanaTransactionNotConfirmedError
	case rpc.CommitmentConfirmed:
		return TxnConfirmed, nil
	case rpc.CommitmentFinalized:
		return TxnFinalized, nil
	default:
		return TxnUnknown, nil
	}
}
