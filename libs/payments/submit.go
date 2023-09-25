package payments

// SubmitRequest is provided to indicate a payment that should be executed.
type SubmitRequest struct {
	DocumentID string `json:"documentId,omitempty"`
	PayoutID   string `json:"payoutId" valid:"required"`
}

// SubmitResponse is returned to provide the status of a payment after submission, along with any
// error that resulted, if necessary.
type SubmitResponse struct {
	Status PaymentStatus `json:"status" valid:"required"`
}
