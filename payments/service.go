package payments

import (
	"context"
	"errors"
	"fmt"

	"github.com/brave-intl/bat-go/payments/pb"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/logging"
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

	// get database from context
	db, ok := ctx.Value(appctx.DatastoreCTXKey).(Datastore)
	if !ok {
		logger.Error().Msg("failed to get the datastore from the context")
		return &pb.PrepareResponse{
			Meta: &pb.MetaResponse{
				Status: pb.MetaResponse_FAILURE,
			},
		}, status.Errorf(codes.Internal, "failed to get datastore from the context")
	}
	// TODO: get qldb from context
	//qldb, ok := ctx.Value(appctx.QLDBSessionCTXKey).(Datastore)

	for _, tx := range pr.GetBatchTxs() {
		logger.Debug().
			Str("dest", tx.Destination).
			Str("orig", tx.Origin).
			Str("amnt", tx.Amount).
			Msg("transaction being prepared")
	}

	docID, err := db.PrepareBatchedTXs(ctx, pr.Custodian, pr.BatchTxs)
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
	if docID == nil {
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
		DocumentId: *docID,
	}, nil
}

// Authorize - implement payments GRPC service
func (s *Service) Authorize(ctx context.Context, ar *pb.AuthorizeRequest) (*pb.AuthorizeResponse, error) {
	// TODO: perform authorization validation here, create an authorization
	auth := new(Authorization)

	logger := logging.Logger(ctx, "payments.authorize")

	// get database from context
	db, ok := ctx.Value(appctx.DatastoreCTXKey).(Datastore)
	if !ok {
		logger.Error().Msg("failed to get the datastore from the context")
		return &pb.AuthorizeResponse{
			Meta: &pb.MetaResponse{
				Status: pb.MetaResponse_FAILURE,
			},
		}, status.Errorf(codes.Internal, "failed to get datastore from the context")
	}

	// after authorization validation:
	if ar.DocumentId == "" {
		err := fmt.Errorf("document id cannot be empty")
		logger.Warn().Err(err).Msg("failed to parse document id")
		return &pb.AuthorizeResponse{
			Meta: &pb.MetaResponse{
				Status: pb.MetaResponse_FAILURE,
				Msg:    err.Error(),
			},
		}, status.Errorf(codes.InvalidArgument, "failed to parse document id: %s", err.Error())
	}

	err := db.RecordAuthorization(ctx, auth, ar.DocumentId)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to record authorization")
		return &pb.AuthorizeResponse{
			Meta: &pb.MetaResponse{
				Status: pb.MetaResponse_FAILURE,
				Msg:    fmt.Sprintf("failed to record authorization: %s", err.Error()),
			},
		}, status.Errorf(codes.Internal, "failed to record authorization: %s", err.Error())
	}

	return &pb.AuthorizeResponse{
		Meta: &pb.MetaResponse{
			Status: pb.MetaResponse_SUCCESS,
		},
	}, nil
}

// Submit - implement payments GRPC service
func (s *Service) Submit(ctx context.Context, sm *pb.SubmitRequest) (*pb.SubmitResponse, error) {
	logger := logging.Logger(ctx, "payments.submit")

	if sm.DocumentId == "" {
		err := fmt.Errorf("document id cannot be empty")
		logger.Warn().Err(err).Msg("failed to parse document id")
		return &pb.SubmitResponse{
			Meta: &pb.MetaResponse{
				Status: pb.MetaResponse_FAILURE,
				Msg:    err.Error(),
			},
		}, status.Errorf(codes.InvalidArgument, "failed to parse document id: %s", err.Error())
	}

	logger.Debug().Str("documentID", sm.DocumentId).Msg("setting state of txs to submitted")

	// TODO: perform the calls to custodians?

	return &pb.SubmitResponse{
		Meta: &pb.MetaResponse{
			Status: pb.MetaResponse_SUCCESS,
		},
	}, nil
}
