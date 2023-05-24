package payments

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/qldb"
	qldbTypes "github.com/aws/aws-sdk-go-v2/service/qldb/types"
	"github.com/aws/aws-sdk-go-v2/service/qldbsession"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/awslabs/amazon-qldb-driver-go/v3/qldbdriver"
	"github.com/brave-intl/bat-go/libs/custodian/provider"
	"github.com/brave-intl/bat-go/libs/nitro"
	appaws "github.com/brave-intl/bat-go/libs/nitro/aws"
	"github.com/google/uuid"
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
	kmsSigningKeyID  string
	kmsSigningClient wrappedKMSClient
	sdkClient        wrappedQldbSdkClient
	pubKey           []byte
}

type serviceNamespaceContextKey struct{}

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

	kmsClient := kms.NewFromConfig(cfg)

	input := &kms.CreateKeyInput{
		Policy: aws.String(keyPolicy),
	}

	result, err := kmsClient.CreateKey(ctx, input)
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
		baseCtx: ctx,
		//secretMgr: &awsClient{},
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
// for a qldbPaymentTransitionHistoryEntry.
func (e *qldbPaymentTransitionHistoryEntry) BuildSigningBytes() ([]byte, error) {
	marshaled, err := ion.MarshalBinary(e.Data.Data)
	if err != nil {
		return nil, fmt.Errorf("Ion marshal failed: %w", err)
	}

	return marshaled, nil
}

// ValueHolder converts a qldbPaymentTransitionHistoryEntry into a QLDB SDK ValueHolder
func (b *qldbPaymentTransitionHistoryEntryBlockAddress) ValueHolder() *qldbTypes.ValueHolder {
	stringValue := fmt.Sprintf("{strandId:\"%s\",sequenceNo:%d}", b.StrandID, b.SequenceNo)
	return &qldbTypes.ValueHolder{
		IonText: &stringValue,
	}
}

// validateTransactionHistory returns whether a slice of entries representing the entire state history for a given id
// include exclusively valid transitions. It also verifies IdempotencyKeys among states and the Merkle tree position of each state
func validateTransactionHistory(
	ctx context.Context,
	idempotencyKey *uuid.UUID,
	transactionHistory []qldbPaymentTransitionHistoryEntry,
	kmsClient wrappedKMSClient,
) (bool, error) {
	var (
		reason error
		err    error
	)
	for i, transaction := range transactionHistory {
		var transactionData Transaction
		namespace := ctx.Value(serviceNamespaceContextKey{}).(uuid.UUID)
		err = ion.Unmarshal(transaction.Data.Data, &transactionData)
		if err != nil {
			return false, fmt.Errorf("failed to unmarshal transaction data: %w", err)
		}

		// Before starting State validation, check that keys and signatures for the record are valid.

		// GenerateIdempotencyKey will verify that the ID is internally consistent within the Transaction.
		dataIdempotencyKey, err := transactionData.GenerateIdempotencyKey(namespace)
		if err != nil {
			return false, fmt.Errorf("ID invalid: %w", err)
		}
		// The data object is serialized in QLDB, but when deserialized should contain an ID that matches
		// the ID on the top level of the QLDB record
		if *dataIdempotencyKey != *idempotencyKey {
			return false, fmt.Errorf("top level ID does not match Transaction ID: %s, %s", dataIdempotencyKey, idempotencyKey)
		}
		// Each transaction's signature must be verified
		txID := transaction.Data.IdempotencyKey.String()
		verifyOutput, err := kmsClient.Verify(ctx, &kms.VerifyInput{
			KeyId:   &txID,
			Message: transaction.Data.Data,
		})
		if err != nil {
			return false, fmt.Errorf("failed to verify signature: %w", err)
		}
		if !verifyOutput.SignatureValid {
			return false, fmt.Errorf("signature for record %s invalid with metadata: %v", transaction.Data.IdempotencyKey.String(), verifyOutput.ResultMetadata)
		}

		// Now that the data itself is verified, proceed to check transition States.
		transactionState := transactionData.State
		// Transitions must always start at 0
		if i == 0 {
			if transactionState != 0 {
				return false, errors.New("initial state is not valid")
			}
			continue
		}
		var previousTransitionData Transaction
		err = ion.Unmarshal(transactionHistory[i-1].Data.Data, &previousTransitionData)
		if err != nil {
			return false, fmt.Errorf("failed to unmarshal previous transition history record: %w", err)
		}
		previousIdempotencyKey, err := previousTransitionData.GenerateIdempotencyKey(namespace)
		if err != nil {
			return false, fmt.Errorf("ID invalid: %w", err)
		}
		// The IdempotencyKeys of all records in the transition history of a Transaction should match
		if *dataIdempotencyKey != *previousIdempotencyKey {
			return false, fmt.Errorf("idempotencyKeys in transition history do not match: %s, %s", idempotencyKey.String(), previousIdempotencyKey.String())
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
	transaction *qldbPaymentTransitionHistoryEntry,
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

func transactionHistoryIsValid(
	ctx context.Context,
	txn wrappedQldbTxnAPI,
	kmsClient wrappedKMSClient,
	id *uuid.UUID,
) (bool, *qldbPaymentTransitionHistoryEntry, error) {
	// Fetch all historical states for this record
	result, err := getTransactionHistory(txn, id)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get transaction history: %w", err)
	}
	if len(result) < 1 {
		return false, nil, errors.New("record not found")
	}
	// Ensure that all state changes in record history were valid
	validTransitions, err := validateTransactionHistory(ctx, id, result, kmsClient)
	if err != nil {
		return false, nil, fmt.Errorf("failed to validate history: %w", err)
	}
	if validTransitions {
		// We only want the latest state of this record once its
		// history is verified and we have confirmed that the new state is valid
		return true, &result[0], nil
	}
	return false, &result[0], fmt.Errorf("invalid transaction history: %w", err)
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

func shouldDryRun(txn *Transaction) bool {
	if txn.DryRun != nil {
		switch txn.State {
		case Prepared:
			return *txn.DryRun == "prepare"
		case Authorized:
			return *txn.DryRun == "submit"
		case Pending:
			return *txn.DryRun == "submit"
		case Paid:
			return *txn.DryRun == "submit"
		case Failed:
			return *txn.DryRun == "submit"
		}
	}
	return false
}
