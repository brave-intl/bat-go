package wallet

import (
	"context"
	"encoding/base64"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/brave-intl/bat-go/libs/ptr"
	"github.com/brave-intl/bat-go/services/wallet/model"
)

type s3Header interface {
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
}

type checkerConfig struct {
	bucket string
}

type solAddrsChecker struct {
	cfg checkerConfig
	s3h s3Header
}

func newSolAddrsChecker(s3h s3Header, cfg checkerConfig) *solAddrsChecker {
	return &solAddrsChecker{
		cfg: cfg,
		s3h: s3h,
	}
}

func (c *solAddrsChecker) IsAllowed(ctx context.Context, addrs string) error {
	key := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString([]byte(addrs))

	param := &s3.HeadObjectInput{
		Bucket: ptr.To(c.cfg.bucket),
		Key:    ptr.To(key),
	}

	if _, err := c.s3h.HeadObject(ctx, param); err != nil {
		var nfe *types.NotFound
		if errors.As(err, &nfe) {
			return nil
		}

		return err
	}

	return model.ErrSolAddrsNotAllowed
}
