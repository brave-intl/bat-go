package event

// Settlement streams and consumer groups
const (
	PrepareStream             = "prepare-settlement"
	PrepareConsumerGroup      = "prepare-consumer-group-settlement"
	SubmitStream              = "submit-settlement"
	SubmitConsumerGroup       = "submit-consumer-group-settlement"
	NotifyStream              = "notify-settlement"
	SubmitStatusStream        = "submit-status-settlement"
	SubmitStatusConsumerGroup = "submit-status-consumer-group-settlement"
	CheckStatusStream         = "check-status-settlement"
	CheckStatusConsumerGroup  = "check-status-consumer-group-settlement"
	ErroredStream             = "errored-settlement"
)

// Settlement MessageType
const (
	Grants   = MessageType("grants")
	Ads      = MessageType("ads")
	Creators = MessageType("creators")
)

// DeadLetterQueue general deadletter queue for settlement
const DeadLetterQueue = "settlement-dlq"
