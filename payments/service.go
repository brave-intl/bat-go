package payments

import (
	"context"
	"fmt"

	"github.com/brave-intl/bat-go/payments/pb"
	"github.com/google/uuid"
)

// Service - Implementation of the PaymentsGRPCServerService from bat-go/payments/pb
type Service struct{}

// Prepare - implement payments GRPC service
func (s *Service) Prepare(ctx context.Context, pr *pb.PrepareRequest) (*pb.PrepareResponse, error) {
	docID, err := InitializeBatchedTXs(ctx, pr.Custodian, pr.BatchTxs)
	if err != nil {
		return &pb.PrepareResponse{
			Meta: &pb.MetaResponse{
				Status: pb.MetaResponse_FAILURE,
			},
		}, fmt.Errorf("error initializing batched txs: %w", err)
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

	// auths, txs, err := ...
	_, _, err = RetrieveTransactionsByID(ctx, &documentID)
	if err != nil {
		pbErr := fmt.Errorf("failed to retrieve transactions by document id: %w", err)
		return &pb.SubmitResponse{
			Meta: &pb.MetaResponse{
				Status: pb.MetaResponse_FAILURE,
				Msg:    pbErr.Error(),
			}}, pbErr
	}

	// TODO: validate the authentications meet business requirements

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
