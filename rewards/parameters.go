package rewards

import "github.com/shopspring/decimal"

// AutoContribute - reward parameters about ac (votes)
type AutoContribute struct {
	Choices []float64 `json:"choices,omitempty"`
	Range   []float64 `json:"range,omitempty"`
}

// Tips - reward parameters about tips (suggestions)
type Tips struct {
	DefaultTipChoices     []float64 `json:"defaultTipChoices,omitempty"`
	DefaultMonthlyChoices []float64 `json:"defaultMonthlyChoices,omitempty"`
}

// Parameters - structure of reward parameters
type Parameters struct {
	BATRate        decimal.Decimal `json:"batRate,omitempty"`
	AutoContribute AutoContribute  `json:"autocontribute,omitempty"`
	Tips           Tips            `json:"tips,omitempty"`
}
