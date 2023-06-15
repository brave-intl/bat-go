package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	awsutils "github.com/brave-intl/bat-go/libs/aws"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/services/settlement/event"
	"github.com/brave-intl/bat-go/services/settlement/payout"
)

type RedisUploader struct {
	redis          *event.RedisClient
	s3UploadAPI    awsutils.S3UploadAPI
	s3UploadConfig awsutils.S3UploadConfig
}

func NewRedisUploader(redis *event.RedisClient, s3UploadAPI awsutils.S3UploadAPI, s3Config awsutils.S3UploadConfig) *RedisUploader {
	return &RedisUploader{
		redis:          redis,
		s3UploadAPI:    s3UploadAPI,
		s3UploadConfig: s3Config,
	}
}

func (r *RedisUploader) Upload(ctx context.Context, config payout.Config) error {
	logger := logging.Logger(ctx, "RedisUploader.Upload")

	input := &s3.CreateMultipartUploadInput{
		Bucket:            aws.String(r.s3UploadConfig.Bucket),
		Key:               aws.String(config.PayoutID),
		ContentType:       aws.String(r.s3UploadConfig.ContentType),
		ChecksumAlgorithm: types.ChecksumAlgorithmSha256,
	}

	out, err := r.s3UploadAPI.CreateMultipartUpload(ctx, input)
	if err != nil {
		return fmt.Errorf("error create multipart upload: %w", err)
	}

	card, err := r.redis.ZCard(ctx, payout.PreparedTransactionsPrefix+config.PayoutID).Result()
	if err != nil {
		return fmt.Errorf("error calling zcard: %w", err)
	}

	if card != int64(config.Count) {
		return fmt.Errorf("error unexpected number of transactions: expected %d got %d", config.Count, card)
	}

	logger.Info().Int64("total_transactions", card).Msg("starting upload")

	partNum := int32(0)
	fanOut := make([]<-chan uploadedPart, 0)

	for i := int64(0); i < card; i += r.s3UploadConfig.PartSize {
		members, err := r.redis.ZRange(ctx, payout.PreparedTransactionsPrefix+config.PayoutID, i, i+r.s3UploadConfig.PartSize).Result()
		if err != nil {
			return fmt.Errorf("error calling zrange: %w", err)
		}

		b, err := json.Marshal(members)
		if err != nil {
			return fmt.Errorf("error marshalling members: %w", err)
		}

		partNum++
		fanOut = append(fanOut, uploadPartAsync(ctx, r.s3UploadAPI, *out, partNum, b))
	}

	completedParts := make([]types.CompletedPart, 0)
	fanIn := event.NewFanIn[uploadedPart]()(ctx, fanOut...)

	for i := 0; i < int(partNum); i++ {
		select {
		case <-ctx.Done():
			return nil
		case part := <-fanIn:
			if part.err != nil {
				return fmt.Errorf("error uploading part: %w", part.err)
			}
			completedParts = append(completedParts, types.CompletedPart{
				ETag:           part.eTag,
				PartNumber:     part.part,
				ChecksumSHA256: part.checksumSHA256,
			})
		}
	}

	sort.Slice(completedParts, func(i, j int) bool {
		return completedParts[i].PartNumber < completedParts[j].PartNumber
	})

	logger.Info().Interface("parts", completedParts).Msg("uploading parts")

	_, err = r.s3UploadAPI.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   out.Bucket,
		Key:      out.Key,
		UploadId: out.UploadId,
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		return fmt.Errorf("error complete multipart upload: %w", err)
	}

	return nil
}

type uploadedPart struct {
	eTag           *string
	part           int32
	checksumSHA256 *string
	err            error
}

func uploadPartAsync(ctx context.Context, s3UploadAPI awsutils.S3UploadAPI, output s3.CreateMultipartUploadOutput, partNumber int32, body []byte) <-chan uploadedPart {
	resultC := make(chan uploadedPart)
	go func() {
		defer close(resultC)

		select {
		case <-ctx.Done():
			return
		default:
			params := &s3.UploadPartInput{
				Body:              bytes.NewReader(body),
				Bucket:            output.Bucket,
				Key:               output.Key,
				PartNumber:        partNumber,
				UploadId:          output.UploadId,
				ContentLength:     int64(len(body)),
				ChecksumAlgorithm: types.ChecksumAlgorithmSha256,
			}

			u, err := s3UploadAPI.UploadPart(ctx, params, func(options *s3.Options) {
				options.RetryMaxAttempts = 5
			})

			// We need to check the upload output from aws is not nil, for example when we get an error,
			// before setting the fields on the uploaded part.
			up := uploadedPart{
				part: partNumber,
				err:  err,
			}
			if u != nil {
				up.eTag = u.ETag
				up.checksumSHA256 = u.ChecksumSHA256
			}

			resultC <- up
		}
	}()
	return resultC
}
