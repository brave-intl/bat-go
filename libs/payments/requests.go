package payments

// PrepareRequest is provided for the initial creation and preparation of a payment. This payment
// must be unique in the database by idempotencyKey, which is derived from the included
// PaymentDetails.
type PrepareRequest struct {
	PaymentDetails
}

// PrepareResponse is sent to the client in response to a PrepareRequest.
type PrepareResponse struct {
	PaymentDetails
	DocumentID string `json:"documentId,omitempty"`
}

// SubmitRequest is provided to indicate a payment that should be executed.
type SubmitRequest struct {
	DocumentID string `json:"documentId,omitempty"`
	PayoutID   string `json:"payoutId" valid:"required"`
}

// SubmitResponse is returned to provide the status of a payment after submission, along with any
// error that resulted, if necessary.
type SubmitResponse struct {
	Status              PaymentStatus `json:"status" valid:"required"`
	PaymentDetails      `json:"paymentDetails,omitempty"`
	ExternalIdempotency string `json:"externalIdempotency,omitempty"`
}

// AddressApprovalRequest is provided to indicate approval of an on-chain address.
type AddressApprovalRequest struct {
	Address string `json:"address" valid:"required"`
}

// OperatorShare represents the association between an operator name and their encrypted share
type OperatorShareData struct {
	Name     string `json:"name" valid:"required"`
	Material []byte `json:"material" valid:"required"`
}

type OperatorPubkeyData struct {
	Name      string `json:"name" valid:"required"`
	PublicKey string `json:"publicKey" valid:"required"`
}

// CreateVaultRequest is provided to request vault creation for secrets storage.
type CreateVaultRequest struct {
	Threshold int `json:"threshold" valid:"required"`
}

// CreateVaultResponseWrapper is a data wrapper that exposes the service's response object to the
// client
type CreateVaultResponseWrapper struct {
	Data VaultResponse `json:"data"`
}

// VerifyVaultResponseWrapper is a data wrapper that exposes the service's response object to the
// client
type VerifyVaultResponseWrapper struct {
	Data VaultResponse `json:"data"`
}

// VerifyVaultRequest is provided to request vault approval for a given configuration and public
// key. The provided parameters and public key must exist and match in QLDB for approval to succeed.
type VerifyVaultRequest struct {
	Threshold int    `json:"threshold" valid:"required"`
	PublicKey string `json:"publicKey" valid:"required"`
}

// VerifyVaultResponse returns the number of approvals, whether a vault is fully approved, and the
// public key of the approved vault.
type VaultResponse struct {
	PublicKey        string              `json:"publicKey" valid:"required"`
	Threshold        int                 `json:"threshold" valid:"required"`
	OperatorKeys     []string            `json:"operatorKeys" valid:"required"`
	Shares           []OperatorShareData `json:"shares" valid:"required"`
	SigningPublicKey string              `json:"signingPublicKey" valid:"required"`
	Signature        []byte              `json:"signature" valid:"required"`
	SigningData      []byte              `json:"signingData" valid:"required"`
}
