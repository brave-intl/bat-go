package payments

import (
	"bytes"
	"context"
	"crypto"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"slices"
	"strings"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	solTypes "github.com/blocto/solana-go-sdk/types"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/nitro"
	nitroawsutils "github.com/brave-intl/bat-go/libs/nitro/aws"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
	"github.com/google/uuid"
	"github.com/hashicorp/vault/shamir"
)

// ChainAddress represents an on-chain address used for payouts. It needs to be persisted
// to QLDB in this form to manage approvals and record the creator.
type ChainAddress struct {
	Chain     string   `ion:"chain"`
	PublicKey string   `ion:"publicKey"`
	Creator   string   `ion:"creator"`
	Approvals []string `ion:"approvals"`
}

// Vault represents a key which has been broken into shamir shares and is used for encrypting
// secrets.
type Vault struct {
	PublicKey      string   `ion:"publicKey"`
	Creator        string   `ion:"creator"`
	Approvals      []string `ion:"approvals"`
	Threshold      int      `ion:"threshold"`
	OperatorKeys   []string `ion:"operatorKeys"`
	IdempotencyKey string   `ion:"idempotencyKey"`
	// must be unexported. these values should never be presisted to QLDB
	shares paymentLib.CreateVaultResponse
}

// SetIdempotencyKey takes and existing vault, checks if it has valid threshold and operators, and
// generates a UUIDv5 using our Payment Service namespace, threshold, and operators.
func (v *Vault) SetIdempotencyKey() error {
	if v.Threshold < 1 {
		return errors.New("invalid threshold")
	}
	if v.OperatorKeys == nil || len(v.OperatorKeys) < 1 || len(v.OperatorKeys) < v.Threshold {
		return errors.New("invalid number of operator keys")
	}
	if len(v.PublicKey) < 1 {
		return errors.New("public key must be defined before idempotency key can be generated")
	}
	// We have to sort the opKeys to ensure that the idempotency key we generate is the same for the
	// same set of keys.
	slices.Sort(v.OperatorKeys)
	v.OperatorKeys = slices.Compact(v.OperatorKeys)
	// Generate an idempotency key using the keys and threshold. This is used to prevent us from
	// creating multiple vaults with the same configuration.
	v.IdempotencyKey = uuid.NewSHA1(
		uuid.MustParse("3c0e75eb-9150-40b4-a988-a017d115de3c"),
		[]byte(fmt.Sprintf("%s%s%s", v.Threshold, strings.Join(v.OperatorKeys, ","), v.PublicKey)),
	).String()
	return nil
}

// createAttestationDocument will create an attestation document and return the private key and
// attestation document which is attesting over the userData supplied
func createAttestationDocument(ctx context.Context, userData []byte) (crypto.PrivateKey, []byte, error) {
	// create a one time use nonce
	nonce, err := createAttestationNonce(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create attestation nonce: %w", err)
	}

	// create rsa private/public key pair for the document
	privateKey, publicKey, err := createAttestationKey(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create attestation key: %w", err)
	}

	publicKeyMarshaled, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode public key bytes for attestation: %w", err)
	}

	// attest to the document with passed in user data
	document, err := nitro.Attest(ctx, nonce, userData, publicKeyMarshaled)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create attestation document: %w", err)
	}
	return privateKey, document, nil

}

// createAttestationKey is a helper to create an RSA key for nitro attestation document
// such that kms will encrypt the results to this created key
func createAttestationKey(ctx context.Context) (crypto.PrivateKey, crypto.PublicKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate rsa private key for attestation: %w", err)
	}
	return privateKey, privateKey.Public(), nil
}

// createAttestationNonce is a helper to create a random nonce for the attestation document
func createAttestationNonce(ctx context.Context) ([]byte, error) {
	nonce := make([]byte, 64)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce for attestation: %w", err)
	}
	return nonce, nil
}

// nitroAwsCfg is a helper to get an aws config for nitro applications
func nitroAwsCfg(ctx context.Context) (aws.Config, error) {
	// get proxy address for outbound
	egressAddr, ok := ctx.Value(appctx.EgressProxyAddrCTXKey).(string)
	if !ok {
		egressAddr = ":1234"
	}

	// download the configuration from s3 bucket/object
	region, ok := ctx.Value(appctx.AWSRegionCTXKey).(string)
	if !ok {
		region = "us-west-2"
	}

	return nitroawsutils.NewAWSConfig(ctx, egressAddr, region)
}

var errSecretsNotLoaded = errors.New("secrets are not yet loaded")

func (s *Service) createVault(
	ctx context.Context,
	request paymentLib.CreateVaultRequest,
	approverKey string,
) (*paymentLib.CreateVaultResponse, error) {
	vault, err := vaultFromRequest(ctx, request.Operators, request.Threshold, approverKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate vault from request: %w", err)
	}
	err = s.datastore.InsertVault(ctx, *vault)
	if err != nil {
		return nil, fmt.Errorf("failed to insert vault into QLDB: %w", err)
	}

	return &vault.shares, nil
}

func (s *Service) approveVault(
	ctx context.Context,
	request paymentLib.ApproveVaultRequest,
	approverKey string,
) (*paymentLib.ApproveVaultResponse, error) {
	var opKeys []string
	for _, key := range request.Operators {
		opKeys = append(opKeys, key.PublicKey)
	}
	vault := Vault{
		Threshold:    request.Threshold,
		OperatorKeys: opKeys,
		PublicKey:    request.PublicKey,
	}
	vault.SetIdempotencyKey()
	updatedVault, err := s.datastore.ApproveVault(
		ctx,
		vault.IdempotencyKey,
		request.PublicKey,
		approverKey,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert vault into QLDB: %w", err)
	}

	return &paymentLib.ApproveVaultResponse{
		Approvals:     updatedVault.Approvals,
		PublicKey:     updatedVault.PublicKey,
		FullyApproved: len(updatedVault.Approvals) > 1,
	}, nil
}

func vaultFromRequest(
	ctx context.Context,
	operators []paymentLib.OperatorDataRequest,
	threshold int,
	approverKey string,
) (*Vault, error) {
	var (
		opRecpt []age.Recipient
		opNames []string
		opKeys  []string
	)
	for _, operator := range operators {
		recipient, err := agessh.ParseRecipient(operator.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("failed to parse public key", err)
		}
		// Keys and names must match indices, as these are used to associate encrypted shares
		// with names in createVault.
		opRecpt = append(opRecpt, recipient)
		opNames = append(opNames, operator.Name)
		opKeys = append(opKeys, operator.PublicKey)
	}
	// In order to map keys to names, we need an equal number of each. They should share an order.
	// The key at index 0 should be the name at index 0, etc.
	if len(opRecpt) != len(opNames) || len(opNames) != len(opKeys) {
		return nil, fmt.Errorf("have %d keys but %d names", len(opKeys), len(opNames))
	}
	// generate key
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, fmt.Errorf("failed to generate X25519 identity: %w", err)
	}

	sharesResult := paymentLib.CreateVaultResponse{
		PublicKey: identity.Recipient().String(),
		Threshold: threshold,
	}

	// Create shares
	operatorShares, err := shamir.Split([]byte(identity.String()), len(opKeys), threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to split identity into shamir shares: %w", err)
	}

	// Encrypt each share with an operator key and associate that key to the operator
	// name in a NamedOperator
	var shares []paymentLib.OperatorDataResponse
	for i, share := range operatorShares {
		buf := new(bytes.Buffer)
		// encrypt each with an operator recipient
		w, err := age.Encrypt(buf, opRecpt[i])
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt to receipient share file: %w", err)
		}

		_, err = w.Write(share)
		if err != nil {
			return nil, fmt.Errorf("failed to write cyphertext to buffer: %w", err)
		}
		w.Close()
		shares = append(shares, paymentLib.OperatorDataResponse{
			Name:     opNames[i],
			Material: buf.Bytes(),
		})
	}
	sharesResult.Shares = shares
	return &Vault{
		PublicKey:    sharesResult.PublicKey,
		Creator:      approverKey,
		Approvals:    []string{approverKey},
		Threshold:    threshold,
		OperatorKeys: opKeys,
		shares:       sharesResult,
	}, nil
}

// AreSecretsLoaded will tell you if we have successfully loaded secrets on the service
func (s *Service) AreSecretsLoaded(ctx context.Context) bool {
	if len(s.secrets) > 0 {
		return true
	}
	return false
}

func (s *Service) createSolanaAddress(ctx context.Context, bucket, creatorKey string) (*ChainAddress, error) {
	solAccount := solTypes.NewAccount()
	b58PubKey := solAccount.PublicKey.ToBase58()
	encSeed, err := s.encryptWithShares(ctx, solAccount.PrivateKey.Seed())
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt seed: %w", err)
	}

	// get the aws configuration
	awsCfg, err := nitroAwsCfg(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create aws config: %w", err)
	}
	s3Client := s3.NewFromConfig(awsCfg)

	encSeedBytes, err := io.ReadAll(encSeed)
	if err != nil {
		return nil, fmt.Errorf("failed to seed to bytes: %w", err)
	}
	h := md5.New()
	h.Write(encSeedBytes)

	input := &s3.PutObjectInput{
		Body:                      bytes.NewBuffer(encSeedBytes),
		Bucket:                    aws.String(bucket),
		Key:                       aws.String("solana-address-" + b58PubKey),
		ContentMD5:                aws.String(base64.StdEncoding.EncodeToString(h.Sum(nil))),
		ObjectLockLegalHoldStatus: s3types.ObjectLockLegalHoldStatusOn,
	}
	_, err = s3Client.PutObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to put key to s3: %w", err)
	}

	chainAdrress := ChainAddress{
		PublicKey: b58PubKey,
		Creator:   creatorKey,
		Chain:     "solana",
	}
	_, err = s.datastore.InsertChainAddress(ctx, chainAdrress)
	if err != nil {
		return nil, fmt.Errorf("failed to save address to QLDB: %w", err)
	}

	return &chainAdrress, nil
}

// NOTE: This function assumes that the http signature has been
// verified before running. This is achieved in the SubmitHandler middleware.
func (s *Service) approveSolanaAddress(ctx context.Context, address, approverKey string) (*ChainAddress, error) {
	chainAddress, err := s.datastore.GetChainAddress(ctx, address)
	if err != nil {
		return nil, fmt.Errorf("failed get address from QLDB: %w", err)
	}

	keyHasNotYetApproved := true
	for _, approval := range chainAddress.Approvals {
		if approval == approverKey {
			keyHasNotYetApproved = false
		}
	}
	if keyHasNotYetApproved {
		chainAddress.Approvals = append(chainAddress.Approvals, approverKey)
		err = s.datastore.UpdateChainAddress(ctx, *chainAddress)
		if err != nil {
			return nil, fmt.Errorf("failed to save address to QLDB: %w", err)
		}
	}

	return chainAddress, nil
}

// fetchSecrets will take an s3 bucket/object and fetch the configuration and store the
// ciphertext on the service for decryption later
func (s *Service) fetchSecrets(ctx context.Context, bucket, secretsObject string, solanaPubAddr string) error {
	awsCfg, err := nitroAwsCfg(ctx)
	if err != nil {
		return fmt.Errorf("failed to create aws config for s3 client: %w", err)
	}

	// get the secrets configurations from s3
	secretsResponse, err := s3.NewFromConfig(awsCfg).GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(secretsObject),
	})
	if err != nil {
		return fmt.Errorf("failed to get secrets from s3: %w", err)
	}

	// we are not able to decrypt secretsCiphertext until all operator shares are available
	s.secretsCiphertext, err = io.ReadAll(secretsResponse.Body)
	if err != nil {
		return fmt.Errorf("failed to read secrets bytes: %w", err)
	}

	if solanaPubAddr != "" {
		chainAddress, err := s.datastore.GetChainAddress(ctx, solanaPubAddr)
		if err != nil {
			return fmt.Errorf("failed to get solana address from QLDB: %w", err)
		}
		if len(chainAddress.Approvals) >= 2 {
			// get the solana address from s3
			solanaAddressResponse, err := s3.NewFromConfig(awsCfg).GetObject(ctx, &s3.GetObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String("solana-address-" + solanaPubAddr),
			})
			if err != nil {
				return fmt.Errorf("failed to get solana address from s3: %w", err)
			}
			s.solanaPrivCiphertext, err = io.ReadAll(solanaAddressResponse.Body)
			if err != nil {
				return fmt.Errorf("failed to read solana address bytes: %w", err)
			}
		} else {
			return fmt.Errorf("provided solana address has insufficient approvals")
		}
	}

	return nil
}

// enoughOperatorShares informs the caller if there are enough operator shares present to attempt a decrypt
func (s *Service) enoughOperatorShares(ctx context.Context, required int) bool {
	if len(s.keyShares) > required { // TODO: configurable in future, right now need two shares
		return true
	}
	return false
}

var (
	errNoSecretsCiphertext = errors.New("failed to get service configuration ciphertext")
)

// configureSecrets takes the ciphertext configuration from fetchSecrets, then decrypts it with the keyshares
// from fetchOperatorShares then stores the values in the configuration map
func (s *Service) configureSecrets(ctx context.Context) error {
	// do we have secrets downloaded?
	if len(s.secretsCiphertext) < 1 {
		return errNoSecretsCiphertext
	}

	// decrypt configuration ciphertext
	secrets, err := s.decryptSecrets(ctx)
	if err != nil {
		return fmt.Errorf("failed to decrypt secrets: %w", err)
	}

	// store conf on service
	s.secrets = secrets

	s.setEnvFromSecrets(secrets)
	return nil
}

// setEnvFromSecrets takes a secrets map and loads the secrets as environment variables
func (s *Service) setEnvFromSecrets(secrets map[string]string) {
	os.Setenv("ZEBPAY_API_KEY", secrets["zebpayApiKey"])
	os.Setenv("ZEBPAY_SIGNING_KEY", secrets["zebpayPrivateKey"])
	os.Setenv("SOLANA_RPC_ENDPOINT", secrets["solanaRpcEndpoint"])

	if solKey, ok := secrets["solanaPrivateKey"]; ok {
		os.Setenv("SOLANA_SIGNING_KEY", solKey)
	}
}

// fetchOperatorShares will take an s3 bucket and fetch all of the operator shares and store them
func (s *Service) fetchOperatorShares(ctx context.Context, bucket string) error {
	// clear out all keyshares and start over, we will be downloading ALL shares from the s3 bucket
	s.keyShares = [][]byte{}

	// get the aws configuration
	awsCfg, err := nitroAwsCfg(ctx)
	if err != nil {
		return fmt.Errorf("failed to create aws config: %w", err)
	}

	s3Client := s3.NewFromConfig(awsCfg)

	pcrs, err := nitro.GetPCRs()
	if err != nil {
		return fmt.Errorf("failed to get PCR values: %w", err)
	}

	// list all objects in the bucket prefixed with operator-share
	shareObjects, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Prefix: aws.String(hex.EncodeToString(pcrs[2]) + "/operator-share"),
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return fmt.Errorf("failed to list s3 objects: %w", err)
	}

	// for each share object, get it, attempt to decrypt and append to keyShares
	for _, shareObject := range shareObjects.Contents {
		// use kms encrypt key arn on service to decrypt each file
		shareResponse, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    shareObject.Key, // the share object key for this iteration
		})
		if err != nil {
			return fmt.Errorf("failed to get operator share from s3: %w", err)
		}

		data, err := ioutil.ReadAll(shareResponse.Body)
		if err != nil {
			return fmt.Errorf("failed to read operator share from s3 response: %w", err)
		}

		privateKey, document, err := createAttestationDocument(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to create attestation document: %w", err)
		}

		// decrypt with kms key that only enclave can decrypt with
		decryptOutput, err := kms.NewFromConfig(awsCfg).Decrypt(ctx, &kms.DecryptInput{
			CiphertextBlob:      data,
			EncryptionAlgorithm: kmsTypes.EncryptionAlgorithmSpecSymmetricDefault,
			KeyId:               aws.String(s.kmsDecryptKeyArn),
			Recipient: &kmsTypes.RecipientInfo{
				AttestationDocument:    document,                                       // attestation document
				KeyEncryptionAlgorithm: kmsTypes.KeyEncryptionMechanismRsaesOaepSha256, // how to decrypt
			},
		})
		if err != nil {
			return fmt.Errorf("failed to decrypt object with kms: %w", err)
		}

		plaintext, err := nitro.Decrypt(privateKey.(*rsa.PrivateKey), decryptOutput.CiphertextForRecipient)
		if err != nil {
			return fmt.Errorf("failed to decrypt the ciphertext for recipient from kms: %w", err)
		}

		// store the decrypted keyShares on the service as [][]byte for later
		share, err := base64.StdEncoding.DecodeString(string(plaintext))
		if err != nil {
			return fmt.Errorf("failed to base64 decode operator key share: %w", err)
		}

		s.keyShares = append(s.keyShares, share)
	}

	return nil
}

// decryptSecrets combines the shamir shares stored on the service instance and decrypts the ciphertext
// returning a map of secret values from the configuration
func (s *Service) decryptSecrets(ctx context.Context) (map[string]string, error) {
	var output = map[string]string{}

	secBuf := bytes.NewBuffer(s.secretsCiphertext)

	sec, err := s.decryptWithShares(ctx, *secBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secrets with shares: %w", err)
	}
	if err := json.NewDecoder(sec).Decode(&output); err != nil {
		return nil, fmt.Errorf("failed to json decode the secrets: %w", err)
	}

	if len(s.solanaPrivCiphertext) > 0 {
		solBuf := bytes.NewBuffer(s.solanaPrivCiphertext)
		solReader, err := s.decryptWithShares(ctx, *solBuf)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt solana address with shares: %w", err)
		}
		buf := new(bytes.Buffer)
		buf.ReadFrom(solReader)
		output["solanaPrivateKey"] = buf.String()
	}

	return output, nil
}

func (s *Service) decryptWithShares(ctx context.Context, buf bytes.Buffer) (io.Reader, error) {
	// combine the service configured key shares
	privateKey, err := shamir.Combine(s.keyShares)
	if err != nil {
		return nil, fmt.Errorf("failed to combine keyShares: %w", err)
	}

	identity, err := age.ParseX25519Identity(string(privateKey))
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key bytes for secret decryption: %w", err)
	}

	// Before we decrypt with this key we must confirm that the key is known to the system and has
	// approved by a sufficient number of operators.
	ok, err := s.keyIsApproved(ctx, identity.Recipient().String())
	if err != nil {
		return nil, fmt.Errorf("failed to check if key is approved: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("provided shares represent an unapproved key")
	}

	return age.Decrypt(bytes.NewReader(buf.Bytes()), identity)
}

func (s *Service) encryptWithShares(ctx context.Context, data []byte) (io.Reader, error) {
	// combine the service configured key shares
	privateKey, err := shamir.Combine(s.keyShares)
	if err != nil {
		return nil, fmt.Errorf("failed to combine keyShares: %w", err)
	}

	identity, err := age.ParseX25519Identity(string(privateKey))
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key bytes for secret decryption: %w", err)
	}

	// Before we decrypt with this key we must confirm that the key is known to the system and has
	// approved by a sufficient number of operators.
	ok, err := s.keyIsApproved(ctx, identity.Recipient().String())
	if err != nil {
		return nil, fmt.Errorf("failed to check if key is approved: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("provided shares represent an unapproved key")
	}

	out := &bytes.Buffer{}

	w, err := age.Encrypt(out, identity.Recipient())
	if err != nil {
		return nil, fmt.Errorf("Failed to create encrypted file: %v", err)
	}
	if _, err := io.WriteString(w, string(data)); err != nil {
		return nil, fmt.Errorf("Failed to write to encrypted file: %v", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("Failed to close encrypted file: %v", err)
	}
	return out, nil
}

func (s *Service) keyIsApproved(ctx context.Context, pubKey string) (bool, error) {
	vault, err := s.datastore.GetVaultWithPublicKey(ctx, pubKey)
	if err != nil {
		return false, fmt.Errorf("failed to fetch vault from QLDB with key: %s", pubKey)
	}
	return len(vault.Approvals) > 1, nil
}
