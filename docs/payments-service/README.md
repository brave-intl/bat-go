# Payments Service Documentation

## Payments Automation

The workflow for payouts starts with automation in the form of a
cron job, manual run, or event driven action (such as upload to payout report bucket).
Upon instantiating action, the automation will parse the payout report and
translate it and submit it to the payments GRPC prepare method, [defined here](../../protos/payments-api.proto).

[Payments Automation Diagram](NitroPaymentsAutomation.pdf) shows the components involved.
[Payments Automation Issue](https://github.com/brave-intl/bat-go/issues/992) is where this we are tracking progress.

It is important to note that automation state should be kept, to make sure the automation retries
in the event of transient failures, and reports on errors that are not retriable.  To accomplish this
it is suggested that within the namespace a data store (redis) is used to track completion of prepare
method calls for (each) transaction(s) and retry ones that warrant a retry.

Below is a list of GRPC Status Codes that one could potentially get from the payment service:

1. `OK` - No Retry; Successfully Submitted Transaction(s)
2. `CANCELLED` - Retry; Client Cancelled
3. `DEADLINE_EXCEEDED` - Retry; Server Slow
4. `RESOURCE_EXHAUSTED` - Retry; Rate limit
5. `ABORTED` - Retry; Server Aborted
6. `UNAVAILABLE` - Retry; Server issue
7. `INTERNAL` - Retry; Raise Failure
8. `UNKNOWN` - Retry; Raise Failure
9. `UNAUTHENTICATED` - No Retry; Raise Failure
10. `INVALID_ARGUMENT` - No Retry; Raise Failure
11. `UNKNOWN` - No Retry; Raise Failure
12. `ALREADY_EXISTS` - No Retry; Successfully Submitted


