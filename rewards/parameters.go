package rewards

// AutoContribute - reward parameters about ac (votes)
type AutoContribute struct {
	Choices []float64 `json:"choices,omitempty"`
	Range   []float64 `json:"range,omitempty"`
}

// Tips - reward parameters about tips (suggestions)
type Tips struct {
	Choices []float64 `json:"choices,omitempty"`
}

// Parameters - structure of reward parameters
type Parameters struct {
	Fee            float64        `json:"fee,omitempty"`
	AutoContribute AutoContribute `json:"autocontribute,omitempty"`
	Tips           Tips           `json:"tips,omitempty"`
}
