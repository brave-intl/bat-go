package responses

// Meta - generic api output metadata
type Meta struct {
	Status  string                 `json:"status,omitempty"`
	Message string                 `json:"message,omitempty"`
	Context map[string]interface{} `json:"context,omitempty"`
}
