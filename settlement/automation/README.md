## Settlement Automation

Settlement automation uses Redis streams for messaging and implements a message abstraction over the underlying infrastructure (see below). There are six consumers representing the different stages of the settlement process, the first stage is prepare which places transactions in the prepare state and begins the automated flow through each stage.   

When a transaction is added to the prepare stream a message route is attached based on the transaction type for example, ads or grants. At each stage if the message is processed successfully it will be advanced to the next stage in the route otherwise sent to deadletter queue if it reaches a retry limit for that route. 

### Message Format

Redis streams uses field-value pairs and settlement automation expects `data` as the key with a json string with the below schema as the value.

	Message struct {
		ID        uuid.UUID   `json:"id"`
		Type      MessageType `json:"type"`
		Timestamp time.Time   `json:"timestamp"`
		Headers   Headers     `json:"headers"`
		Routing   *Routing    `routing:"routing"`
		Body      string      `json:"body"`
	}

An example message with a transaction for the body 

`'{"id":"796d5bb9-9bec-49cc-b877-fb26e40a60b3","type":"grants","timestamp":"2022-04-26T15:29:12.905143768Z","headers":null,"Routing":null,"body":"{\"idempotencyKey\":\"61e99482-d6a4-401d-a806-178a7d42e3b6\",\"amount\":\"1\",\"to\":\"127f8447-b246-4785-ab73-009d5353da59\",\"from\":\"4b574cea-fc6e-4265-bc76-f48facb552cf\"}"}'`

### Sending messages using redis-cli 

redis command - 

`XADD <stream-name> * data <json-message-as-string>`

`XADD prepare-settlement * data '{"id":"796d5bb9-9bec-49cc-b877-fb26e40a60b3","type":"grants","timestamp":"2022-04-26T15:29:12.905143768Z","headers":null,"Routing":null,"body":"{\"idempotencyKey\":\"61e99482-d6a4-401d-a806-178a7d42e3b6\",\"amount\":\"1\",\"to\":\"127f8447-b246-4785-ab73-009d5353da59\",\"from\":\"4b574cea-fc6e-4265-bc76-f48facb552cf\"}"}'`

## Submit Worker

The submit worker is responsible for submitting transactions to the payment service. Messages should not be added directly to the submit stream but rather added to the prepare stream which will then be advanced through the necessary steps.

An example of a valid submit message with a transaction

```json
{
  "id":"edde94b3-dd74-438c-a6c1-a7e55d16c1ec",
  "type":"grants",
  "timestamp":"2022-04-27T15:01:13.327432493Z",
  "headers":null,
  "Routing":{
    "position":0,
    "slip":[
      {
        "stream":"submit-settlement",
        "onError":"errored-settlement"
      },
      {
        "stream":"submit-status-settlement",
        "onError":"errored-settlement"
      }
    ],
    "errorHandling": {"maxRetries":5, "attempt":0 }
  },
  "body":"{\"idempotencyKey\":\"8888e286-a0ba-480a-b8eb-42f57ce4b49f\",\"custodian\":\"QF6tidKuhy\",\"amount\":\"1\",\"to\":\"b067fd0d-57c3-4466-b920-0f5ea970fc9\",\"from\":\"26c48ada-f876-4c1e-bd59-bfacafaea9d4\",\"documentId\":\"fa4nBNJE5U\"}"
}
```