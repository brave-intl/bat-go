package event

// Settlement streams and consumer groups
const (
	PrepareStream             = "prepare-settlement"
	PrepareConsumerGroup      = "prepare-consumer-group-settlement"
	SubmitStream              = "submit-settlement"
	SubmitConsumerGroup       = "submit-consumer-group-settlement"
	NotifyStream              = "notify-settlement"
	NotifyConsumerGroup       = "notify-consumer-group-settlement"
	SubmitStatusStream        = "submit-status-settlement"
	SubmitStatusConsumerGroup = "submit-status-consumer-group-settlement"
	CheckStatusStream         = "check-status-settlement"
	CheckStatusConsumerGroup  = "check-status-consumer-group-settlement"
	ErroredStream             = "errored-settlement"
	DeadLetterQueue           = "dlq-settlement"
)

const (
	Deadletter = MessageType("dead-letter")
)

// Settlement MessageType
const (
	Grants   = MessageType("grants")
	Ads      = MessageType("ads")
	Creators = MessageType("creators")
)
