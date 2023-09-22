package payments

import (
	"github.com/google/uuid"
)

// idempotencyNamespace is a uuidv5 namespace for creating transaction idempotency keys
var idempotencyNamespace, _ = uuid.Parse("1286fb9f-c6ac-4e97-97a3-9fd866c95926")

// Custodian is a string identifier for a given custodian.
type Custodian string

// String implements stringer interface
func (c Custodian) String() string {
	return string(c)
}
