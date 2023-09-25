package payments

const (
	PreparePrefix = "prepare-"
	SubmitPrefix  = "submit-"
)

var (
	PrepareConfigStream         = PreparePrefix + "config"
	PrepareConfigConsumerGroup  = PrepareConfigStream + "-wg"
	PrepareResponseStreamPrefix = PreparePrefix + "response-"
	SubmitConfigStream          = SubmitPrefix + "config"
	SubmitConfigConsumerGroup   = SubmitConfigStream + "-wg"
	SubmitResponseStreamPrefix  = SubmitPrefix + "response-"
)
