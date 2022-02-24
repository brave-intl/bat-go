package subscriptions

import (
	"net/http"
	"regexp"

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
	jwt     string `json:"jwt"`
	refresh string `json:"refresh"`
}

const TalkPremiumSku = "brave-talk-premium"

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
			return handlers.WrapError(err, "Invalid room name", http.StatusBadRequest)
		}

		var crq CreateRoomRequest
		err = requestutils.ReadJSON(r.Body, &crq)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
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

		JAASTenantID, ok := ctx.Value(appctx.JAASTenantIDCTXKey).(string)
		if !ok {
			return handlers.WrapError(err, "Invalid JAAS tenant ID in context", http.StatusInternalServerError)
		}

		jwt, err := service.MakeJWT(ro.makeClaim(crq.MauP, JAASTenantID, true, is_group_room, userID), ctx)
		if err != nil {
			return handlers.WrapError(err, err.Error(), http.StatusInternalServerError)
		}

		refresh, err := service.MakeJWT(ro.makeRefreshClaim(true, is_group_room), ctx)
		if err != nil {
			return handlers.WrapError(err, err.Error(), http.StatusInternalServerError)
		}

		resp := CreateRoomResponse{
			jwt:     jwt,
			refresh: refresh,
		}

		return handlers.RenderContent(r.Context(), resp, w, http.StatusOK)
	})
}
