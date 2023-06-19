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
	"github.com/brave-intl/bat-go/libs/clients/payment"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/services/settlement/event"
	"github.com/brave-intl/bat-go/services/settlement/payout"
)

type PreparedTransactionAPI interface {
	GetNumberOfPreparedTransactions(ctx context.Context, payoutID string) (int64, error)
	GetPreparedTransactionsByRange(ctx context.Context, payoutID string, start, stop int64) ([]payment.AttestedTransaction, error)
}

type PreparedTransactionUploadClient struct {
	preparedTransactionAPI PreparedTransactionAPI
	s3UploadAPI            awsutils.S3UploadAPI
	s3UploadConfig         awsutils.S3UploadConfig
}

func NewPreparedTransactionUploadClient(preparedTransactionAPI PreparedTransactionAPI, s3UploadAPI awsutils.S3UploadAPI,
	s3Config awsutils.S3UploadConfig) *PreparedTransactionUploadClient {
	return &PreparedTransactionUploadClient{
		preparedTransactionAPI: preparedTransactionAPI,
		s3UploadAPI:            s3UploadAPI,
		s3UploadConfig:         s3Config,
	}
}

func (r *PreparedTransactionUploadClient) Upload(ctx context.Context, config payout.Config) error {
	logger := logging.Logger(ctx, "PreparedTransactionUploadClient.Upload")

	input := &s3.CreateMultipartUploadInput{
		Bucket:            aws.String(r.s3UploadConfig.Bucket),
		Key:               aws.String(config.PayoutID),
		ContentType:       aws.String(r.s3UploadConfig.ContentType),
		ChecksumAlgorithm: types.ChecksumAlgorithmSha256,
	}

	multipartUpload, err := r.s3UploadAPI.CreateMultipartUpload(ctx, input)
	if err != nil {
		return fmt.Errorf("error create multipart upload: %w", err)
	}

	totalTransactions, err := r.preparedTransactionAPI.GetNumberOfPreparedTransactions(ctx, config.PayoutID)
	if err != nil {
		return fmt.Errorf("error getting number of prepared transactions: %w", err)
	}

	if totalTransactions != int64(config.Count) {
		return fmt.Errorf("error unexpected number of transactions: expected %d got %d", config.Count, totalTransactions)
	}

	logger.Info().Int64("total_transactions", totalTransactions).Msg("starting upload")

	partNum := int32(0)
	fanOut := make([]<-chan uploadedPart, 0)

	// Step through the total transactions by part size and upload each part async.
	for i := int64(0); i < totalTransactions; i += r.s3UploadConfig.PartSize {
		t, err := r.preparedTransactionAPI.GetPreparedTransactionsByRange(ctx, config.PayoutID, i, i+r.s3UploadConfig.PartSize)
		if err != nil {
			return fmt.Errorf("error getting prepared transactions by range: %w", err)
		}

		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Errorf("error marshalling prepared transactions: %w", err)
		}

		partNum++
		c := make(chan uploadedPart)
		uploadPartAsync(ctx, c, r.s3UploadAPI, *multipartUpload, partNum, b)
		fanOut = append(fanOut, c)
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
		Bucket:   multipartUpload.Bucket,
		Key:      multipartUpload.Key,
		UploadId: multipartUpload.UploadId,
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		return fmt.Errorf("error calling complete multipart upload: %w", err)
	}

	return nil
}

type uploadedPart struct {
	eTag           *string
	part           int32
	checksumSHA256 *string
	err            error
}

func uploadPartAsync(ctx context.Context, resultC chan<- uploadedPart, s3UploadAPI awsutils.S3UploadAPI,
	output s3.CreateMultipartUploadOutput, partNumber int32, body []byte) {
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

			// We need to check the upload output part from AWS is not nil before setting the fields on the upload part.
			// This happens when we get an error, we just want return the error and part number with zero value fields.
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
}
