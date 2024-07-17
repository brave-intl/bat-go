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
	"os"
	"strings"
	"time"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	solTypes "github.com/blocto/solana-go-sdk/types"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/nitro"
	nitroawsutils "github.com/brave-intl/bat-go/libs/nitro/aws"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
	"github.com/hashicorp/vault/shamir"
	"github.com/rs/zerolog"
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
	Threshold      int      `ion:"threshold"`
	OperatorKeys   []string `ion:"operatorKeys"`
	IdempotencyKey string   `ion:"idempotencyKey"`
	// must be unexported. these values should never be presisted to QLDB
	shares paymentLib.CreateVaultResponse
}

type OperatorKey = *age.X25519Identity

// State of vault unsealing
type Unsealing struct {
	// id for AWS KMS key to encrypt/decrypt operator shares
	kmsDecryptKeyArn string
	getChainAddress  func(ctx context.Context, address string) (*ChainAddress, error)

	// private key reconstructed from the operator shares
	operatorKey OperatorKey

	keyShares            [][]byte
	secretsCiphertext    []byte
	solanaPrivCiphertext []byte
	secrets              map[string]string
}

// Number of operator Shamir shares sufficient to decrypt the config.
const requiredOperatorShares = 2

// createAttestationDocument will create an attestation document and return the private key and
// attestation document which is attesting the private key
func createAttestationDocument(ctx context.Context) (crypto.PrivateKey, []byte, error) {
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

	// attest to the document
	document, err := nitro.Attest(ctx, nonce, nil, publicKeyMarshaled)
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

	region, ok := ctx.Value(appctx.AWSRegionCTXKey).(string)
	if !ok {
		region = "us-west-2"
	}

	return nitroawsutils.NewAWSConfig(ctx, egressAddr, region)
}

var errSecretsNotLoaded = errors.New("secrets are not yet loaded")

func (s *Service) createVault(
	ctx context.Context,
	threshold int,
) (*paymentLib.CreateVaultResponse, error) {
	managerKeys := vaultManagerKeys()
	shares, vaultPubkey, err := generateShares(len(managerKeys), threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to generate new key with shares: %w", err)
	}
	operatorShareData, err := encryptShares(shares, managerKeys)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt shares to operator keys: %w", err)
	}

	vault := Vault{
		PublicKey:    vaultPubkey,
		Threshold:    threshold,
		OperatorKeys: managerKeys,
		shares: paymentLib.CreateVaultResponse{
			PublicKey: vaultPubkey,
			Threshold: threshold,
			Shares:    operatorShareData,
		},
	}

	err = s.datastore.InsertVault(ctx, vault)
	if err != nil {
		return nil, fmt.Errorf("failed to insert vault into QLDB: %w", err)
	}

	return &vault.shares, nil
}

func (s *Service) verifyVault(
	ctx context.Context,
	request paymentLib.VerifyVaultRequest,
) (*paymentLib.VerifyVaultResponse, error) {
	fetchedVault, err := s.datastore.GetVault(ctx, request.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get vault from QLDB: %w", err)
	}

	return &paymentLib.VerifyVaultResponse{
		Operators: fetchedVault.OperatorKeys,
		Threshold: fetchedVault.Threshold,
		PublicKey: fetchedVault.PublicKey,
	}, nil
}

// generateShares generates a new ed25519 key and splits it into shares, returning the shares and
// the public key for the newly generated private key.
func generateShares(numberOfOperators, unlockThreshold int) ([][]byte, string, error) {
	vaultIdentity, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate X25519 identity: %w", err)
	}
	shares, err := shamir.Split([]byte(vaultIdentity.String()), numberOfOperators, unlockThreshold)
	if err != nil {
		return nil, "", fmt.Errorf("failed to split key into shares: %w", err)
	}
	return shares, vaultIdentity.Recipient().String(), nil
}

func encryptShares(shares [][]byte, operatorKeys []string) ([]paymentLib.OperatorShareData, error) {
	// Encrypt each share with an operator key and associate that key to the operator
	// name in a NamedOperator
	var shareResult []paymentLib.OperatorShareData
	for i, share := range shares {
		recipient, err := agessh.ParseRecipient(operatorKeys[i])
		if err != nil {
			return nil, fmt.Errorf("failed to parse public key: %w", err)
		}
		buf := new(bytes.Buffer)
		// encrypt each with an operator recipient
		w, err := age.Encrypt(buf, recipient)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt to receipient share file: %w", err)
		}

		_, err = io.WriteString(w, base64.StdEncoding.EncodeToString(share))
		// Cannot defer this close because we are writing and using this writer in a loop. If this
		// close is deferred, the shares will be corrupted.
		err2 := w.Close()
		if err != nil || err2 != nil {
			return nil, fmt.Errorf("failed to write encoded ciphertext to encrypted buffer: %w", errors.Join(err, err2))
		}

		keyEmail := strings.Split(string(operatorKeys[i]), " ")
		shareResult = append(shareResult, paymentLib.OperatorShareData{
			Name:     strings.TrimSpace(keyEmail[len(keyEmail)-1]),
			Material: buf.Bytes(),
		})
	}
	return shareResult, nil
}

func (s *Service) createSolanaAddress(ctx context.Context, bucket, creatorKey string) (*ChainAddress, error) {
	solAccount := solTypes.NewAccount()
	b58PubKey := solAccount.PublicKey.ToBase58()
	encBuf := &bytes.Buffer{}
	err := encryptToWriter(ctx, s.operatorKey, solAccount.PrivateKey.Seed(), encBuf)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt seed: %w", err)
	}

	// get the aws configuration
	awsCfg, err := nitroAwsCfg(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create aws config: %w", err)
	}

	var s3OptFns []func(*s3.Options)
	if nitro.EnclaveMocking() {
		// Force connection to the local stack
		s3OptFns = append(s3OptFns, func(o *s3.Options) {
			o.BaseEndpoint = aws.String("http://localstack:4566")
		})
	}
	s3Client := s3.NewFromConfig(awsCfg, s3OptFns...)

	h := md5.New()
	h.Write(encBuf.Bytes())

	input := &s3.PutObjectInput{
		Body:                      encBuf,
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
func (u *Unsealing) tryFetchSecrets(ctx context.Context, bucket, secretsObject string, solanaPubAddr string) error {
	logger := logging.Logger(ctx, "payments.secrets")
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
	u.secretsCiphertext, err = io.ReadAll(secretsResponse.Body)
	if err != nil {
		return fmt.Errorf("failed to read secrets bytes: %w", err)
	}

	if solanaPubAddr != "" {
		logger.Debug().Str("solana public key", string(solanaPubAddr)).Msg("fetching solana key from s3")
		chainAddress, err := u.getChainAddress(ctx, solanaPubAddr)
		if err != nil {
			return fmt.Errorf("failed to get solana address from QLDB: %w", err)
		}
		if len(chainAddress.Approvals) >= 2 {
			logger.Debug().Str("solana approvers", strings.Join(chainAddress.Approvals, ",")).Msg("fetching solana key from s3")
			// get the solana address from s3
			solanaAddressResponse, err := s3.NewFromConfig(awsCfg).GetObject(ctx, &s3.GetObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String("solana-address-" + solanaPubAddr),
			})
			if err != nil {
				return fmt.Errorf("failed to get solana address from s3: %w", err)
			}
			logger.Debug().Msg("no error reading solana key from s3")
			u.solanaPrivCiphertext, err = io.ReadAll(solanaAddressResponse.Body)
			if err != nil {
				return fmt.Errorf("failed to read solana address bytes: %w", err)
			}
			logger.Debug().Int("solana ciphertext length", len(u.solanaPrivCiphertext)).Msg("setting solana ciphertext to service")
		} else {
			return fmt.Errorf("provided solana address has insufficient approvals")
		}
	}

	return nil
}

func (u *Unsealing) fetchSecretes(
	ctx context.Context,
	logger *zerolog.Logger,
) error {
	// get the secrets object key and bucket name from environment
	secretsBucketName, ok := ctx.Value(appctx.EnclaveSecretsBucketNameCTXKey).(string)
	if !ok {
		return errNoSecretsBucketConfigured
	}

	// download the configuration file, kms decrypt the file
	secretsObjectName, ok := ctx.Value(appctx.EnclaveSecretsObjectNameCTXKey).(string)
	if !ok {
		return errNoSecretsObjectConfigured
	}
	solanaAddress, ok := ctx.Value(appctx.EnclaveSolanaAddressCTXKey).(string)
	if !ok {
		return errNoSolanaAddressConfigured
	}
	logger.Debug().Str("solana address:", solanaAddress).Msg("solana address configured")

	for {
		// fetch the secrets, result will store the secrets (age ciphertext) on the service instance
		if err := u.tryFetchSecrets(ctx, secretsBucketName, secretsObjectName, solanaAddress); err != nil {
			// log the error, we will retry again
			logger.Error().Err(err).Msg("failed to fetch secrets, will retry shortly")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(30 * time.Second):
				continue
			}
		}
		break
	}

	return nil
}

// fetchOperatorShares will take an s3 bucket and fetch all of the operator shares and store them
func (u *Unsealing) tryFetchOperatorShares(ctx context.Context, bucket string) error {
	// clear out all keyshares and start over, we will be downloading ALL shares from the s3 bucket
	u.keyShares = [][]byte{}

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

		data, err := io.ReadAll(shareResponse.Body)
		if err != nil {
			return fmt.Errorf("failed to read operator share from s3 response: %w", err)
		}

		plaintext, err := u.decryptWithNitroKMS(ctx, awsCfg, data)
		if err != nil {
			return err
		}

		// store the decrypted keyShares on the service as [][]byte for later
		share, err := base64.StdEncoding.DecodeString(string(plaintext))
		if err != nil {
			return fmt.Errorf("failed to base64 decode operator key share: %w", err)
		}

		u.keyShares = append(u.keyShares, share)
	}

	return nil
}

func (u *Unsealing) decryptWithNitroKMS(
	ctx context.Context, awsCfg aws.Config, cipherText []byte,
) ([]byte, error) {
	if nitro.EnclaveMocking() {
		// data is not encrypted. Localstack stack does not support nitro
		// attestation/encryption.
		return cipherText, nil
	}

	privateKey, document, err := createAttestationDocument(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create attestation document: %w", err)
	}

	// decrypt with kms key that only enclave can decrypt with
	decryptOutput, err := kms.NewFromConfig(awsCfg).Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob:      cipherText,
		EncryptionAlgorithm: kmsTypes.EncryptionAlgorithmSpecSymmetricDefault,
		KeyId:               aws.String(u.kmsDecryptKeyArn),
		Recipient: &kmsTypes.RecipientInfo{
			AttestationDocument:    document,                                       // attestation document
			KeyEncryptionAlgorithm: kmsTypes.KeyEncryptionMechanismRsaesOaepSha256, // how to decrypt
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt object with kms: %w", err)
	}

	plaintext, err := nitro.Decrypt(privateKey.(*rsa.PrivateKey), decryptOutput.CiphertextForRecipient)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt the ciphertext for recipient from kms: %w", err)
	}
	return plaintext, nil
}

func (u *Unsealing) fetchOperatorShares(
	ctx context.Context,
	logger *zerolog.Logger,
) error {
	// operator shares files
	operatorSharesBucketName, ok := ctx.Value(appctx.EnclaveOperatorSharesBucketNameCTXKey).(string)
	if !ok {
		return errNoOperatorSharesBucketConfigured
	}

	for {
		// do we have enough shares to attempt to reconstitute the key?
		if err := u.tryFetchOperatorShares(ctx, operatorSharesBucketName); err != nil {
			logger.Error().Err(err).Msg("failed to fetch operator shares, will retry shortly")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(60 * time.Second):
				continue
			}
		}
		if len(u.keyShares) >= requiredOperatorShares {
			break
		}
		logger.Error().Msg("need more operator shares to decrypt secrets")
		// no - poll for operator shares until we can attempt to decrypt the file
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(60 * time.Second):
			continue
		}
	}

	// combine the service configured key shares
	privateKey, err := shamir.Combine(u.keyShares)
	if err != nil {
		return fmt.Errorf("failed to combine keyShares: %w", err)
	}

	u.operatorKey, err = age.ParseX25519Identity(string(privateKey))
	if err != nil {
		return fmt.Errorf("failed to parse private key bytes for secret decryption: %w", err)
	}

	return nil
}

// decryptSecrets combines the shamir shares stored on the service instance and decrypts the ciphertext
// returning a map of secret values from the configuration
func (u *Unsealing) decryptSecrets(ctx context.Context) error {
	// do we have secrets downloaded?
	if len(u.secretsCiphertext) < 1 {
		return errors.New("empty configuration ciphertext")
	}

	logger := logging.Logger(ctx, "payments.secrets")
	var output = map[string]string{}

	sec, err := getDecryptReader(ctx, u.operatorKey, u.secretsCiphertext)
	if err != nil {
		return fmt.Errorf("failed to decrypt secrets with shares: %w", err)
	}
	if err := json.NewDecoder(sec).Decode(&output); err != nil {
		return fmt.Errorf("failed to json decode the secrets: %w", err)
	}

	if len(u.solanaPrivCiphertext) > 0 {
		logger.Debug().Int("solana ciphertext length", len(u.solanaPrivCiphertext)).Msg("decrypting solana ciphertext")
		solReader, err := getDecryptReader(ctx, u.operatorKey, u.solanaPrivCiphertext)
		if err != nil {
			return fmt.Errorf("failed to decrypt solana address with shares: %w", err)
		}
		logger.Debug().Msg("decryptWithShares completed without error")
		buf := new(bytes.Buffer)
		buf.ReadFrom(solReader)

		output["solanaPrivateKey"] = base64.StdEncoding.EncodeToString(buf.Bytes())
		logger.Debug().Int("solana key length", len(output["solanaPrivateKey"])).Msg("set decrypted key to secret map")
	}

	u.secrets = output
	return nil
}

func (u *Unsealing) readTestSecretes() error {
	envName := "BAT_PAYMENT_TEST_SECRETS"
	secretsPath := os.Getenv(envName)
	if secretsPath == "" {
		return fmt.Errorf("The environment variable %s is not set", envName)
	}
	f, err := os.Open(secretsPath)
	if err != nil {
		return fmt.Errorf("Failed to open the test secrets from %s - %w", envName, err)
	}

	output := map[string]string{}
	if err := json.NewDecoder(f).Decode(&output); err != nil {
		return fmt.Errorf(
			"failed to json decode the test secretes %s: %w", secretsPath, err)
	}
	u.secrets = output
	return nil
}

func getDecryptReader(
	ctx context.Context,
	key OperatorKey,
	cipherText []byte,
) (io.Reader, error) {
	reader := bytes.NewReader(cipherText)
	return age.Decrypt(reader, key)
}

func encryptToWriter(ctx context.Context, key OperatorKey, data []byte, destination io.Writer) error {

	w, err := age.Encrypt(destination, key.Recipient())
	if err != nil {
		return fmt.Errorf("Failed to create encryption stream: %v", err)
	}
	_, err = w.Write(data)
	err2 := w.Close()
	if err != nil || err2 != nil {
		err = errors.Join(err, err2)
		return fmt.Errorf("Failed to encrypt %d bytes: %w", len(data), err)
	}
	return nil
}
