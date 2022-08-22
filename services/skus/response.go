package skus

// SubmitReceiptResponseV1 - response from submit receipt
type SubmitReceiptResponseV1 struct {
	ExternalID string `json:"externalId"`
	Vendor     string `json:"vendor"`
}
