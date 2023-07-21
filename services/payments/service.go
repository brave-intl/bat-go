package payments

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"text/template"

	"encoding/base64"
	"encoding/json"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/aws-sdk-go-v2/service/qldb"
	qldbTypes "github.com/aws/aws-sdk-go-v2/service/qldb/types"
	"github.com/brave-intl/bat-go/libs/custodian/provider"
	"github.com/brave-intl/bat-go/libs/nitro"
	"github.com/google/uuid"
	"github.com/hashicorp/vault/shamir"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/brave-intl/bat-go/libs/logging"
	nitroawsutils "github.com/brave-intl/bat-go/libs/nitro/aws"
	appsrv "github.com/brave-intl/bat-go/libs/service"
)

// Service struct definition of payments service.
type Service struct {
	// concurrent safe
	datastore  wrappedQldbDriverAPI
	custodians map[string]provider.Custodian
	awsCfg     aws.Config

	baseCtx          context.Context
	secretMgr        appsrv.SecretManager
	keyShares        [][]byte
	kmsDecryptKeyArn string
	kmsSigningKeyID  string
	kmsSigningClient wrappedKMSClient
	sdkClient        wrappedQldbSDKClient
	pubKey           []byte
}

func parseKeyPolicyTemplate(ctx context.Context, templateFile string) (string, error) {
	// perform enclave attestation
	nonce := make([]byte, 64)
	_, err := rand.Read(nonce)
	if err != nil {
		return "", fmt.Errorf("failed to create nonce for attestation: %w", err)
	}

	document, err := nitro.Attest(nonce, nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create attestation document: %w", err)
	}

	var logger = logging.Logger(ctx, "payments.configureKMSKey")
	logger.Debug().Msgf("document: %+v", document)

	t, err := template.ParseFiles(templateFile)
	if err != nil {
		logger.Error().Err(err).Msgf("failed to parse template file: %+v", templateFile)
		return "", err
	}

	type keyTemplateData struct {
		Pcr0 string
		Pcr1 string
		Pcr2 string
		Sha  string
	}

	buf := bytes.NewBuffer([]byte{})
	if err := t.Execute(buf, keyTemplateData{
		Pcr0: "", // TODO: get the pcr values for the condition from the document ^^
		Pcr1: "",
		Pcr2: "",
		Sha:  "",
	}); err != nil {
		logger.Error().Err(err).Msgf("failed to execute template file: %+v", templateFile)
		return "", err
	}

	return buf.String(), nil
}

type serviceNamespaceContextKey struct{}

// configureSigningKey creates the enclave kms key which is only sign capable with enclave attestation.
func (s *Service) configureSigningKey(ctx context.Context) error {

	// parse the key policy
	policy, err := parseKeyPolicyTemplate(ctx, "templates/sign-policy.tmpl")
	if err != nil {
		return fmt.Errorf("failed to parse signing policy template: %w", err)
	}

	// get the aws configuration loaded
	kmsClient := kms.NewFromConfig(s.awsCfg)

	input := &kms.CreateKeyInput{
		BypassPolicyLockoutSafetyCheck: true,
		Description:                    aws.String("Transaction signing key for settlement enclave"),
		KeySpec:                        kmstypes.KeySpecEccNistP521,
		KeyUsage:                       kmstypes.KeyUsageTypeSignVerify,
		Policy:                         aws.String(policy),
	}

	result, err := kmsClient.CreateKey(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to make key: %w", err)
	}

	s.kmsSigningKeyID = *result.KeyMetadata.KeyId
	s.kmsSigningClient = kmsClient
	return nil
}

// configureKMSKey creates the enclave kms key which is only decrypt capable with enclave attestation.
func (s *Service) configureKMSKey(ctx context.Context) error {

	// parse the key policy
	policy, err := parseKeyPolicyTemplate(ctx, "templates/decrypt-policy.tmpl")
	if err != nil {
		return fmt.Errorf("failed to parse decrypt policy template: %w", err)
	}

	// get the aws configuration loaded
	cfg := s.awsCfg
	kmsClient := kms.NewFromConfig(cfg)

	input := &kms.CreateKeyInput{
		Policy: aws.String(policy),
	}

	result, err := kmsClient.CreateKey(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to make key: %w", err)
	}

	s.kmsDecryptKeyArn = *result.KeyMetadata.KeyId
	return nil
}

// NewService creates a service using the passed datastore and clients configured from the environment.
func NewService(ctx context.Context) (context.Context, *Service, error) {
	var logger = logging.Logger(ctx, "payments.NewService")

	egressAddr, ok := ctx.Value(appctx.EgressProxyAddrCTXKey).(string)
	if !ok {
		logger.Error().Msg("no egress addr for payments service")
		return nil, nil, errors.New("no egress addr for payments service")
	}
	region, ok := ctx.Value(appctx.AWSRegionCTXKey).(string)
	if !ok {
		region = "us-west-2"
	}

	awsCfg, err := nitroawsutils.NewAWSConfig(ctx, egressAddr, region)
	if err != nil {
		logger.Error().Msg("no egress addr for payments service")
		return nil, nil, errors.New("no egress addr for payments service")
	}

	service := &Service{
		baseCtx: ctx,
		awsCfg:  awsCfg,
	}

	if err := service.configureKMSKey(ctx); err != nil {
		logger.Error().Err(err).Msg("could not create kms secret decryption key")
	}

	if err := service.configureSigningKey(ctx); err != nil {
		logger.Error().Err(err).Msg("could not create kms signing key")
		return nil, nil, errors.New("could not create kms signing key")
	}

	if err := service.configureDatastore(ctx); err != nil {
		logger.Fatal().Msg("could not configure datastore")
	}

	/*
			FIXME
		// setup our custodian integrations
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
	*/

	return ctx, service, nil
}

// DecryptBootstrap - use service keyShares to reconstruct the decryption key.
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

// ValueHolder converts a qldbPaymentTransitionHistoryEntry into a QLDB SDK ValueHolder.
func (b *qldbPaymentTransitionHistoryEntryBlockAddress) ValueHolder() *qldbTypes.ValueHolder {
	stringValue := fmt.Sprintf("{strandId:\"%s\",sequenceNo:%d}", b.StrandID, b.SequenceNo)
	return &qldbTypes.ValueHolder{
		IonText: &stringValue,
	}
}

// validateTransactionHistory returns whether a slice of entries representing the entire state history for a given id
// include exclusively valid transitions. It also verifies IdempotencyKeys among states and the Merkle tree position of each state.
func validateTransactionHistory(
	ctx context.Context,
	idempotencyKey *uuid.UUID,
	namespace uuid.UUID,
	transactionHistory []qldbPaymentTransitionHistoryEntry,
	kmsClient wrappedKMSClient,
) (bool, error) {
	var (
		reason                    error
		err                       error
		unmarshaledTransactionSet []Transaction
	)
	// Unmarshal the transactions in advance so that we don't have to do it multiple
	// times per transaction in the next loop.
	for _, marshaledTransaction := range transactionHistory {
		var transaction Transaction
		err = json.Unmarshal(marshaledTransaction.Data.Data, &transaction)
		if err != nil {
			return false, fmt.Errorf("failed to unmarshal transaction data: %w", err)
		}
		unmarshaledTransactionSet = append(unmarshaledTransactionSet, transaction)
	}
	for i, transaction := range unmarshaledTransactionSet {
		// Transitions must always start at 0
		if i == 0 {
			if transaction.State != Prepared {
				return false, &InvalidTransitionState{}
			}
			continue
		}

		// Before starting State validation, check that keys and signatures for the record are valid.
		// GenerateIdempotencyKey will verify that the ID is internally consistent within the Transaction.
		dataIdempotencyKey, err := transaction.GenerateIdempotencyKey(namespace)
		if err != nil {
			return false, fmt.Errorf("ID invalid: %w", err)
		}
		// The data object is serialized in QLDB, but when deserialized should contain an ID that matches
		// the ID on the top level of the QLDB record
		if *dataIdempotencyKey != *idempotencyKey {
			return false, fmt.Errorf("top level ID does not match Transaction ID: %s, %s", dataIdempotencyKey, idempotencyKey)
		}
		// Each transaction's signature must be verified
		txID := transaction.ID.String()
		verifyOutput, err := kmsClient.Verify(ctx, &kms.VerifyInput{
			KeyId:   &txID,
			Message: transactionHistory[i].Data.Data,
		})
		if err != nil {
			return false, fmt.Errorf("failed to verify signature: %w", err)
		}
		if !verifyOutput.SignatureValid {
			return false, fmt.Errorf("signature for record %s invalid with metadata: %v", transaction.ID.String(), verifyOutput.ResultMetadata)
		}

		// Now that the data itself is verified, proceed to check transition States.
		previousTransitionData := unmarshaledTransactionSet[i-1]
		previousIdempotencyKey, err := previousTransitionData.GenerateIdempotencyKey(namespace)
		if err != nil {
			return false, fmt.Errorf("ID invalid: %w", err)
		}
		// The IdempotencyKeys of all records in the transition history of a Transaction should match
		if *dataIdempotencyKey != *previousIdempotencyKey {
			return false, fmt.Errorf("idempotencyKeys in transition history do not match: %s, %s", idempotencyKey.String(), previousIdempotencyKey.String())
		}
		// New transaction state should be present in the list of valid next states for the "previous" (current) state.
		if !previousTransitionData.nextStateValid(transaction.State) {
			return false, &InvalidTransitionState{}
		}
	}
	return true, reason
}

// revisionValidInTree verifies a document revision in QLDB using a digest and the Merkle
// hashes to re-derive the digest.
func revisionValidInTree(
	ctx context.Context,
	client wrappedQldbSDKClient,
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
	var hashes [][32]byte

	// This Ion unmarshal gives us the hashes as bytes. The documentation implies that
	// these are base64 encoded strings, but testing indicates that is not the case.
	err = ion.UnmarshalString(*revision.Proof.IonText, &hashes)

	if err != nil {
		return false, fmt.Errorf("Failed to unmarshal revision proof: %w", err)
	}
	return verifyHashSequence(digest, transaction.Hash, hashes)
}

func verifyHashSequence(
	digest *qldb.GetDigestOutput,
	initialHash qldbPaymentTransitionHistoryEntryHash,
	hashes [][32]byte,
) (bool, error) {
	var concatenatedHash [32]byte
	for i, providedHash := range hashes {
		// During the first integration concatenatedHash hasn't been populated.
		// Populate it with the hash from the provided transaction.
		if i == 0 {
			decodedHash, err := base64.StdEncoding.DecodeString(string(initialHash))
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

	if bytes.Compare(concatenatedHash[:], decodedDigest) == 0 {
		return true, nil
	}
	return false, nil
}

func transactionHistoryIsValid(
	ctx context.Context,
	txn wrappedQldbTxnAPI,
	kmsClient wrappedKMSClient,
	id *uuid.UUID,
	namespace uuid.UUID,
) (bool, *qldbPaymentTransitionHistoryEntry, error) {
	// Fetch all historical states for this record
	result, err := getTransactionHistory(txn, id)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get transaction history: %w", err)
	}
	if len(result) < 1 {
		return false, nil, &QLDBTransitionHistoryNotFoundError{}
	}
	// Ensure that all state changes in record history were valid
	stateTransitionsAreValid, err := validateTransactionHistory(ctx, id, namespace, result, kmsClient)
	if err != nil {
		return false, nil, fmt.Errorf("failed to validate history: %w", err)
	}
	if stateTransitionsAreValid {
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

func (t *Transaction) shouldDryRun() bool {
	if t.DryRun == nil {
		return false
	}

	switch t.State {
	case Prepared:
		return *t.DryRun == "prepare"
	case Authorized:
		return *t.DryRun == "submit"
	case Pending:
		return *t.DryRun == "submit"
	case Paid:
		return *t.DryRun == "submit"
	case Failed:
		return *t.DryRun == "submit"
	default:
		return false
	}
}
