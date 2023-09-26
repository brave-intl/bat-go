package payments

import ()

// PrepareRequest is provided for the initial creation and preparation of a payment. This payment
// must be unique in the database by idempotencyKey, which is derived from the included
// PaymentDetails.
type PrepareRequest struct {
	PaymentDetails
	DryRun *string `json:"dryRun"`
}

// PrepareResponse is sent to the client in response to a PrepareRequest.
type PrepareResponse struct {
	PaymentDetails
	DocumentID string `json:"documentId,omitempty"`
}
