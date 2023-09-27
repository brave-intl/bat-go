package report

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/brave-intl/bat-go/services/settlement/internal/payment"

	awsutils "github.com/brave-intl/bat-go/libs/aws"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/ptr"
	"github.com/brave-intl/bat-go/services/settlement/internal/consumer/concurrent"
)

// TODO(#clD11) look at removing the id param and impl. an iterator or stream.

// Source defines the methods need to obtain the items for uploading.
type Source interface {
	// Count returns the total number of items to be uploaded.
	Count(ctx context.Context, id string) (int64, error)
	// Fetch returns the next range of items to be uploaded.
	Fetch(ctx context.Context, id string, start, stop int64) (any, error)
}

type MultiPartUploader struct {
	src          Source
	s3           awsutils.S3UploadAPI
	s3UploadConf awsutils.S3UploadConfig
}

func NewMultiPartUploader(src Source, s3 awsutils.S3UploadAPI, s3UploadConf awsutils.S3UploadConfig) *MultiPartUploader {
	return &MultiPartUploader{
		src:          src,
		s3:           s3,
		s3UploadConf: s3UploadConf,
	}
}

type CompletedUpload struct {
	Location  string
	VersionID string
}

func (m *MultiPartUploader) Upload(ctx context.Context, config payment.Config) (*CompletedUpload, error) {
	l := logging.Logger(ctx, "MultiPartUploader.Upload")

	input := &s3.CreateMultipartUploadInput{
		Bucket:            aws.String(m.s3UploadConf.Bucket),
		Key:               aws.String(config.PayoutID),
		ContentType:       aws.String(m.s3UploadConf.ContentType),
		ChecksumAlgorithm: types.ChecksumAlgorithmSha256,
	}

	multipartUpload, err := m.s3.CreateMultipartUpload(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("error create multipart upload: %w", err)
	}

	count, err := m.src.Count(ctx, config.PayoutID)
	if err != nil {
		return nil, fmt.Errorf("error getting number of items: %w", err)
	}

	if count != int64(config.Count) {
		return nil, fmt.Errorf("error unexpected number of items: expected %d got %d", config.Count, count)
	}

	l.Info().Int64("total_items", count).Msg("starting upload")

	partNum := int32(0)
	fanOut := make([]<-chan uploadedPart, 0)

	// Step through the total transactions by part size and upload each part async.
	for i := int64(0); i < count; i += m.s3UploadConf.PartSize {
		items, err := m.src.Fetch(ctx, config.PayoutID, i, i+m.s3UploadConf.PartSize)
		if err != nil {
			return nil, fmt.Errorf("error getting next items: %w", err)
		}

		b, err := json.Marshal(items)
		if err != nil {
			return nil, fmt.Errorf("error marshalling items: %w", err)
		}

		partNum++
		c := make(chan uploadedPart)
		uploadPartAsync(ctx, c, m.s3, *multipartUpload, partNum, b)
		fanOut = append(fanOut, c)
	}

	completedParts := make([]types.CompletedPart, 0)
	fanIn := concurrent.NewFanIn[uploadedPart]()(ctx, fanOut...)

	for i := 0; i < int(partNum); i++ {
		select {
		case <-ctx.Done():
			return nil, nil
		case part := <-fanIn:
			if part.err != nil {
				return nil, fmt.Errorf("error uploading part: %w", part.err)
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

	l.Info().Interface("parts", completedParts).Msg("uploading parts")

	completeMultipartUpload, err := m.s3.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   multipartUpload.Bucket,
		Key:      multipartUpload.Key,
		UploadId: multipartUpload.UploadId,
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error calling complete multipart upload: %w", err)
	}

	if completeMultipartUpload == nil {
		return nil, errors.New("error complete multipart upload is nil")
	}

	if completeMultipartUpload.Location == nil {
		return nil, errors.New("error complete multipart upload location is nil")
	}

	// Note, the complete multipart upload versionId can be nil if the bucket does not have versionId enabled.
	cu := &CompletedUpload{
		Location:  *completeMultipartUpload.Location,
		VersionID: ptr.StringOr(completeMultipartUpload.VersionId, ""),
	}

	return cu, nil
}

type uploadedPart struct {
	eTag           *string
	part           int32
	checksumSHA256 *string
	err            error
}

func uploadPartAsync(ctx context.Context, resultC chan<- uploadedPart, s3UploadAPI awsutils.S3UploadAPI, output s3.CreateMultipartUploadOutput, partNumber int32, body []byte) {
	go func() {
		defer close(resultC)

		select {
		case <-ctx.Done():
			return
		default:
			input := &s3.UploadPartInput{
				Body:              bytes.NewReader(body),
				Bucket:            output.Bucket,
				Key:               output.Key,
				PartNumber:        partNumber,
				UploadId:          output.UploadId,
				ContentLength:     int64(len(body)),
				ChecksumAlgorithm: types.ChecksumAlgorithmSha256,
			}

			out, err := s3UploadAPI.UploadPart(ctx, input, func(opts *s3.Options) {
				opts.RetryMaxAttempts = 5
			})

			// We need to check the upload output part from AWS is not nil before setting the fields on the
			// upload part. This happens when we get an error, we just want to return the error and part
			// number with zero value fields.
			prt := uploadedPart{
				part: partNumber,
				err:  err,
			}
			if out != nil {
				prt.eTag = out.ETag
				prt.checksumSHA256 = out.ChecksumSHA256
			}

			resultC <- prt
		}
	}()
}
