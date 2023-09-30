package payments

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"
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

type compatDatastore interface {
	Datastore
	wrappedQldbDriverAPI
}

// Service struct definition of payments service.
type Service struct {
	// concurrent safe
	datastore  compatDatastore
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

	verifier Verifier
}

func parseKeyPolicyTemplate(ctx context.Context, templateFile string) (string, string, error) {
	// perform enclave attestation
	nonce := make([]byte, 64)
	_, err := rand.Read(nonce)
	if err != nil {
		return "", "", fmt.Errorf("failed to create nonce for attestation: %w", err)
	}

	document, err := nitro.Attest(ctx, nonce, nil, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create attestation document: %w", err)
	}

	var logger = logging.Logger(ctx, "payments.configureKMSKey")
	logger.Info().Msgf("document: %+v", document)

	// parse the root certificate
	block, _ := pem.Decode([]byte(nitro.RootAWSNitroCert))

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse certificate: %w", err)
	}

	// parse document
	ad, err := nitro_eclave_attestation_document.AuthenticateDocument(document, *cert, true)
	if err != nil {
		logger.Error().Err(err).Msgf("failed to parse template attestation document: %+v", ad)
		return "", "", err
	}
	logger.Info().Msgf("digest: %+v", ad.Digest)
	logger.Info().Msgf("pcrs: %+v", ad.PCRs)

	t, err := template.ParseFiles(templateFile)
	if err != nil {
		logger.Error().Err(err).Msgf("failed to parse template file: %+v", templateFile)
		return "", "", err
	}

	type keyTemplateData struct {
		PCR0       string
		PCR1       string
		PCR2       string
		AWSAccount string
	}

	buf := bytes.NewBuffer([]byte{})
	if err := t.Execute(buf, keyTemplateData{
		PCR0:       hex.EncodeToString(ad.PCRs[0]),
		PCR1:       hex.EncodeToString(ad.PCRs[1]),
		PCR2:       hex.EncodeToString(ad.PCRs[2]),
		AWSAccount: os.Getenv("AWS_ACCOUNT"),
	}); err != nil {
		logger.Error().Err(err).Msgf("failed to execute template file: %+v", templateFile)
		return "", "", err
	}

	policy := buf.String()

	logger.Info().Msgf("key policy: %+v", policy)

	return policy, hex.EncodeToString(ad.PCRs[2]), nil
}

type serviceNamespaceContextKey struct{}

// configureSigningKey creates the enclave kms key which is only sign capable with enclave
// attestation.
func (s *Service) configureSigningKey(ctx context.Context) error {
	// get the aws configuration loaded
	kmsClient := kms.NewFromConfig(s.awsCfg)

	// parse the key policy
	policy, imageSHA, err := parseKeyPolicyTemplate(ctx, "/sign-policy.tmpl")
	if err != nil {
		return fmt.Errorf("failed to parse signing policy template: %w", err)
	}

	// if the key alias already exists, pull down that particular key
	getKeyResult, err := kmsClient.DescribeKey(ctx, &kms.DescribeKeyInput{
		KeyId: aws.String("alias/signing-" + imageSHA),
	})
	// If the error is that the key wasn't found, proceed. Otherwise, fail with error.
	if err != nil {
		if !strings.Contains(err.Error(), "NotFoundException") {
			return fmt.Errorf("failed to get key by alias: %w", err)
		}
	}

	if getKeyResult != nil {
		// key exists, check the key policy matches
		// if the key alias already exists, pull down that particular key
		getKeyPolicyResult, err := kmsClient.GetKeyPolicy(ctx, &kms.GetKeyPolicyInput{
			KeyId: getKeyResult.KeyMetadata.KeyId,
		})
		if err != nil {
			return fmt.Errorf("failed to get key by alias: %w", err)
		}

		if *getKeyPolicyResult.Policy == policy {
			// if the policy matches, we should use this key
			s.kmsSigningKeyID = *getKeyResult.KeyMetadata.KeyId
			s.kmsSigningClient = kmsClient
			return nil
		}
	}

	// otherwise create and alias

	input := &kms.CreateKeyInput{
		KeySpec:  kmsTypes.KeySpecEccNistP256,
		KeyUsage: kmsTypes.KeyUsageTypeSignVerify,
		Policy:   aws.String(policy),
		BypassPolicyLockoutSafetyCheck: true,
		Tags: []kmsTypes.Tag{
			{TagKey: aws.String("Purpose"), TagValue: aws.String("settlements")},
		},
	}

	result, err := kmsClient.CreateKey(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to make key: %w", err)
	}

	// create a key alias for this key
	aliasInput := &kms.CreateAliasInput{
		AliasName:   aws.String("alias/signing-" + imageSHA),
		TargetKeyId: result.KeyMetadata.KeyId,
	}

	_, err = kmsClient.CreateAlias(ctx, aliasInput)
	if err != nil {
		return fmt.Errorf("failed to make key alias: %w", err)
	}

	s.kmsSigningKeyID = *result.KeyMetadata.KeyId
	s.kmsSigningClient = kmsClient
	return nil
}

// configureKMSKey creates the enclave kms key which is only decrypt capable with enclave
// attestation.
func (s *Service) configureKMSKey(ctx context.Context) error {
	// get the aws configuration loaded
	cfg := s.awsCfg
	kmsClient := kms.NewFromConfig(cfg)

	// parse the key policy
	policy, imageSHA, err := parseKeyPolicyTemplate(ctx, "/decrypt-policy.tmpl")
	if err != nil {
		return fmt.Errorf("failed to parse decrypt policy template: %w", err)
	}

	// if the key alias already exists, pull down that particular key
	getKeyResult, err := kmsClient.DescribeKey(ctx, &kms.DescribeKeyInput{
		KeyId: aws.String("alias/decryption-" + imageSHA),
	})
	// If the error is that the key wasn't found, proceed. Otherwise, fail with error.
	if err != nil {
		if !strings.Contains(err.Error(), "NotFoundException") {
			return fmt.Errorf("failed to get key by alias: %w", err)
		}
	}

	if getKeyResult != nil {
		// key exists, check the key policy matches
		// if the key alias already exists, pull down that particular key
		getKeyPolicyResult, err := kmsClient.GetKeyPolicy(ctx, &kms.GetKeyPolicyInput{
			KeyId: getKeyResult.KeyMetadata.KeyId,
		})
		if err != nil {
			return fmt.Errorf("failed to get key by alias: %w", err)
		}

		if *getKeyPolicyResult.Policy == policy {
			// if the policy matches, we should use this key
			s.kmsDecryptKeyArn = *getKeyResult.KeyMetadata.KeyId
			return nil
		}
	}

	input := &kms.CreateKeyInput{
		Policy: aws.String(policy),
		BypassPolicyLockoutSafetyCheck: true,
		Tags: []kmsTypes.Tag{
			{TagKey: aws.String("Purpose"), TagValue: aws.String("settlements")},
		},
	}

	result, err := kmsClient.CreateKey(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to make key: %w", err)
	}

	aliasInput := &kms.CreateAliasInput{
		AliasName:   aws.String("alias/decryption-" + imageSHA),
		TargetKeyId: result.KeyMetadata.KeyId,
	}

	_, err = kmsClient.CreateAlias(ctx, aliasInput)
	if err != nil {
		return fmt.Errorf("failed to make key alias: %w", err)
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
	service.verifier = KMSVerifier{
		kmsSigningKeyID: service.kmsSigningKeyID,
		kmsClient:       service.kmsSigningClient,
	}

	if err := service.configureDatastore(ctx); err != nil {
		logger.Fatal().Err(err).Msg("could not configure datastore")
	}

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

// revisionValidInTree verifies a document revision in QLDB using a digest and the Merkle
// hashes to re-derive the digest.
func revisionValidInTree(
	ctx context.Context,
	client wrappedQldbSDKClient,
	transaction *QLDBPaymentTransitionHistoryEntry,
) (bool, error) {
	qldbLedgerName, ok := ctx.Value(appctx.PaymentsQLDBLedgerNameCTXKey).(string)
	if !ok {
		return false, fmt.Errorf("empty qldb ledger name. revision not verified for state: %v", transaction)
	}
	digest, err := client.GetDigest(ctx, &qldb.GetDigestInput{Name: &qldbLedgerName})

	if err != nil {
		return false, fmt.Errorf("Failed to get digest: %w", err)
	}

	revision, err := client.GetRevision(ctx, &qldb.GetRevisionInput{
		BlockAddress:     transaction.BlockAddress.ValueHolder(),
		DocumentId:       &transaction.Metadata.ID,
		Name:             &qldbLedgerName,
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

// signPaymentState - perform KMS signing of the transaction, return publicKey and
// signature in hex string.
func signPaymentState(
	ctx context.Context,
	kmsClient wrappedKMSClient,
	keyID string,
	state PaymentState,
) ([]byte, []byte, error) {
	pubkeyOutput, err := kmsClient.GetPublicKey(ctx, &kms.GetPublicKeyInput{
		KeyId: &keyID,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get public key: %w", err)
	}

	signingOutput, err := kmsClient.Sign(ctx, &kms.SignInput{
		KeyId:            &keyID,
		Message:          state.UnsafePaymentState,
		MessageType:      kmsTypes.MessageTypeRaw,
		SigningAlgorithm: kmsTypes.SigningAlgorithmSpecEcdsaSha256,
	})

	if err != nil {
		return nil, nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return pubkeyOutput.PublicKey, signingOutput.Signature, nil
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
