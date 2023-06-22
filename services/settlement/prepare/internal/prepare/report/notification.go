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

type NotificationClient struct {
	publisherAPI snslib.PublishAPI
	retry        backoff.RetryFunc
	topic        string
}

func NewNotificationClient(publisherAPI snslib.PublishAPI, topic string, retry backoff.RetryFunc) *NotificationClient {
	return &NotificationClient{
		publisherAPI: publisherAPI,
		topic:        topic,
		retry:        retry,
	}
}

type notification struct {
	PayoutID  string `json:"PayoutID"`
	ReportURI string `json:"ReportURI"`
	VersionID string `json:"Version"`
}

// SendNotification sends a notification to the configured SNS topic. SendNotification does not guarantee delivery but
// is configured to retry many times.
func (n *NotificationClient) SendNotification(ctx context.Context, payoutID, reportURI string, versionID string) error {
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

	publishOperation := func() (interface{}, error) {
		return n.publisherAPI.Publish(ctx, input)
	}

	_, err = n.retry(ctx, publishOperation, retryPolicy, func(error) bool { return true })
	if err != nil {
		return fmt.Errorf("error calling publish operation: %w", err)
	}

	return nil
}
