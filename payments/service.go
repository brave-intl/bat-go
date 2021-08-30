package payments

import (
	"context"
	"errors"
	"fmt"

	"github.com/brave-intl/bat-go/payments/pb"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Service - Implementation of the PaymentsGRPCServerService from bat-go/payments/pb
type Service struct {
	pb.UnimplementedPaymentsGRPCServiceServer
}

// Prepare - implement payments GRPC service
func (s *Service) Prepare(ctx context.Context, pr *pb.PrepareRequest) (*pb.PrepareResponse, error) {
	logger := logging.Logger(ctx, "payments.prepare")

	for _, tx := range pr.GetBatchTxs() {
		logger.Debug().
			Str("dest", tx.Destination).
			Str("orig", tx.Origin).
			Str("amnt", tx.Amount).
			Msg("transaction being prepared")
	}

	docID, err := PrepareBatchedTXs(ctx, pr.Custodian, pr.BatchTxs)
	if err != nil {
		logger.Error().Err(err).Msg("failed to initialize batched txs")
		if errors.Is(err, errorutils.ErrNotImplemented) {
			return &pb.PrepareResponse{
				Meta: &pb.MetaResponse{
					Status: pb.MetaResponse_FAILURE,
				},
			}, status.Errorf(codes.Unimplemented, "error initializing batched txs: %s", err.Error())
		}
		return &pb.PrepareResponse{
			Meta: &pb.MetaResponse{
				Status: pb.MetaResponse_FAILURE,
			},
		}, status.Errorf(codes.Unknown, "error initializing batched txs: %s", err.Error())
	}

	return &pb.PrepareResponse{
		Meta: &pb.MetaResponse{
			Status: pb.MetaResponse_SUCCESS,
		},
		DocumentId: docID.String(),
	}, nil
}

// Authorize - implement payments GRPC service
func (s *Service) Authorize(ctx context.Context, ar *pb.AuthorizeRequest) (*pb.AuthorizeResponse, error) {
	// TODO: perform authorization validation here, create an authorization
	auth := new(Authorization)

	// after authorization validation:
	documentID, err := uuid.Parse(ar.DocumentId)
	if err != nil {
		pbErr := fmt.Errorf("failed to parse document id: %w", err)
		return &pb.AuthorizeResponse{
			Meta: &pb.MetaResponse{
				Status: pb.MetaResponse_FAILURE,
				Msg:    pbErr.Error(),
			},
		}, pbErr
	}

	err = RecordAuthorization(ctx, auth, &documentID)
	if err != nil {
		pbErr := fmt.Errorf("failed to record authorization: %w", err)
		return &pb.AuthorizeResponse{
			Meta: &pb.MetaResponse{
				Status: pb.MetaResponse_FAILURE,
				Msg:    pbErr.Error(),
			}}, pbErr
	}

	return &pb.AuthorizeResponse{
		Meta: &pb.MetaResponse{
			Status: pb.MetaResponse_SUCCESS,
		},
	}, nil
}

// Submit - implement payments GRPC service
func (s *Service) Submit(ctx context.Context, sm *pb.SubmitRequest) (*pb.SubmitResponse, error) {
	documentID, err := uuid.Parse(sm.DocumentId)
	if err != nil {
		pbErr := fmt.Errorf("failed to parse document id: %w", err)
		return &pb.SubmitResponse{
			Meta: &pb.MetaResponse{
				Status: pb.MetaResponse_FAILURE,
				Msg:    pbErr.Error(),
			},
		}, pbErr
	}

	auths, txs, err := RetrieveTransactionsByID(ctx, &documentID)
	if err != nil {
		pbErr := fmt.Errorf("failed to retrieve transactions by document id: %w", err)
		return &pb.SubmitResponse{
			Meta: &pb.MetaResponse{
				Status: pb.MetaResponse_FAILURE,
				Msg:    pbErr.Error(),
			}}, pbErr
	}

	// TODO: validate the authentications meet business requirements
	fmt.Printf("auths: %+v\ntxs: %+v\n", auths, txs)

	// record the state change
	err = RecordStateChange(ctx, &documentID, pb.State_IN_PROGRESS)
	if err != nil {
		pbErr := fmt.Errorf("failed to retrieve transactions by document id: %w", err)
		return &pb.SubmitResponse{
			Meta: &pb.MetaResponse{
				Status: pb.MetaResponse_FAILURE,
				Msg:    pbErr.Error(),
			}}, pbErr
	}

	// TODO: perform the calls to custodians?

	return &pb.SubmitResponse{
		Meta: &pb.MetaResponse{
			Status: pb.MetaResponse_SUCCESS,
		},
	}, nil
}
