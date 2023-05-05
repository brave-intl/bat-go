package payments

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/config"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/brave-intl/bat-go/libs/nitro"
	"github.com/google/uuid"
	"strings"
	"time"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/qldb"
	qldbTypes "github.com/aws/aws-sdk-go-v2/service/qldb/types"
	"github.com/aws/aws-sdk-go-v2/service/qldbsession"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/awslabs/amazon-qldb-driver-go/v3/qldbdriver"
	"github.com/brave-intl/bat-go/libs/custodian/provider"
	appaws "github.com/brave-intl/bat-go/libs/nitro/aws"
	"github.com/hashicorp/vault/shamir"
	"golang.org/x/exp/slices"

	"encoding/base64"
	"encoding/json"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/brave-intl/bat-go/libs/logging"
	appsrv "github.com/brave-intl/bat-go/libs/service"
)

// Service - struct definition of payments service
type Service struct {
	// concurrent safe
	datastore  wrappedQldbDriverAPI
	custodians map[string]provider.Custodian

	baseCtx          context.Context
	secretMgr        appsrv.SecretManager
	keyShares        [][]byte
	kmsDecryptKeyArn string
	kmsSigningKeyId  string
	kmsSigningClient *kms.Client
	pubKey           []byte
}

// qldbPaymentTransitionHistoryEntryBlockAddress defines blockAddress data for QLDBPaymentTransitionHistoryEntry
type qldbPaymentTransitionHistoryEntryBlockAddress struct {
	StrandID   string `ion:"strandID"`
	SequenceNo int64  `ion:"sequenceNo"`
}

// QLDBPaymentTransitionHistoryEntryHash defines hash for QLDBPaymentTransitionHistoryEntry
type QLDBPaymentTransitionHistoryEntryHash string

// QLDBPaymentTransitionHistoryEntrySignature defines signature for QLDBPaymentTransitionHistoryEntry
type QLDBPaymentTransitionHistoryEntrySignature []byte

// QLDBPaymentTransitionHistoryEntryData defines data for QLDBPaymentTransitionHistoryEntry
type QLDBPaymentTransitionHistoryEntryData struct {
	Signature      []byte     `ion:"signature"`
	Data           []byte     `ion:"data"`
	IdempotencyKey *uuid.UUID `ion:"idempotencyKey"`
}

// QLDBPaymentTransitionHistoryEntryMetadata defines metadata for QLDBPaymentTransitionHistoryEntry
type QLDBPaymentTransitionHistoryEntryMetadata struct {
	ID      string    `ion:"id"`
	Version int64     `ion:"version"`
	TxTime  time.Time `ion:"txTime"`
	TxID    string    `ion:"txId"`
}

// QLDBPaymentTransitionHistoryEntry defines top level entry for a QLDB transaction
type QLDBPaymentTransitionHistoryEntry struct {
	BlockAddress qldbPaymentTransitionHistoryEntryBlockAddress `ion:"blockAddress"`
	Hash         QLDBPaymentTransitionHistoryEntryHash         `ion:"hash"`
	Data         QLDBPaymentTransitionHistoryEntryData         `ion:"data"`
	Metadata     QLDBPaymentTransitionHistoryEntryMetadata     `ion:"metadata"`
}

// configureKMSKey creates the enclave kms key which is only decrypt capable with enclave attestation.
func (s *Service) configureKMSKey(ctx context.Context) error {
	// perform enclave attestation
	nonce := make([]byte, 64)
	_, err := rand.Read(nonce)
	if err != nil {
		return fmt.Errorf("failed to create nonce for attestation: %w", err)
	}
	document, err := nitro.Attest(nonce, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to create attestation document: %w", err)
	}
	var logger = logging.Logger(ctx, "payments.configureKMSKey")
	logger.Debug().Msgf("document: %+v", document)

	// get the aws configuration loaded
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load aws configuration: %w", err)
	}

	// TODO: get the pcr values for the condition from the document ^^
	imageSha384 := ""
	pcr0 := ""
	pcr1 := ""
	pcr2 := ""

	// get the secretsmanager id from ctx for the template
	templateSecretID, ok := ctx.Value(appctx.EnclaveDecryptKeyTemplateSecretIDCTXKey).(string)
	if !ok {
		return errors.New("template secret id for enclave decrypt key not found on context")
	}

	smClient := secretsmanager.NewFromConfig(cfg)
	o, err := smClient.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(templateSecretID),
	})

	if err != nil {
		return fmt.Errorf("failed to get key policy template from secrets manager: %w", err)
	}

	if o.SecretString == nil {
		return errors.New("secret is not defined in secrets manager")
	}

	keyPolicy := *o.SecretString
	keyPolicy = strings.ReplaceAll(keyPolicy, "<IMAGE_SHA384>", imageSha384)
	keyPolicy = strings.ReplaceAll(keyPolicy, "<PCR0>", pcr0)
	keyPolicy = strings.ReplaceAll(keyPolicy, "<PCR1>", pcr1)
	keyPolicy = strings.ReplaceAll(keyPolicy, "<PCR2>", pcr2)

	kClient := kms.NewFromConfig(cfg)

	input := &kms.CreateKeyInput{
		Policy: aws.String(keyPolicy),
	}

	result, err := kClient.CreateKey(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to make key: %w", err)
	}

	s.kmsDecryptKeyArn = *result.KeyMetadata.KeyId
	return nil
}

// NewService creates a service using the passed datastore and clients configured from the environment
func NewService(ctx context.Context) (context.Context, *Service, error) {
	var logger = logging.Logger(ctx, "payments.NewService")

	service := &Service{
		baseCtx:   ctx,
		secretMgr: &awsClient{},
	}

	if err := service.configureKMSKey(ctx); err != nil {
		logger.Fatal().Msg("could not create kms secret decryption key")
	}

	if err := service.configureDatastore(ctx); err != nil {
		logger.Fatal().Msg("could not configure datastore")
	}

	// set up our custodian integrations
	upholdCustodian, err := provider.New(ctx, provider.Config{Provider: provider.Uphold})
	if err != nil {
		logger.Error().Err(err).Msg("failed to create uphold custodian")
		return ctx, nil, fmt.Errorf("failed to create uphold custodian: %w", err)
	}
	geminiCustodian, err := provider.New(ctx, provider.Config{Provider: provider.Gemini})
	if err != nil {
		logger.Error().Err(err).Msg("failed to create gemini custodian")
		return ctx, nil, fmt.Errorf("failed to create gemini custodian: %w", err)
	}
	bitflyerCustodian, err := provider.New(ctx, provider.Config{Provider: provider.Bitflyer})
	if err != nil {
		logger.Error().Err(err).Msg("failed to create bitflyer custodian")
		return ctx, nil, fmt.Errorf("failed to create bitflyer custodian: %w", err)
	}

	service.custodians = map[string]provider.Custodian{
		provider.Uphold:   upholdCustodian,
		provider.Gemini:   geminiCustodian,
		provider.Bitflyer: bitflyerCustodian,
	}

	return ctx, service, nil
}

// DecryptBootstrap - use service keyShares to reconstruct the decryption key
func (s *Service) DecryptBootstrap(ctx context.Context, ciphertext []byte) (map[appctx.CTXKey]interface{}, error) {
	// combine the service configured key shares
	key, err := shamir.Combine(s.keyShares)
	if err != nil {
		return nil, fmt.Errorf("failed to combine keyShares: %w", err)
	}

	// pull nonce off ciphertext blob
	var nonce [32]byte
	copy(nonce[:], ciphertext[:32]) // nonce is in first 32 bytes of ciphertext

	// shove key into array
	var k [32]byte
	copy(k[:], key) // nonce is in first 32 bytes of ciphertext

	// decrypted is the encryption key used to decrypt secrets now
	v, err := cryptography.DecryptMessage(k, ciphertext[32:], nonce[:])
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secrets: %w", err)
	}

	// decrypted message is a json blob, convert to our output
	var output = map[appctx.CTXKey]interface{}{}
	err = json.Unmarshal([]byte(v), &output)
	if err != nil {
		return nil, fmt.Errorf("failed to json decode secrets: %w", err)
	}

	return output, nil
}

// BuildSigningBytes returns the bytes that should be signed over when creating a signature
// for a QLDBPaymentTransitionHistoryEntry.
func (e *QLDBPaymentTransitionHistoryEntry) BuildSigningBytes() ([]byte, error) {
	marshaled, err := ion.MarshalBinary(e.Data.Data)
	if err != nil {
		return nil, fmt.Errorf("Ion marshal failed: %w", err)
	}

	return marshaled, nil
}

// ValueHolder converts a QLDBPaymentTransitionHistoryEntry into a QLDB SDK ValueHolder
func (b qldbPaymentTransitionHistoryEntryBlockAddress) ValueHolder() *qldbTypes.ValueHolder {
	stringValue := fmt.Sprintf("{strandId:\"%s\",sequenceNo:%d}", b.StrandID, b.SequenceNo)
	return &qldbTypes.ValueHolder{
		IonText: &stringValue,
	}
}

// getTransitionHistory returns a slice of entries representing the entire state history
// for a given id.
func getTransitionHistory(txn wrappedQldbTxnAPI, id *uuid.UUID) ([]QLDBPaymentTransitionHistoryEntry, error) {
	result, err := txn.Execute("SELECT * FROM history(transactions) AS h WHERE h.metadata.id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("QLDB transaction failed: %w", err)
	}
	var collectedData []QLDBPaymentTransitionHistoryEntry
	for result.Next(txn) {
		var data QLDBPaymentTransitionHistoryEntry
		err := ion.Unmarshal(result.GetCurrentData(), &data)
		if err != nil {
			return nil, fmt.Errorf("Ion unmarshal failed: %w", err)
		}
		collectedData = append(collectedData, data)
	}
	if len(collectedData) > 0 {
		return collectedData, nil
	}
	return nil, nil
}

// validateTransitionHistory returns whether a slice of entries representing the entire state history for a given id
// include exclusively valid transitions. It also verifies IdempotencyKeys among states and the Merkle tree position of each state
func validateTransitionHistory(ctx context.Context, idempotencyKey *uuid.UUID, transactionHistory []QLDBPaymentTransitionHistoryEntry) (bool, error) {
	var (
		reason error
		err    error
	)
	for i, transaction := range transactionHistory {
		var transactionData Transaction
		err = json.Unmarshal(transaction.Data.Data, &transactionData)
		if err != nil {
			return false, fmt.Errorf("failed to unmarshal transation data: %w", err)
		}
		// GetIdempotencyKey will verify that the ID is internally consistent within the Transaction.
		dataIdempotencyKey, err := transactionData.GetIdempotencyKey(ctx)
		if err != nil {
			return false, fmt.Errorf("IdempotencyKey invalid: %w", err)
		}
		// The data object is serialized in QLDB, but when deserialized should contain an IdempotencyKey that matches
		// the IdempotencyKey on the top level of the QLDB record
		if dataIdempotencyKey != idempotencyKey {
			return false, fmt.Errorf("top level IdempotencyKey does not match Transaction IdempotencyKey: %s, %s", dataIdempotencyKey, idempotencyKey)
		}
		transactionState := transactionData.State
		// Transitions must always start at 0
		if i == 0 {
			if transactionState != 0 {
				return false, errors.New("initial state is not valid")
			}
			continue
		}
		var previousTransitionData Transaction
		err = json.Unmarshal(transactionHistory[i-1].Data.Data, &previousTransitionData)
		if err != nil {
			return false, fmt.Errorf("failed to unmarshal previous transition history record: %w", err)
		}
		previousIdempotencyKey, err := previousTransitionData.GetIdempotencyKey(ctx)
		if err != nil {
			return false, fmt.Errorf("IdempotencyKey invalid: %w", err)
		}
		// The IdempotencyKeys of all records in the transition history of a Transaction should match
		if dataIdempotencyKey != previousIdempotencyKey {
			return false, fmt.Errorf("IdempotencyKeys in transition history do not match: %s, %s", idempotencyKey.String(), previousIdempotencyKey.String())
		}
		previousTransitionState := previousTransitionData.State
		// New transaction state should be present in the list of valid next states for the "previous" (current) state.
		if !slices.Contains(Transitions[previousTransitionState], transactionState) {
			return false, errors.New("invalid state transition")
		}
	}
	return true, reason
}

// revisionValidInTree verifies a document revision in QLDB using a digest and the Merkle
// hashes to re-derive the digest
func revisionValidInTree(
	ctx context.Context,
	client wrappedQldbSdkClient,
	transaction *QLDBPaymentTransitionHistoryEntry,
) (bool, error) {
	ledgerName := "LEDGER_NAME"
	digest, err := client.GetDigest(ctx, &qldb.GetDigestInput{Name: &ledgerName})

	if err != nil {
		return false, fmt.Errorf("Failed to get digest: %w", err)
	}

	revision, err := client.GetRevision(ctx, &qldb.GetRevisionInput{
		BlockAddress:     transaction.BlockAddress.ValueHolder(),
		DocumentId:       &transaction.Metadata.ID,
		Name:             &ledgerName,
		DigestTipAddress: digest.DigestTipAddress,
	})

	if err != nil {
		return false, fmt.Errorf("Failed to get revision: %w", err)
	}
	var (
		hashes           [][32]byte
		concatenatedHash [32]byte
	)

	// This Ion unmarshal gives us the hashes as bytes. The documentation implies that
	// these are base64 encoded strings, but testing indicates that is not the case.
	err = ion.UnmarshalString(*revision.Proof.IonText, &hashes)

	if err != nil {
		return false, fmt.Errorf("Failed to unmarshal revision proof: %w", err)
	}

	for i, providedHash := range hashes {
		// During the first integration concatenatedHash hasn't been populated.
		// Populate it with the hash from the provided transaction.
		if i == 0 {
			decodedHash, err := base64.StdEncoding.DecodeString(string(transaction.Hash))
			if err != nil {
				return false, err
			}
			copy(concatenatedHash[:], decodedHash)
		}
		// QLDB determines hash order by comparing the hashes byte by byte until
		// one is greater than the other. The larger becomes the left hash and the
		// smaller becomes the right hash for the next phase of hash generation.
		// This is not documented, but can be inferred from the Java reference
		// implementation here: https://github.com/aws-samples/amazon-qldb-dmv-sample-java/blob/master/src/main/java/software/amazon/qldb/tutorial/Verifier.java#L60
		sortedHashes, err := sortHashes(providedHash[:], concatenatedHash[:])
		if err != nil {
			return false, err
		}
		// Concatenate the hashes and then hash the result to get the next hash
		// in the tree.
		concatenatedHash = sha256.Sum256(append(sortedHashes[0], sortedHashes[1]...))
	}

	// The digest comes to us as a base64 encoded string. We need to decode it before
	// using it.
	decodedDigest, err := base64.StdEncoding.DecodeString(string(digest.Digest))

	if err != nil {
		return false, fmt.Errorf("Failed to base64 decode digest: %w", err)
	}

	if string(concatenatedHash[:]) == string(decodedDigest) {
		return true, nil
	}

	return false, nil
}

func transitionHistoryIsValid(ctx context.Context, txn wrappedQldbTxnAPI, id *uuid.UUID) (bool, *QLDBPaymentTransitionHistoryEntry, error) {
	// Fetch all historical states for this record
	result, err := getTransitionHistory(txn, id)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get transition history: %w", err)
	}
	if len(result) < 1 {
		return false, nil, errors.New("record not found")
	}
	// Ensure that all state changes in record history were valid
	valid, err := validateTransitionHistory(ctx, id, result)
	if err != nil {
		return false, nil, fmt.Errorf("failed to validate history: %w", err)
	}
	if valid {
		// We only want the latest state of this record once its
		// history is verified
		return true, &result[0], nil
	}
	return false, &result[0], fmt.Errorf("invalid transition history: %w", err)
}

// getQLDBObject returns the latest version of an entry for a given ID after doing all requisite validation
func getQLDBObject(
	ctx context.Context,
	txn wrappedQldbTxnAPI,
	client wrappedQldbSdkClient,
	id *uuid.UUID,
) (*QLDBPaymentTransitionHistoryEntry, error) {
	valid, result, err := transitionHistoryIsValid(ctx, txn, id)
	if err != nil || !valid {
		return nil, fmt.Errorf("failed to validate transition history: %w", err)
	}
	// If no record was found, return nothing
	if result == nil {
		return nil, nil
	}
	merkleValid, err := revisionValidInTree(ctx, client, result)
	if err != nil {
		return nil, fmt.Errorf("failed to verify Merkle tree: %w", err)
	}
	if !merkleValid {
		return nil, fmt.Errorf("invalid Merkle tree for record: %#v", result)
	}
	return result, nil
}

// GetQLDBObject returns the latest version of a record from QLDB if it exists, after doing all requisite validation
func GetQLDBObject(
	ctx context.Context,
	qldbDriver wrappedQldbDriverAPI,
	qldbSDK wrappedQldbSdkClient,
	id *uuid.UUID,
) (*QLDBPaymentTransitionHistoryEntry, error) {
	data, err := qldbDriver.Execute(context.Background(), func(txn qldbdriver.Transaction) (interface{}, error) {
		entry, err := getQLDBObject(ctx, txn, qldbSDK, id)
		if err != nil {
			return nil, fmt.Errorf("failed to get QLDB record: %w", err)
		}
		return entry, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query QLDB: %w", err)
	}
	assertedData, ok := data.(QLDBPaymentTransitionHistoryEntry)
	if !ok {
		return nil, fmt.Errorf("database response was the wrong type: %w", err)
	}
	return &assertedData, nil
}

// WriteQLDBObject persists an object in a transaction after verifying that its change
// represents a valid state transition.
func WriteQLDBObject(
	ctx context.Context,
	qldbDriver wrappedQldbDriverAPI,
	qldbSDK wrappedQldbSdkClient,
	kmsClient wrappedKMSClient,
	transaction *Transaction,
) (*Transaction, error) {
	_, err := qldbDriver.Execute(ctx, func(txn qldbdriver.Transaction) (interface{}, error) {
		// Determine if the transaction already exists or if it needs to be initialized. This call will do all necessary
		// record and history validation if they exist for this record
		record, err := getQLDBObject(ctx, txn, qldbSDK, transaction.IdempotencyKey)
		if err != nil {
			return nil, fmt.Errorf("failed to query QLDB: %w", err)
		}
		signingBytes := transaction.BuildSigningBytes()
		// @TODO: Get key ID
		todoString := "nil"
		if err != nil {
			return nil, fmt.Errorf("JSON marshal failed: %w", err)
		}
		signingOutput, _ := kmsClient.Sign(ctx, &kms.SignInput{
			KeyId:            &todoString,
			Message:          signingBytes,
			SigningAlgorithm: kmsTypes.SigningAlgorithmSpecEcdsaSha256,
		})
		transaction.Signature = signingOutput.Signature

		if record == nil {
			return txn.Execute("INSERT INTO transactions ?", transaction)
		} else {
			return txn.Execute("UPDATE transactions SET state = ? WHERE id = ?", transaction.State, transaction.IdempotencyKey)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("QLDB write execution failed: %w", err)
	}
	return transaction, nil
}

func sortHashes(a, b []byte) ([][]byte, error) {
	if len(a) != len(b) {
		return nil, errors.New("provided hashes do not have matching length")
	}
	for i := 0; i < len(a); i++ {
		if a[i] > b[i] {
			return [][]byte{a, b}, nil
		} else if a[i] < b[i] {
			return [][]byte{b, a}, nil
		}
	}
	return [][]byte{a, b}, nil
}

func isQLDBReady(ctx context.Context) bool {
	logger := logging.Logger(ctx, "payments.isQLDBReady")
	// decrypt the aws region
	qldbArn, qldbArnOK := ctx.Value(appctx.PaymentsQLDBRoleArnCTXKey).(string)
	// decrypt the aws region
	region, regionOK := ctx.Value(appctx.AWSRegionCTXKey).(string)
	// get proxy address for outbound
	egressAddr, egressOK := ctx.Value(appctx.EgressProxyAddrCTXKey).(string)
	if regionOK && egressOK && qldbArnOK {
		return true
	}
	logger.Warn().
		Str("region", region).
		Str("egressAddr", egressAddr).
		Str("qldbArn", qldbArn).
		Msg("service is not configured to access qldb")
	return false
}

// newQLDBDatastore - create a new qldbDatastore
func newQLDBDatastore(ctx context.Context) (*qldbdriver.QLDBDriver, error) {
	logger := logging.Logger(ctx, "payments.newQLDBDatastore")

	if !isQLDBReady(ctx) {
		return nil, ErrNotConfiguredYet
	}

	egressProxyAddr, ok := ctx.Value(appctx.EgressProxyAddrCTXKey).(string)
	if !ok {
		return nil, fmt.Errorf("failed to get egress proxy for qldb")
	}

	// decrypt the aws region
	region, ok := ctx.Value(appctx.AWSRegionCTXKey).(string)
	if !ok {
		err := errors.New("empty aws region")
		logger.Error().Err(err).Str("region", region).Msg("aws region")
		return nil, err
	}

	// qldb role arn
	qldbRoleArn, ok := ctx.Value(appctx.PaymentsQLDBRoleArnCTXKey).(string)
	if !ok {
		err := errors.New("empty qldb role arn")
		logger.Error().Err(err).Str("qldbRoleArn", qldbRoleArn).Msg("qldb role arn empty")
		return nil, err
	}

	// qldb ledger name
	qldbLedgerName, ok := ctx.Value(appctx.PaymentsQLDBLedgerNameCTXKey).(string)
	if !ok {
		err := errors.New("empty qldb ledger name")
		logger.Error().Err(err).Str("qldbLedgerName", qldbLedgerName).Msg("qldb ledger name empty")
		return nil, err
	}

	logger.Info().
		Str("egress", egressProxyAddr).
		Str("region", region).
		Str("qldbRoleArn", qldbRoleArn).
		Str("qldbLedgerName", qldbLedgerName).
		Msg("qldb details")

	cfg, err := appaws.NewAWSConfig(ctx, egressProxyAddr, region)
	if err != nil {
		logger.Error().Err(err).Str("region", region).Msg("aws config failed")
		return nil, fmt.Errorf("failed to create aws config: %w", err)
	}
	awsCfg, ok := cfg.(aws.Config)
	if !ok {
		return nil, fmt.Errorf("invalid aws configuration: %w", err)
	}

	// assume correct role for qldb access
	creds := stscreds.NewAssumeRoleProvider(sts.NewFromConfig(awsCfg), qldbRoleArn)
	awsCfg.Credentials = aws.NewCredentialsCache(creds)

	client := qldbsession.NewFromConfig(awsCfg)
	// create our qldb driver
	driver, err := qldbdriver.New(
		qldbLedgerName, // the ledger to attach to
		client,         // the qldb session
		func(options *qldbdriver.DriverOptions) {
			// debug mode?
			debug, err := appctx.GetBoolFromContext(ctx, appctx.DebugLoggingCTXKey)
			if err == nil && debug {
				options.LoggerVerbosity = qldbdriver.LogDebug
			} else {
				// default to info
				options.LoggerVerbosity = qldbdriver.LogInfo
			}
		})
	if err != nil {
		return nil, fmt.Errorf("failed to setup the qldb driver: %w", err)
	}
	// set up a retry policy
	// Configuring an exponential backoff strategy with base of 20 milliseconds
	retryPolicy2 := qldbdriver.RetryPolicy{
		MaxRetryLimit: 2,
		Backoff:       qldbdriver.ExponentialBackoffStrategy{SleepBase: 20, SleepCap: 4000}}

	// Overrides the retry policy set by the driver instance
	driver.SetRetryPolicy(retryPolicy2)

	return driver, nil
}
