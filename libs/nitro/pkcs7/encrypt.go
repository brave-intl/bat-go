package pkcs7

/*
https://github.com/mozilla-services/pkcs7/blob/master/encrypt.go

The MIT License (MIT)

Copyright (c) 2015 Andrew Smith

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

import (
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
)

type envelopedData struct {
	Version              int
	RecipientInfos       []recipientInfo `asn1:"set"`
	EncryptedContentInfo encryptedContentInfo
}

type encryptedData struct {
	Version              int
	EncryptedContentInfo encryptedContentInfo
}

type recipientInfo struct {
	Version                int
	RecipientIdentifier    []byte `asn1:"tag:0"`
	KeyEncryptionAlgorithm pkix.AlgorithmIdentifier
	EncryptedKey           []byte
}

type encryptedContentInfo struct {
	ContentType                asn1.ObjectIdentifier
	ContentEncryptionAlgorithm pkix.AlgorithmIdentifier
	EncryptedContent           asn1.RawValue `asn1:"tag:0,optional"`
}

const (
	// EncryptionAlgorithmDESCBC is the DES CBC encryption algorithm
	EncryptionAlgorithmDESCBC = iota

	// EncryptionAlgorithmAES128CBC is the AES 128 bits with CBC encryption algorithm
	// Avoid this algorithm unless required for interoperability; use AES GCM instead.
	EncryptionAlgorithmAES128CBC

	// EncryptionAlgorithmAES256CBC is the AES 256 bits with CBC encryption algorithm
	// Avoid this algorithm unless required for interoperability; use AES GCM instead.
	EncryptionAlgorithmAES256CBC

	// EncryptionAlgorithmAES128GCM is the AES 128 bits with GCM encryption algorithm
	EncryptionAlgorithmAES128GCM

	// EncryptionAlgorithmAES256GCM is the AES 256 bits with GCM encryption algorithm
	EncryptionAlgorithmAES256GCM
)

// ContentEncryptionAlgorithm determines the algorithm used to encrypt the
// plaintext message. Change the value of this variable to change which
// algorithm is used in the Encrypt() function.
var ContentEncryptionAlgorithm = EncryptionAlgorithmDESCBC

// ErrUnsupportedEncryptionAlgorithm is returned when attempting to encrypt
// content with an unsupported algorithm.
var ErrUnsupportedEncryptionAlgorithm = errors.New("pkcs7: cannot encrypt content: only DES-CBC, AES-CBC, and AES-GCM supported")

// ErrPSKNotProvided is returned when attempting to encrypt
// using a PSK without actually providing the PSK.
var ErrPSKNotProvided = errors.New("pkcs7: cannot encrypt content: PSK not provided")

type aesGCMParameters struct {
	Nonce  []byte `asn1:"tag:4"`
	ICVLen int
}
