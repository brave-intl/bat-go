package payments

const (
	PreparePrefix  = "prepare-"
	SubmitPrefix   = "submit-"
	ResponseSuffix = "-response"
)

var (
	PrepareConfigStream        = PreparePrefix + "config"
	PrepareConfigConsumerGroup = PrepareConfigStream + "-wg"
	SubmitConfigStream         = SubmitPrefix + "config"
	SubmitConfigConsumerGroup  = SubmitConfigStream + "-wg"
)
