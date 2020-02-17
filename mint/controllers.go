package mint

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/albrow/forms"
	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/clients/slack"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
)

// Router for promotion endpoints
func Router(service *Service) chi.Router {
	r := chi.NewRouter()
	r.Method("POST", "/slack/actions", middleware.InstrumentHandler("PostMintSlackActions", PostMintSlackActions(service)))
	r.Method("GET", "/slack", middleware.InstrumentHandler("GetMintSlack", GetMintSlack(service)))
	r.Method("POST", "/slack", middleware.InstrumentHandler("PostMintSlack", PostMintSlack(service)))
	return r
}

// GetMintSlack gets the json structure for displaying a slack interactive modal
func GetMintSlack(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		w.WriteHeader(http.StatusOK)
		return nil
	})
}

// SlackText a structure for holding data about a slack placeholder
type SlackText struct {
	Emoji bool   `json:"emoji" valid:"required"`
	Text  string `json:"text" valid:"required"`
	Type  string `json:"type" valid:"required"`
}

// SlackOption an interactive slack option
type SlackOption struct {
	Text  SlackText `json:"text" valid:"required"`
	Value string    `json:"value" valid:"required"`
}

// SlackTeam holds information about the team the command came from
type SlackTeam struct {
	Domain string `json:"domain" valid:"required"`
	ID     string `json:"id" valid:"required"`
}

//

// SlackAction holds an interactive action
type SlackAction struct {
	ActionID       string      `json:"action_id" valid:"required"`
	ActionTS       time.Time   `json:"action_ts" valid:"required"`
	BlockID        string      `json:"block_id" valid:"required"`
	Placeholder    SlackText   `json:"placeholder" valid:"required"`
	SelectedOption SlackOption `json:"selected_option" valid:"required"`
	Type           string      `json:"type" valid:"required"`
}

// PostMintSlackActions handles responding to options
func PostMintSlackActions(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		form, err := forms.Parse(r)
		if err != nil {
			return handlers.WrapError(err, "unable to parse request", http.StatusBadRequest)
		}
		var payload slack.ActionsRequest
		err = json.Unmarshal([]byte(form.Get("payload")), &payload)
		if err != nil {
			return handlers.WrapError(err, "unable to parse payload", http.StatusBadRequest)
		}
		switch payload.Type {
		case "block_actions":
			err := service.HandleBlockAction(r.Context(), payload)
			if err != nil {
				return handlers.WrapError(err, "unable to upsert action", http.StatusBadRequest)
			}
		case "view_submission":
			// the form was submitted
			err := service.HandleViewSubmission(r.Context(), payload)
			if err != nil {
				return handlers.WrapError(err, "unable to complete view submission", http.StatusBadRequest)
			}
		case "view_closed":
			err := service.Datastore().RemoveModalActions(r.Context(), payload.View.CallbackID)
			if err != nil {
				return handlers.WrapError(err, "unable to remove modal actions", http.StatusServiceUnavailable)
			}
		}
		w.WriteHeader(http.StatusOK)
		return nil
	})
}

// PostMintSlackRequest holds data about the form submission
type PostMintSlackRequest struct {
	Token       string `json:"token" valid:"required"`
	TeamID      string `json:"team_id" valid:"required"`
	TeamDomain  string `json:"team_domain" valid:"required"`
	ChannelID   string `json:"channel_id" valid:"required"`
	ChannelName string `json:"channel_name" valid:"required"`
	UserID      string `json:"user_id" valid:"required"`
	UserName    string `json:"user_name" valid:"required"`
	Command     string `json:"command" valid:"required"`
	Text        string `json:"text" valid:"required"`
	ResponseURL string `json:"response_url" valid:"required"`
	TriggerID   string `json:"trigger_id" valid:"required"`
}

// PostMintSlack recieves the form from slack to mint grants
func PostMintSlack(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		form, err := forms.Parse(r)
		if err != nil {
			return handlers.WrapError(err, "unable to parse form", http.StatusBadRequest)
		}
		req := PostMintSlackRequest{
			Token:       form.Get("token"),
			TeamID:      form.Get("team_id"),
			TeamDomain:  form.Get("team_domain"),
			ChannelID:   form.Get("channel_id"),
			ChannelName: form.Get("channel_name"),
			UserID:      form.Get("user_id"),
			UserName:    form.Get("user_name"),
			Command:     form.Get("command"),
			Text:        form.Get("text"),
			ResponseURL: form.Get("response_url"),
			TriggerID:   form.Get("trigger_id"),
		}
		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapError(err, "unable to validate request", http.StatusBadRequest)
		}
		text := req.Text
		split := strings.Split(text, " ")

		switch split[0] {
		case "create":
			_, err := service.SlackClient.PromotionCreateOpenModal(r.Context(), req.TriggerID, uuid.NewV4())
			if err != nil {
				return handlers.WrapError(err, "unable to open modal", http.StatusBadRequest)
			}
		}
		w.WriteHeader(http.StatusOK)
		return nil
	})
}
