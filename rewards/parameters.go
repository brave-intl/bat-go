package rewards

import (
	"github.com/brave-intl/bat-go/utils/sku"
)

// AutoContribute - reward parameters about ac (votes)
type AutoContribute struct {
	Choices       []float64 `json:"choices,omitempty"`
	DefaultChoice float64   `json:"defaultChoice,omitempty"`
}

// Tips - reward parameters about tips (suggestions)
type Tips struct {
	DefaultTipChoices     []float64 `json:"defaultTipChoices,omitempty"`
	DefaultMonthlyChoices []float64 `json:"defaultMonthlyChoices,omitempty"`
}

// ParametersV1 - structure of reward parameters
type ParametersV1 struct {
	BATRate        float64        `json:"batRate,omitempty"`
	AutoContribute AutoContribute `json:"autocontribute,omitempty"`
	Tips           Tips           `json:"tips,omitempty"`
}

// ParametersV2 - structure of reward parameters
type ParametersV2 struct {
	ParametersV1
	SKUTokens sku.Set `json:"skuTokens,omitempty"`
}
