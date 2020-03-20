package slack

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/brave-intl/bat-go/utils/clients"
	uuid "github.com/satori/go.uuid"
)

func createPromotionCreateOpenModal(triggerID string, workflowID uuid.UUID) []byte {
	str := []byte(fmt.Sprintf(`{
		"trigger_id": "%s",
		"view": {
			"notify_on_close": true,
			"type": "modal",
			"callback_id": "%s",
			"title": {
				"type": "plain_text",
				"text": "Create Promotions",
				"emoji": true
			},
			"submit": {
				"type": "plain_text",
				"text": "Submit",
				"emoji": true
			},
			"close": {
				"type": "plain_text",
				"text": "Cancel",
				"emoji": true
			},
			"blocks": [
				{
					"type": "input",
					"block_id": "wallet_id",
					"optional": true,
					"element": {
						"type": "plain_text_input",
						"action_id": "wallet_id",
						"placeholder": {
							"type": "plain_text",
							"text": "00000000-0000-4000-0000-000000000000"
						}
					},
					"label": {
						"type": "plain_text",
						"text": "Wallet ID (comma separated)"
					},
					"hint": {
						"type": "plain_text",
						"text": "The wallet(s) that the promotions should be created for"
					}
				},
				{
					"type": "input",
					"block_id": "grants",
					"element": {
						"type": "plain_text_input",
						"action_id": "grants",
						"placeholder": {
							"type": "plain_text",
							"text": "1"
						}
					},
					"label": {
						"type": "plain_text",
						"text": "Number of Grants",
						"emoji": true
					}
				},
				{
					"type": "section",
					"block_id": "type",
					"text": {
						"type": "mrkdwn",
						"text": "Type of promotion"
					},
					"accessory": {
						"type": "static_select",
						"action_id": "type",
						"placeholder": {
							"type": "plain_text",
							"text": "Select a type",
							"emoji": true
						},
						"initial_option": {
							"text": {
								"type": "plain_text",
								"text": "Ads",
								"emoji": true
							},
							"value": "ads"
						},
						"options": [
							{
								"text": {
									"type": "plain_text",
									"text": "Ads",
									"emoji": true
								},
								"value": "ads"
							},
							{
								"text": {
									"type": "plain_text",
									"text": "UGP (Universal Grant Protocol)",
									"emoji": true
								},
								"value": "ugp"
							}
						]
					}
				},
				{
					"type": "section",
					"block_id": "platform",
					"text": {
						"type": "mrkdwn",
						"text": "Platform filter"
					},
					"accessory": {
						"type": "static_select",
						"action_id": "platform",
						"placeholder": {
							"type": "plain_text",
							"text": "Select a platform",
							"emoji": true
						},
						"initial_option": {
							"text": {
								"type": "plain_text",
								"text": "Desktop",
								"emoji": true
							},
							"value": "desktop"
						},
						"options": [
							{
								"text": {
									"type": "plain_text",
									"text": "Desktop",
									"emoji": true
								},
								"value": "desktop"
							},
							{
								"text": {
									"type": "plain_text",
									"text": "OSX",
									"emoji": true
								},
								"value": "osx"
							},
							{
								"text": {
									"type": "plain_text",
									"text": "Android",
									"emoji": true
								},
								"value": "android"
							},
							{
								"text": {
									"type": "plain_text",
									"text": "iOS",
									"emoji": true
								},
								"value": "ios"
							},
							{
								"text": {
									"type": "plain_text",
									"text": "Linux",
									"emoji": true
								},
								"value": "linux"
							},
							{
								"text": {
									"type": "plain_text",
									"text": "Windows",
									"emoji": true
								},
								"value": "windows"
							}
						]
					}
				},
				{
					"type": "input",
					"block_id": "amount",
					"optional": true,
					"element": {
						"action_id": "amount",
						"type": "plain_text_input",
						"placeholder": {
							"type": "plain_text",
							"text": "10.000000000000000000"
						}
					},
					"label": {
						"type": "plain_text",
						"text": "Amount in BAT",
						"emoji": true
					}
				},
				{
					"type": "input",
					"block_id": "bonus",
					"optional": true,
					"element": {
						"action_id": "bonus",
						"type": "plain_text_input",
						"placeholder": {
							"type": "plain_text",
							"text": "0.000000000000000000"
						}
					},
					"label": {
						"type": "plain_text",
						"text": "Bonus in BAT",
						"emoji": true
					}
				},
				{
					"type": "section",
					"block_id": "legacy",
					"text": {
						"type": "mrkdwn",
						"text": "Sets up the claim as if it was claimed before virtual grants existed"
					},
					"accessory": {
						"type": "radio_buttons",
						"action_id": "legacy",
						"initial_option": {
							"text": {
								"type": "plain_text",
								"text": "no"
							},
							"value": "no"
						},
						"options": [
							{
								"text": {
									"type": "plain_text",
									"text": "no"
								},
								"value": "no"
							},
							{
								"text": {
									"type": "plain_text",
									"text": "yes"
								},
								"value": "yes"
							}
						]
					}
				},
				{
					"type": "section",
					"block_id": "expiry_time",
					"text": {
						"type": "mrkdwn",
						"text": "Set an expiry time"
					},
					"accessory": {
						"type": "datepicker",
						"action_id": "expiry_time",
						"placeholder": {
							"type": "plain_text",
							"text": "Select an expiration date",
							"emoji": true
						}
					}
				}
			]
		}
	}`, triggerID, workflowID.String()))
	var body interface{}
	err := json.Unmarshal([]byte(str), &body)
	if err != nil {
		panic(err)
	}
	str, err = json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return str
}

// Client abstracts over the underlying client
type Client interface {
	PromotionCreateOpenModal(
		ctx context.Context,
		triggerID string,
		workflowID uuid.UUID,
	) (*interface{}, error)
	PromotionUpdateOpenModal(
		ctx context.Context,
		view View,
	) (*interface{}, error)
}

// HTTPClient wraps http.Client for interacting with the ledger server
type HTTPClient struct {
	clients.SimpleHTTPClient
}

// New returns a new HTTPClient, retrieving the base URL from the environment
func New() (*HTTPClient, error) {
	client, err := clients.New("SLACK_SERVER", "SLACK_ACCESS_TOKEN")
	if err != nil {
		return nil, err
	}
	return &HTTPClient{*client}, err
}

// PromotionCreateOpenModal fetches the rate of a currency to BAT
func (c *HTTPClient) PromotionCreateOpenModal(ctx context.Context, triggerID string, workflowID uuid.UUID) (*interface{}, error) {
	requestBody := createPromotionCreateOpenModal(triggerID, workflowID)
	req, err := c.NewRequest(ctx, "POST", "/api/views.open", requestBody)
	if err != nil {
		return nil, err
	}

	var body interface{}
	resp, err := c.Do(ctx, req, body)

	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return nil, nil
		}
		return nil, err
	}

	return &body, err
}

// PromotionUpdateOpenModal updates open modal
func (c *HTTPClient) PromotionUpdateOpenModal(
	ctx context.Context,
	view View,
) (*interface{}, error) {
	hash := view.Hash
	externalID := view.ExternalID
	viewJSON, err := json.Marshal(view)
	if err != nil {
		return nil, err
	}
	req, err := c.NewRequest(ctx, "POST", "/api/views.update", &ViewUpdateRequest{
		View:       string(viewJSON),
		ExternalID: externalID,
		Hash:       hash,
	})

	if err != nil {
		return nil, err
	}

	var body interface{}
	resp, err := c.Do(ctx, req, body)

	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return nil, nil
		}
		return nil, err
	}
	return &body, err
}
