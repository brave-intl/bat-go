package subscriptions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/brave-intl/bat-go/payment"
	"github.com/lib/pq"
)

type order struct {
	OrderID        string      `db:"order_id"`
	SubscriptionID string      `db:"subscription_id"`
	CreatedAt      pq.NullTime `db:"created_at"`
	ModifiedAt     pq.NullTime `db:"modified_at"`
	CanceledAt     pq.NullTime `db:"canceled_at"`
}

type items struct {
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
}

type newOrderReq struct {
	Items []items `json:"items"`
	Email string  `json:"email"`
}

type newOrderResp struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"createdAt"`
	Currency   string    `json:"currency"`
	UpdatedAt  time.Time `json:"updatedAt"`
	TotalPrice string    `json:"totalPrice"`
	Location   string    `json:"location"`
	Status     string    `json:"status"`
	Items      []items   `json:"items"`
}

type setOrderTrialDaysReq struct {
	TrialDays int64 `json:"trialDays"`
}

type orderService interface {
	NewOrder(i newOrderReq, existingSub bool) (*order, error)
	VerifyCred(i payment.VerifyCredentialRequestV1, expectedMerchantID string, expectedSKU string) error
	ResetOrderCred(orderID string) (*bool, error)
	CancelOrder(orderID string) error
}

type SkuClient struct {
	BaseURL string
	Token   string
}

type credPresentation struct {
	IssuedAt  string `json:"issued_at"`
	ExpiresAt string `json:"expires_at"`
	Token     string `json:"token"`
}

type verifyCredRes struct {
	Valid bool
}

func InitSKUClient(paymentAPIUrl string, paymentAPIToken string) *SkuClient {
	return &SkuClient{
		BaseURL: paymentAPIUrl,
		Token:   paymentAPIToken,
	}
}

func (om *SkuClient) NewOrder(i newOrderReq, existingSub bool) (*order, error) {
	reqBody, err := json.Marshal(i)
	if err != nil {
		return nil, err
	}

	url := om.BaseURL + "/v1/orders"

	client := http.Client{
		Timeout: time.Second * 2,
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.Body != nil {
		defer res.Body.Close()
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != 201 {
		return nil, fmt.Errorf("Failed in creating order. %s", string(body))
	}

	o1 := newOrderResp{}
	err = json.Unmarshal(body, &o1)
	if err != nil {
		return nil, err
	}

	o := order{
		// SubscriptionID: sub.SubscriptionID,
		OrderID: o1.ID,
	}

	// if !existingSub {
	// 	if sub.ProductID == TALK_PRODUCT_ID_SANDBOX || sub.ProductID == TALK_PRODUCT_ID_PRODUCTION {
	// 		err = om.SetOrderTrialDays(o.OrderID, 30)
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 	}
	// }

	return &o, nil
}

func (om *SkuClient) SetOrderTrialDays(orderID string, days int64) error {
	sotdr := setOrderTrialDaysReq{
		TrialDays: days,
	}

	reqBody, err := json.Marshal(sotdr)
	if err != nil {
		return err
	}

	url := om.BaseURL + "/v1/orders/" + orderID + "/set-trial"

	client := http.Client{
		Timeout: time.Second * 2,
	}

	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+om.Token)
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Failed in setting order trial.")
	}

	return nil
}

func (om *SkuClient) VerifyCred(i payment.VerifyCredentialRequestV1, expectedMerchantID string, expectedSKU string) error {
	// Override the merchantID and SKU with expected values, SKU service will enforce credential matches
	i.MerchantID = expectedMerchantID
	i.SKU = expectedSKU

	reqBody, err := json.Marshal(i)
	if err != nil {
		return err
	}

	url := om.BaseURL + "/v1/credentials/subscription/verifications"

	client := http.Client{
		Timeout: time.Second * 5,
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+om.Token)
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return err
	}

	if res.Body != nil {
		defer res.Body.Close()
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	valid := res.StatusCode == http.StatusOK

	if !valid {
		return fmt.Errorf("failed to verfiy credentials: %s", string(body))
	}

	return nil
}

func (om *SkuClient) ResetOrderCred(orderID string) (*bool, error) {
	url := fmt.Sprintf("%s/v1/orders/%s/credentials", om.BaseURL, orderID)

	client := http.Client{
		Timeout: time.Second * 2,
	}

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+om.Token)
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.Body != nil {
		defer res.Body.Close()
	}

	successful := res.StatusCode == http.StatusOK

	return &successful, nil
}

func (om *SkuClient) CancelOrder(orderID string) error {
	url := fmt.Sprintf("%s/v1/orders/%s", om.BaseURL, orderID)

	client := http.Client{
		Timeout: time.Second * 2,
	}

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+om.Token)

	res, err := client.Do(req)
	if err != nil {
		return err
	}

	if res.Body != nil {
		defer res.Body.Close()
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	successful := res.StatusCode == http.StatusOK

	if !successful {
		return fmt.Errorf("canceling order failed, %s, %s, %d", orderID, string(body), res.StatusCode)
	}

	return nil
}
