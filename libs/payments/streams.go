package payments

const (
	PreparePrefix = "prepare-"
	SubmitPrefix  = "submit-"
)

var (
	PrepareConfigStream = PreparePrefix + "config"
	SubmitConfigStream  = SubmitPrefix + "config"
)
