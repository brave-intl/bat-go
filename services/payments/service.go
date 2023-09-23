package payments

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"os"
	"text/template"

	nitro_eclave_attestation_document "github.com/veracruz-project/go-nitro-enclave-attestation-document"

	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/aws-sdk-go-v2/service/qldb"
	"github.com/brave-intl/bat-go/libs/custodian/provider"
	"github.com/brave-intl/bat-go/libs/nitro"
	"github.com/hashicorp/vault/shamir"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/cryptography"
	"github.com/brave-intl/bat-go/libs/logging"
	nitroawsutils "github.com/brave-intl/bat-go/libs/nitro/aws"
	. "github.com/brave-intl/bat-go/libs/payments"
	appsrv "github.com/brave-intl/bat-go/libs/service"
)

// Service struct definition of payments service.
type Service struct {
	// concurrent safe
	datastore  wrappedQldbDriverAPI
	custodians map[string]provider.Custodian
	awsCfg     aws.Config

	baseCtx              context.Context
	secretMgr            appsrv.SecretManager
	keyShares            [][]byte
	kmsDecryptKeyArn     string
	kmsSigningKeyID      string
	kmsSigningClient     wrappedKMSClient
	sdkClient            wrappedQldbSDKClient
	pubKey               []byte
	idempotencyNamespace uuid.UUID
}

// prepareTransaction - perform a qldb insertion on the transaction.
func (s *Service) prepareTransaction(
	ctx context.Context,
	prepareRequest *PrepareRequest,
) (*PrepareResponse, error) {
	documentID, err := s.insertPayment(ctx, prepareRequest.PaymentDetails)
	if err != nil {
		return nil, fmt.Errorf("failed to insert payment: %w", err)
	}
	return &PrepareResponse{DocumentID: documentID}, nil
}

func parseKeyPolicyTemplate(ctx context.Context, templateFile string) (string, error) {
	// perform enclave attestation
	nonce := make([]byte, 64)
	_, err := rand.Read(nonce)
	if err != nil {
		return "", fmt.Errorf("failed to create nonce for attestation: %w", err)
	}

	document, err := nitro.Attest(ctx, nonce, nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create attestation document: %w", err)
	}

	var logger = logging.Logger(ctx, "payments.configureKMSKey")
	logger.Info().Msgf("document: %+v", document)

	// parse the root certificate
	block, _ := pem.Decode([]byte(nitro.RootAWSNitroCert))

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse certificate: %w", err)
	}

	// parse document
	ad, err := nitro_eclave_attestation_document.AuthenticateDocument(document, *cert, true)
	if err != nil {
		logger.Error().Err(err).Msgf("failed to parse template attestation document: %+v", ad)
		return "", err
	}
	logger.Info().Msgf("digest: %+v", ad.Digest)
	logger.Info().Msgf("pcrs: %+v", ad.PCRs)

	t, err := template.ParseFiles(templateFile)
	if err != nil {
		logger.Error().Err(err).Msgf("failed to parse template file: %+v", templateFile)
		return "", err
	}

	type keyTemplateData struct {
		PCR0       string
		PCR1       string
		PCR2       string
		ImageSHA   string
		AWSAccount string
	}

	buf := bytes.NewBuffer([]byte{})
	if err := t.Execute(buf, keyTemplateData{
		PCR0:       hex.EncodeToString(ad.PCRs[0]),
		PCR1:       hex.EncodeToString(ad.PCRs[1]),
		PCR2:       hex.EncodeToString(ad.PCRs[2]),
		ImageSHA:   ad.Digest,
		AWSAccount: os.Getenv("AWS_ACCOUNT"),
	}); err != nil {
		logger.Error().Err(err).Msgf("failed to execute template file: %+v", templateFile)
		return "", err
	}

	policy := buf.String()

	logger.Info().Msgf("key policy: %+v", policy)

	return policy, nil
}

type serviceNamespaceContextKey struct{}

// configureSigningKey creates the enclave kms key which is only sign capable with enclave
// attestation.
func (s *Service) configureSigningKey(ctx context.Context) error {
	// parse the key policy
	policy, err := parseKeyPolicyTemplate(ctx, "/sign-policy.tmpl")
	if err != nil {
		return fmt.Errorf("failed to parse signing policy template: %w", err)
	}

	// get the aws configuration loaded
	kmsClient := kms.NewFromConfig(s.awsCfg)

	input := &kms.CreateKeyInput{
		Policy:                         aws.String(policy),
		BypassPolicyLockoutSafetyCheck: true,
		Tags: []kmsTypes.Tag{
			{TagKey: aws.String("Purpose"), TagValue: aws.String("settlements")},
		},
	}

	result, err := kmsClient.CreateKey(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to make key: %w", err)
	}

	s.kmsSigningKeyID = *result.KeyMetadata.KeyId
	s.kmsSigningClient = kmsClient
	return nil
}

// configureKMSKey creates the enclave kms key which is only decrypt capable with enclave
// attestation.
func (s *Service) configureKMSKey(ctx context.Context) error {

	// parse the key policy
	policy, err := parseKeyPolicyTemplate(ctx, "/decrypt-policy.tmpl")
	if err != nil {
		return fmt.Errorf("failed to parse decrypt policy template: %w", err)
	}

	// get the aws configuration loaded
	cfg := s.awsCfg
	kmsClient := kms.NewFromConfig(cfg)

	input := &kms.CreateKeyInput{
		Policy:                         aws.String(policy),
		BypassPolicyLockoutSafetyCheck: true,
		Tags: []kmsTypes.Tag{
			{TagKey: aws.String("Purpose"), TagValue: aws.String("settlements")},
		},
	}

	result, err := kmsClient.CreateKey(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to make key: %w", err)
	}

	s.kmsDecryptKeyArn = *result.KeyMetadata.KeyId
	return nil
}

// NewService creates a service using the passed datastore and clients configured from the
// environment.
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

	namespaceUUID, err := uuid.Parse(os.Getenv("namespaceUUID"))
	if err != nil {
		logger.Error().Msg("namespaceUUID not properly formatted")
		return nil, nil, errors.New("namespaceUUID not properly formatted")
	}

	service := &Service{
		baseCtx:              ctx,
		awsCfg:               awsCfg,
		idempotencyNamespace: namespaceUUID,
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
func (s *Service) DecryptBootstrap(
	ctx context.Context,
	ciphertext []byte,
) (map[appctx.CTXKey]interface{}, error) {
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

// validateTransactionHistory returns whether a slice of entries representing the entire state
// history for a given id include exclusively valid transitions. It also verifies IdempotencyKeys
// among states and the Merkle tree position of each state.
func validateTransactionHistory(
	ctx context.Context,
	transactionHistory []QLDBPaymentTransitionHistoryEntry,
) (bool, error) {
	var (
		reason                    error
		err                       error
		unmarshaledTransactionSet []AuthenticatedPaymentState
	)
	// Unmarshal the transactions in advance so that we don't have to do it multiple
	// times per transaction in the next loop.
	for _, marshaledTransaction := range transactionHistory {
		var transaction AuthenticatedPaymentState
		err = json.Unmarshal(marshaledTransaction.Data.UnsafePaymentState, &transaction)
		if err != nil {
			return false, fmt.Errorf("failed to unmarshal transaction data: %w", err)
		}
		unmarshaledTransactionSet = append(unmarshaledTransactionSet, transaction)
	}
	for i, transaction := range unmarshaledTransactionSet {
		// Transitions must always start at 0
		if i == 0 {
			if transaction.Status != Prepared {
				return false, &InvalidTransitionState{}
			}
			continue
		}

		// Now that the data itself is verified, proceed to check transition States.
		previousTransitionData := unmarshaledTransactionSet[i-1]
		// New transaction state should be present in the list of valid next states for the
		// "previous" (current) state.
		if !previousTransitionData.NextStateValid(transaction.Status) {
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
	initialHash QLDBPaymentTransitionHistoryEntryHash,
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
	documentID string,
) (bool, *QLDBPaymentTransitionHistoryEntry, error) {
	// Fetch all historical states for this record
	result, err := getTransactionHistory(txn, documentID)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get transaction history: %w", err)
	}
	if len(result) < 1 {
		return false, nil, &QLDBTransitionHistoryNotFoundError{}
	}
	// Ensure that all state changes in record history were valid
	stateTransitionsAreValid, err := validateTransactionHistory(
		ctx,
		result,
	)
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
