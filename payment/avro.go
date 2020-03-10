package payment

const voteSchema = `{
  "namespace": "brave.payments",
  "type": "record",
  "name": "vote",
  "doc": "This message is sent when a user funded wallet has successfully auto-contributed to a channel",
  "fields": [
    { "name": "id", "type": "string" },
    { "name": "type", "type": "string" },
    { "name": "channel", "type": "string" },
    { "name": "createdAt", "type": "string" },
    { "name": "baseVoteValue", "type": "string", "default":"0.25" },
    { "name": "voteTally", "type": "long", "default":1 },
    { "name": "fundingSource", "type": "string", "default": "uphold" }
  ]
}`

// funding source is anon card and the type is one of the 3
// user funded is just ac

// Don't use votes from both anon cards and user funds at same time

// voting is separate from ordering
// voting is on the transaction level - so we'll emit per 'key' + day

// one issuer for user funded ac and one issuer for anon-card transaction
// defined as "fundingSource" anon-card or User Wallet (uphold or coinbase)

// there is no reason to issuerIdentifier, don't know if it's necessarily.
// Reason being regularly rotating issuers isn't going to be as regular as the anonize every 3 days.

// voteTally or number of votes, if we change the vote amounts.
//  What if we decide time partition these votes what we actually want to do is scale these votes to some other things
// Kind of like what we did with anonize
// SOmething that /could/ happen.

// TODO: Can I define a default for an avro schema?
// two separate event types for the same kafka topic - MAYBE
