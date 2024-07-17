package httpsignature

import (
	"crypto"
	"os"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/nitro"
)

func TestVerifyNitroAttestation(t *testing.T) {
	pcr3 := "e48b6ac6bab30e3717d28c2c88f2ba8b614e454590eb00b26170eef0d707b5b8e3a97662c20b2ced6192d3aaa2f5e24e"
	pcr3Decoded, err := nitro.ParsePCRHex(pcr3)
	if err != nil {
		t.Fatal("error decoding PCR 3:", err)
	}
	pcrs := nitro.PCRMap{
		3: pcr3Decoded,
	}

	// FIXME we need a better attestation doc sample
	// this one is over a nil userData
	// https://github.com/aws-samples/aws-iot-validate-enclave-attestation
	doc, err := os.ReadFile("att_doc_sample.bin")
	if err != nil {
		t.Fatal("error reading sample attestation doc:", err)
	}

	verifier := nitro.Verifier{
		PCRs:             pcrs,
		VerificationTime: time.Date(2023, time.March, 28, 12, 0, 0, 0, time.UTC),
	}

	valid, err := verifier.Verify(nil, doc, crypto.Hash(0))
	if err != nil {
		t.Fatal("error verifying sample attestation doc:", err)
	}
	if !valid {
		t.Fatal("sample attestation doc was invalid")
	}

	valid, err = verifier.Verify([]byte{0}, doc, crypto.Hash(0))
	if err != nil {
		t.Fatal("error verifying sample attestation doc:", err)
	}
	if valid {
		t.Fatal("sample attestation doc should be invalid for different message")
	}
}
