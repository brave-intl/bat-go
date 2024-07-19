package payments

const (
	// PreparePrefix is the prefix for streams dealing with prepare events
	PreparePrefix = "prepare-"
	// SubmitPrefix is the prefix for streams dealing with submit events
	SubmitPrefix = "submit-"
	// ResponseSuffix is the suffix for streams dealing with responses
	ResponseSuffix = "-response"
	// Statusuffix is the suffix for the set containing response statuses
	StatusSuffix = "-status"
)

var (
	// PrepareConfigStream is the stream for configuration events for new settlement prepare streams
	PrepareConfigStream = PreparePrefix + "config"
	// PrepareConfigConsumerGroup is the consumergroup for the prepare config stream
	PrepareConfigConsumerGroup = PrepareConfigStream + "-wg"
	// SubmitConfigStream is the stream for configuration events for new settlement submit streams
	SubmitConfigStream = SubmitPrefix + "config"
	// SubmitConfigConsumerGroup is the consumergroup for the submit config stream
	SubmitConfigConsumerGroup = SubmitConfigStream + "-wg"
)
