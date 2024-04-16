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

// AddressApprovalRequest is provided to indicate approval of an on-chain address.
type AddressApprovalRequest struct {
	Address string `json:"address" valid:"required"`
}

// SubmitResponse is returned to provide the status of a payment after submission, along with any
// error that resulted, if necessary.
type SubmitResponse struct {
	Status              PaymentStatus `json:"status" valid:"required"`
	PaymentDetails      `json:"paymentDetails,omitempty"`
	ExternalIdempotency string `json:"externalIdempotency,omitempty"`
}
