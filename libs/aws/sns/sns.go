package sns

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
)

type PublishAPI interface {
	Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
}

type Client struct {
	PublishAPI
}

func New(cfg aws.Config) *Client {
	c := sns.NewFromConfig(cfg)
	return &Client{
		PublishAPI: c,
	}
}
