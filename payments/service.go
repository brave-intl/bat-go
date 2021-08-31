package payments

import (
	"context"
	"errors"
	"fmt"

	"github.com/brave-intl/bat-go/payments/pb"
	appctx "github.com/brave-intl/bat-go/utils/context"
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
	logger := logging.Logger(ctx, "payments.authorize")

	// is the document id valid from the request
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

	// get environment from context
	env, ok := ctx.Value(appctx.EnvironmentCTXKey).(string)
	if !ok {
		logger.Error().Msg("failed to get environment from context")
		return &pb.AuthorizeResponse{
			Meta: &pb.MetaResponse{
				Status: pb.MetaResponse_FAILURE,
			},
		}, status.Errorf(codes.Internal, "failed to get datastore from the context")
	}
	// is this public key in our list of authorized keys?
	if keys, ok := authorizedKeys[env]; ok {
		// is our key in here, and does the signature validate?
		if !isKeyValid(ar.PublicKey, keys) {
			logger.Error().
				Str("environment", env).
				Str("pub_key", ar.PublicKey).
				Msg("invalid public key for payment authorizations")
			return &pb.AuthorizeResponse{
				Meta: &pb.MetaResponse{
					Status: pb.MetaResponse_FAILURE,
				},
			}, status.Errorf(codes.InvalidArgument, "invalid public key for payments authorization")
		}

		// does the signature validate?
		if !isSignatureValid(ar.DocumentId, ar.PublicKey, ar.Signature) {
			logger.Error().
				Str("environment", env).
				Str("pub_key", ar.PublicKey).
				Str("doc_id", ar.DocumentId).
				Str("signature", ar.Signature).
				Msg("invalid signature for authorization")
			return &pb.AuthorizeResponse{
				Meta: &pb.MetaResponse{
					Status: pb.MetaResponse_FAILURE,
				},
			}, status.Errorf(codes.InvalidArgument, "invalid signature for authorization")
		}
	} else {
		logger.Error().
			Str("environment", env).
			Msg("failed to get authorized keys for environment")
		return &pb.AuthorizeResponse{
			Meta: &pb.MetaResponse{
				Status: pb.MetaResponse_FAILURE,
			},
		}, status.Errorf(codes.Internal, "failed to get authorized keys for environment")
	}

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

	authID := uuid.New()

	err := db.RecordAuthorization(ctx, &Authorization{
		ID:        &authID,
		DocID:     ar.DocumentId,
		PublicKey: ar.PublicKey,
		Signature: ar.Signature,
	})
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
