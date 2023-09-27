package report

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snslib "github.com/brave-intl/bat-go/libs/aws/sns"
	"github.com/brave-intl/bat-go/libs/backoff"
	"github.com/brave-intl/bat-go/libs/backoff/retrypolicy"
)

var (
	retryPolicy = retrypolicy.DefaultRetry
)

type Notifier struct {
	sns   snslib.PublishAPI
	retry backoff.RetryFunc
	topic string
}

func NewNotifier(sns snslib.PublishAPI, topic string, retry backoff.RetryFunc) *Notifier {
	return &Notifier{
		sns:   sns,
		topic: topic,
		retry: retry,
	}
}

type notification struct {
	PayoutID  string `json:"PayoutID"`
	ReportURI string `json:"ReportURI"`
	VersionID string `json:"Version"`
}

// Notify sends a notification to the configured SNS topic.
func (n *Notifier) Notify(ctx context.Context, payoutID, reportURI string, versionID string) error {
	b, err := json.Marshal(notification{
		PayoutID:  payoutID,
		ReportURI: reportURI,
		VersionID: versionID,
	})
	if err != nil {
		return fmt.Errorf("error marshaling notification message: %w", err)
	}

	input := &sns.PublishInput{
		Message:                aws.String(string(b)),
		MessageDeduplicationId: aws.String(payoutID),
		TopicArn:               aws.String(n.topic),
	}

	pubOp := func() (interface{}, error) {
		return n.sns.Publish(ctx, input)
	}

	_, err = n.retry(ctx, pubOp, retryPolicy, func(error) bool { return true })
	if err != nil {
		return fmt.Errorf("error calling publish operation: %w", err)
	}

	return nil
}
