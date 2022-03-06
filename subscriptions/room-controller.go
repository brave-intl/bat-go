package subscriptions

import (
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/brave-intl/bat-go/middleware"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/go-chi/chi"
)

type CreateRoomRequest struct {
	MauP bool `json:"mauP,omitempty"`
}

type CreateRoomResponse struct {
	JWT     string `json:"jwt"`
	Refresh string `json:"refresh"`
}

type UpdateModeratorRequest struct {
	MauP bool   `json:"mauP,omitempty"`
	Jwt  string `json:"jwt"`
}

const TalkPremiumSku = "brave-talk-premium"

var (
	errInvalidRoomName     = errors.New("invalid room name")
	errInvalidRequestBody  = errors.New("invalid request body")
	errRoomReachedCapacity = errors.New("room reached max capacity")
	errRoomNotFound        = errors.New("room doesn't exist")
)

func CreateRoomV1Router(service *Service) chi.Router {
	r := chi.NewRouter()
	skuMiddleware := VerifyAnonOptional([]string{TalkPremiumSku}, service.SKUClient)
	r.Method("POST", "/v1/rooms/{name}", middleware.InstrumentHandler("CreateRoom", skuMiddleware(PostCreateRoomHandler(service))))
	r.Method("PUT", "/v1/rooms/{name}", middleware.InstrumentHandler("JoinRoom", PutJoinRoom(service)))
	r.Method("PUT", "/v1/rooms/{name}/moderator", middleware.InstrumentHandler("UpdateModerator", PutUpdateModerator(service)))

	return r
}

func PostCreateRoomHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()
		logger, err := appctx.GetLogger(ctx)
		if err != nil {
			ctx, logger = logging.SetupLogger(ctx)
		}

		roomName := chi.URLParam(r, "name")
		matched, err := regexp.MatchString(`^[A-Za-z0-9-_]{43}$`, roomName)
		if err != nil {
			return handlers.WrapError(err, err.Error(), http.StatusBadRequest)
		}
		if !matched {
			return handlers.WrapError(err, errInvalidRoomName.Error(), http.StatusBadRequest)
		}

		var crq CreateRoomRequest
		err = requestutils.ReadJSON(r.Body, &crq)
		if err != nil {
			return handlers.WrapError(err, errInvalidRequestBody.Error(), http.StatusBadRequest)
		}

		sku, ok := ctx.Value(SkuAuthContextKey).(string)

		roomTier := "free"

		if ok && sku == TalkPremiumSku {
			roomTier = "paid"
		}

		logger.Info().Str("roomName", roomName).Str("sku", sku).Str("roomTier", roomTier).Msg("creating room")

		ro := Room{
			Name: roomName,
			Tier: roomTier,
		}

		err = service.Datastore.createRoom(ro)
		if err != nil {
			conflict := regexp.MustCompile(".*rooms_pkey.*")
			if conflict.MatchString(err.Error()) {
				return handlers.WrapError(err, "Requested name is already in use", http.StatusConflict)
			}
			return handlers.WrapError(err, err.Error(), http.StatusInternalServerError)
		}

		if crq.MauP {
			go service.Datastore.increaseMau()
		}

		var userID string
		// if s.config.Env == "local" || s.config.Env == "development" {
		// 	counter, err := s.repositoryModule.incrRoom(ro)
		// 	if err != nil {
		// 		return s.httpModule.errorResponse(w, req, err.Error(), http.StatusInternalServerError, err)
		// 	}
		// 	userID = fmt.Sprintf("%d", counter-1)
		// }

		is_group_room := roomTier == "paid"

		claim, err := MakeClaim(ctx, ro, crq.MauP, true, is_group_room, userID)
		if err != nil {
			return handlers.WrapError(err, err.Error(), http.StatusInternalServerError)
		}

		jwt, err := MakeJWT(ctx, claim)
		if err != nil {
			return handlers.WrapError(err, err.Error(), http.StatusInternalServerError)
		}

		refresh, err := MakeJWT(ctx, MakeRefreshClaim(ro, true, is_group_room))
		if err != nil {
			return handlers.WrapError(err, err.Error(), http.StatusInternalServerError)
		}

		resp := CreateRoomResponse{
			JWT:     jwt,
			Refresh: refresh,
		}

		return handlers.RenderContent(r.Context(), resp, w, http.StatusCreated)
	})
}

func PutJoinRoom(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()
		logger, err := appctx.GetLogger(ctx)
		if err != nil {
			ctx, logger = logging.SetupLogger(ctx)
		}

		roomName := chi.URLParam(r, "name")
		matched, err := regexp.MatchString(`^[A-Za-z0-9-_]{43}$`, roomName)
		if err != nil {
			return handlers.WrapError(err, err.Error(), http.StatusBadRequest)
		}
		if !matched {
			return handlers.WrapError(err, errInvalidRoomName.Error(), http.StatusBadRequest)
		}

		var crq CreateRoomRequest
		err = requestutils.ReadJSON(r.Body, &crq)
		if err != nil {
			return handlers.WrapError(err, errInvalidRequestBody.Error(), http.StatusBadRequest)
		}

		ro := Room{
			Name: roomName,
		}

		logger.Info().Str("roomName", roomName).Msg("joining room")

		err = service.Datastore.joinRoom(ro)
		if err != nil {
			limit := regexp.MustCompile(".*rooms.*free_head_count_limit.*")
			if limit.MatchString(err.Error()) {
				return handlers.WrapError(err, errRoomReachedCapacity.Error(), http.StatusBadRequest)
			}
			notFound := regexp.MustCompile(".*no rows.*")
			if notFound.MatchString(err.Error()) {
				return handlers.WrapError(err, errRoomNotFound.Error(), http.StatusNotFound)
			}
			return handlers.WrapError(err, err.Error(), http.StatusInternalServerError)
		}

		var userID string
		// if s.config.Env == "local" || s.config.Env == "development" {
		// 	counter, err := s.repositoryModule.incrRoom(ro)
		// 	if err != nil {
		// 		return s.httpModule.errorResponse(w, req, err.Error(), http.StatusInternalServerError, err)
		// 	}
		// 	userID = fmt.Sprintf("%d", counter-1)
		// }

		if crq.MauP {
			go service.Datastore.increaseMau()
		}
		claim, err := MakeClaim(ctx, ro, crq.MauP, false, false, userID)
		if err != nil {
			return handlers.WrapError(err, err.Error(), http.StatusInternalServerError)
		}

		jwt, err := MakeJWT(ctx, &claim)
		if err != nil {
			return handlers.WrapError(err, err.Error(), http.StatusInternalServerError)
		}

		resp := CreateRoomResponse{
			JWT: jwt,
		}

		return handlers.RenderContent(r.Context(), resp, w, http.StatusOK)
	})

}

func PutUpdateModerator(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()
		logger, err := appctx.GetLogger(ctx)
		if err != nil {
			ctx, logger = logging.SetupLogger(ctx)
		}

		roomName := chi.URLParam(r, "name")
		matched, err := regexp.MatchString(`^[A-Za-z0-9-_]{43}$`, roomName)
		if err != nil {
			return handlers.WrapError(err, err.Error(), http.StatusBadRequest)
		}
		if !matched {
			return handlers.WrapError(err, errInvalidRoomName.Error(), http.StatusBadRequest)
		}

		var umr UpdateModeratorRequest
		err = requestutils.ReadJSON(r.Body, &umr)
		if err != nil {
			return handlers.WrapError(err, errInvalidRequestBody.Error(), http.StatusBadRequest)
		}

		old := RoomRefreshClaims{}
		err = MustVerify(ctx, umr.Jwt, &old)
		if err != nil {
			formatReg := regexp.MustCompile(".*JWS format.*")
			if formatReg.MatchString(err.Error()) {
				return handlers.WrapError(err, "Token is corrupted", http.StatusBadRequest)
			}
			return handlers.WrapError(err, err.Error(), http.StatusInternalServerError)
		}
		if old.Aud != "chat" {
			return handlers.WrapError(err, "Token is not a refresh token", http.StatusBadRequest)
		}
		if old.Claims.NotBefore.Time().After(time.Now()) {
			return handlers.WrapError(err, "Token not yet valid", http.StatusBadRequest)
		}
		if old.Claims.Expiry.Time().Before(time.Now()) {
			return handlers.WrapError(err, "Token no longer valid", http.StatusBadRequest)
		}
		if old.Room != roomName {
			return handlers.WrapError(err, "Token is not for the correct room", http.StatusBadRequest)
		}

		ro := Room{
			Name: roomName,
		}

		logger.Info().Str("roomName", roomName).Msg("updating moderator")

		claim, err := MakeClaim(ctx, ro, umr.MauP, old.Moderator, old.Group, "")
		if err != nil {
			return handlers.WrapError(err, err.Error(), http.StatusInternalServerError)
		}

		jwt, err := MakeJWT(ctx, &claim)
		if err != nil {
			return handlers.WrapError(err, err.Error(), http.StatusInternalServerError)
		}

		refresh, err := MakeJWT(ctx, MakeRefreshClaim(ro, true, old.Group))
		if err != nil {
			return handlers.WrapError(err, err.Error(), http.StatusInternalServerError)
		}

		resp := CreateRoomResponse{
			JWT:     jwt,
			Refresh: refresh,
		}

		return handlers.RenderContent(r.Context(), resp, w, http.StatusCreated)
	})
}
