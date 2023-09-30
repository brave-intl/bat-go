package payments

import (
	"context"
	"crypto"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
)

type KMSVerifier struct {
	kmsSigningKeyID string
	kmsClient       wrappedKMSClient
}

func (v KMSVerifier) Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error) {
	verifyOutput, err := v.kmsClient.Verify(context.Background(), &kms.VerifyInput{
		KeyId:            &v.kmsSigningKeyID,
		Message:          message,
		MessageType:      kmsTypes.MessageTypeRaw,
		Signature:        sig,
		SigningAlgorithm: kmsTypes.SigningAlgorithmSpecEcdsaSha256,
	})
	if err != nil {
		return false, fmt.Errorf("failed to verify state signature: %e", err)
	}

	if !verifyOutput.SignatureValid {
		return false, fmt.Errorf("signature was not valid")
	}
	return true, nil
}
