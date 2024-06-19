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

// CreateVaultResponse provides shares, associated with names provided in CreateVaultRequest, as
// well as the public key resulting from creation and the threshold specified in the request.
type CreateVaultResponse struct {
	Shares    []OperatorShareData `json:"operatorData" valid:"required"`
	PublicKey string              `json:"publicKey" valid:"required"`
	Threshold int                 `json:"threshold" valid:"required"`
}

// CreateVaultResponseWrapper is a data wrapper that exposes the service's response object to the
// client
type CreateVaultResponseWrapper struct {
	Data CreateVaultResponse `json:"data"`
}

// VerifyVaultResponseWrapper is a data wrapper that exposes the service's response object to the
// client
type VerifyVaultResponseWrapper struct {
	Data Vault `json:"data"`
}

// VerifyVaultRequest is provided to request vault approval for a given configuration and public
// key. The provided parameters and public key must exist and match in QLDB for approval to succeed.
type VerifyVaultRequest struct {
	Threshold int    `json:"threshold" valid:"required"`
	PublicKey string `json:"publicKey" valid:"required"`
}

// Vault represents a key which has been broken into shamir shares and is used for encrypting
// secrets. It's shared between the service and the tooling and is tagged accordingly.
type Vault struct {
	PublicKey        string   `ion:"publicKey" json:"publicKey" valid:"required"`
	Threshold        int      `ion:"threshold" json:"threshold" valid:"required"`
	OperatorKeys     []string `ion:"operatorKeys" json:"operatorKeys" valid:"required"`
	Signature        []byte   `ion:"signature" json:"signature" valid:"required"`
	SigningPublicKey string   `ion:"signingPublicKey" json:"signingPublicKey" valid:"required"`
	SignedData       []byte   `ion:"signedData" json:"signedData" valid:"required"`
}
