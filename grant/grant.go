package grant

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/datastore"
	"github.com/brave-intl/bat-go/utils"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	"github.com/garyburd/redigo/redis"
	raven "github.com/getsentry/raven-go"
	"github.com/pressly/lg"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/ed25519"
	jose "gopkg.in/square/go-jose.v2"
)

const (
	lowerTxLimit        = 5
	upperTxLimit        = 120
	ninetyDaysInSeconds = 60 * 60 * 24 * 90
	productionEnv       = "production"
)

var (
	// SettlementDestination is the address of the settlement wallet
	SettlementDestination = os.Getenv("BAT_SETTLEMENT_ADDRESS")
	// GrantSignatorPublicKeyHex is the hex encoded public key of the keypair used to sign grants
	GrantSignatorPublicKeyHex    = os.Getenv("GRANT_SIGNATOR_PUBLIC_KEY")
	grantWalletPublicKeyHex      = os.Getenv("GRANT_WALLET_PUBLIC_KEY")
	grantWalletPrivateKeyHex     = os.Getenv("GRANT_WALLET_PRIVATE_KEY")
	grantWalletCardID            = os.Getenv("GRANT_WALLET_CARD_ID")
	grantPublicKey               ed25519.PublicKey
	grantWallet                  *uphold.Wallet
	refreshBalance               = true  // for testing we can disable balance refresh
	testSubmit                   = true  // for testing we can disable testing tx submit
	registerGrantInstrumentation = true  // for testing we can disable grant claim / redeem instrumentation registration
	safeMode                     = false // if set true disables grant redemption
	claimedGrantsCounter         = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "claimed_grants_total",
			Help: "Number of grants claimed since start.",
		},
		[]string{},
	)
	redeemedGrantsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redeemed_grants_total",
			Help: "Number of grants redeemed since start.",
		},
		[]string{"promotionId"},
	)
)

type grantService struct {
	pool                      *redis.Pool
	outstandingGrantCountDesc *prometheus.Desc
	completedGrantCountDesc   *prometheus.Desc
}

// Describe returns all descriptions of the collector.
// We implement this and the Collect function to fulfill the prometheus.Collector interface
func (gs *grantService) Describe(ch chan<- *prometheus.Desc) {
	ch <- gs.outstandingGrantCountDesc
	ch <- gs.completedGrantCountDesc
}

// Collect returns the current state of all metrics of the collector.
// We implement this and the Describe function to fulfill the prometheus.Collector interface
func (gs *grantService) Collect(ch chan<- prometheus.Metric) {
	conn := gs.pool.Get()
	defer utils.PanicCloser(conn)

	kv := datastore.GetRedisKv(&conn)
	ogCount, err := kv.Count("grant:*")
	if err != nil {
		raven.CaptureError(err, map[string]string{})
		return
	}
	ch <- prometheus.MustNewConstMetric(
		gs.outstandingGrantCountDesc,
		prometheus.GaugeValue,
		float64(ogCount),
	)
	promotions, err := kv.Keys("promotion:*:grants")
	if err != nil {
		raven.CaptureError(err, map[string]string{})
		return
	}
	for i := 0; i < len(promotions); i++ {
		promotionSet := datastore.GetRedisSet(&conn, promotions[i])
		promotionID := strings.TrimSuffix(strings.TrimPrefix(promotions[i], "promotion:"), ":grants")
		completedCount, err := promotionSet.Cardinality()
		if err != nil {
			raven.CaptureError(err, map[string]string{})
			return
		}

		ch <- prometheus.MustNewConstMetric(
			gs.completedGrantCountDesc,
			prometheus.GaugeValue,
			float64(completedCount),
			promotionID,
		)
	}
}

// InitGrantService initializes the grant service
func InitGrantService(pool *redis.Pool) error {
	var err error
	grantPublicKey, err = hex.DecodeString(GrantSignatorPublicKeyHex)
	if err != nil {
		return err
	}

	if os.Getenv("ENV") == productionEnv && !refreshBalance {
		return errors.New("refreshBalance must be true in production")
	}
	if os.Getenv("ENV") == productionEnv && !testSubmit {
		return errors.New("testSubmit must be true in production")
	}

	if len(grantWalletCardID) > 0 {
		var info wallet.Info
		info.Provider = "uphold"
		info.ProviderID = grantWalletCardID
		{
			tmp := altcurrency.BAT
			info.AltCurrency = &tmp
		}

		var pubKey httpsignature.Ed25519PubKey
		var privKey ed25519.PrivateKey
		var err error

		pubKey, err = hex.DecodeString(grantWalletPublicKeyHex)
		if err != nil {
			return err
		}
		privKey, err = hex.DecodeString(grantWalletPrivateKeyHex)
		if err != nil {
			return err
		}

		grantWallet, err = uphold.New(info, privKey, pubKey)
		if err != nil {
			return err
		}
	} else if os.Getenv("ENV") == productionEnv {
		return errors.New("GRANT_WALLET_CARD_ID must be set in production")
	}

	if registerGrantInstrumentation {
		if pool != nil {
			gs := &grantService{
				pool: pool,
				outstandingGrantCountDesc: prometheus.NewDesc(
					"outstanding_grants_total",
					"Outstanding grants that have been claimed and have not expired.",
					[]string{},
					prometheus.Labels{},
				),
				completedGrantCountDesc: prometheus.NewDesc(
					"completed_grants_total",
					"Completed grants that have been redeemed.",
					[]string{"promotionId"},
					prometheus.Labels{},
				),
			}
			prometheus.MustRegister(gs)
		}

		prometheus.MustRegister(claimedGrantsCounter)
		prometheus.MustRegister(redeemedGrantsCounter)
	}

	return nil
}

// Grant - a "check" good for the amount inscribed, redeemable between maturityTime and expiryTime
type Grant struct {
	AltCurrency       *altcurrency.AltCurrency `json:"altcurrency"`
	GrantID           uuid.UUID                `json:"grantId"`
	Probi             decimal.Decimal          `json:"probi"`
	PromotionID       uuid.UUID                `json:"promotionId"`
	MaturityTimestamp int64                    `json:"maturityTime"`
	ExpiryTimestamp   int64                    `json:"expiryTime"`
}

// ByProbi implements sort.Interface for []Grant based on the Probi field.
type ByProbi []Grant

func (a ByProbi) Len() int           { return len(a) }
func (a ByProbi) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByProbi) Less(i, j int) bool { return a[i].Probi.LessThan(a[j].Probi) }

// CreateGrants creates the specified number of grants and returns them in compact JWS serialization
func CreateGrants(
	signer jose.Signer,
	promotionUUID uuid.UUID,
	grantCount uint,
	altCurrency altcurrency.AltCurrency,
	value float64,
	maturityDate time.Time,
	expiryDate time.Time,
) []string {
	grants := make([]string, 0, grantCount)
	for i := 0; i < int(grantCount); i++ {
		var grant Grant
		grant.AltCurrency = &altCurrency
		grant.GrantID = uuid.NewV4()
		grant.Probi = altCurrency.ToProbi(decimal.NewFromFloat(value))
		grant.PromotionID = promotionUUID
		grant.MaturityTimestamp = maturityDate.Unix()
		grant.ExpiryTimestamp = expiryDate.Unix()

		serializedGrant, err := json.Marshal(grant)
		if err != nil {
			log.Fatalln(err)
		}
		jws, err := signer.Sign(serializedGrant)
		if err != nil {
			log.Fatalln(err)
		}
		serializedJWS, err := jws.CompactSerialize()
		if err != nil {
			log.Fatalln(err)
		}
		grants = append(grants, serializedJWS)
	}
	return grants
}

// FromCompactJWS parses a Grant object from one stored using compact JWS serialization.
// It returns a pointer to the parsed Grant object if it is valid and signed by the grantPublicKey.
// Otherwise an error is returned.
func FromCompactJWS(s string) (*Grant, error) {
	jws, err := jose.ParseSigned(s)
	if err != nil {
		return nil, err
	}
	for _, sig := range jws.Signatures {
		if sig.Header.Algorithm != "EdDSA" {
			return nil, errors.New("Error unsupported JWS algorithm")
		}
	}
	jwk := jose.JSONWebKey{Key: grantPublicKey}
	grantBytes, err := jws.Verify(jwk)
	if err != nil {
		return nil, err
	}

	var grant Grant
	err = json.Unmarshal(grantBytes, &grant)
	if err != nil {
		return nil, err
	}
	return &grant, nil
}

// ClaimGrantRequest is a request to claim a grant
type ClaimGrantRequest struct {
	WalletInfo wallet.Info `json:"wallet" valid:"required"`
}

// Claim registers a claim on behalf of a user wallet to a particular Grant.
// Registered claims are enforced by RedeemGrantsRequest.Verify.
func (req *ClaimGrantRequest) Claim(ctx context.Context, grantID string) error {
	log := lg.Log(ctx)

	kvDatastore, err := datastore.GetKvDatastore(ctx)
	if err != nil {
		return err
	}
	defer utils.PanicCloser(kvDatastore)

	_, err = kvDatastore.Set("grant:"+grantID+":claim", req.WalletInfo.ProviderID, ninetyDaysInSeconds, false)
	if err != nil {
		log.Error("Attempt to claim previously claimed grant!")
		return errors.New("An existing claim to the grant already exists")
	}
	claimedGrantsCounter.With(prometheus.Labels{}).Inc()

	return nil
}

// RedeemGrantsRequest a request to redeem the included grants for the wallet whose information
// is included in order to fulfill the included transaction
type RedeemGrantsRequest struct {
	Grants      []string    `json:"grants" valid:"compactjws"`
	WalletInfo  wallet.Info `json:"wallet" valid:"required"`
	Transaction string      `json:"transaction" valid:"base64"`
}

// DecodeGrants decodes grants and sends back a new array
func DecodeGrants(grants []string) ([]Grant, error) {
	// 1. Check grant signatures and decode
	decoded := make([]Grant, 0, len(grants))
	for _, grantJWS := range grants {
		grant, err := FromCompactJWS(grantJWS)
		if err != nil {
			return nil, err
		}
		decoded = append(decoded, *grant)
	}
	return decoded, nil
}

// VerifyAndConsume one or more grants to fulfill the included transaction for wallet
// Note that this is destructive, on success consumes grants.
// Further calls to Verify with the same request will fail as the grants are consumed.
//
// 1. Check grant signatures and decode
//
// 2. Check transaction signature and decode, enforce minimum transaction amount
//
// 3. Sort decoded grants, largest probi to smallest
//
// 4. Sum from largest to smallest until value is gt transaction amount
//
// 5. Fail if there are leftover grants
//
// 6. Iterate through grants and check that:
//
// a) this wallet has not yet redeemed a grant for the given promotionId
//
// b) this grant has not yet been redeemed by any wallet
//
// Returns transaction info for grant fufillment
func (req *RedeemGrantsRequest) VerifyAndConsume(ctx context.Context) (*wallet.TransactionInfo, error) {
	log := lg.Log(ctx)
	// 1. Check grant signatures and decode
	grants, err := DecodeGrants(req.Grants)
	if err != nil {
		return nil, err
	}
	// 2. Check transaction signature and decode, enforce transaction checks
	userWallet, err := provider.GetWallet(req.WalletInfo)
	if err != nil {
		return nil, err
	}
	// this ensures we have a valid wallet if refreshBalance == true
	balance, err := userWallet.GetBalance(refreshBalance)
	if err != nil {
		return nil, err
	}
	// NOTE for uphold provider we currently check against user provided publicKey
	//      thus this check does not protect us from a valid fake signature
	txInfo, err := userWallet.VerifyTransaction(req.Transaction)
	if err != nil {
		return nil, err
	}
	if *txInfo.AltCurrency != altcurrency.BAT {
		return nil, errors.New("only grants submitted with BAT transactions are supported")
	}
	if txInfo.Probi.LessThan(decimal.Zero) {
		return nil, errors.New("included transaction cannot be for negative BAT")
	}
	if txInfo.Probi.LessThan(altcurrency.BAT.ToProbi(decimal.New(lowerTxLimit, 0))) {
		return nil, fmt.Errorf("included transaction must be for a minimum of %d BAT", lowerTxLimit)
	}
	if txInfo.Probi.GreaterThan(altcurrency.BAT.ToProbi(decimal.New(upperTxLimit, 0))) {
		return nil, fmt.Errorf("included transaction must be for a maxiumum of %d BAT", upperTxLimit)
	}
	if txInfo.Destination != SettlementDestination {
		return nil, errors.New("included transactions must have settlement as their destination")
	}

	// 3. Sort decoded grants, largest probi to smallest
	sort.Sort(sort.Reverse(ByProbi(grants)))

	// 4. Sum from largest to smallest until value is gt transaction amount
	sumProbi := decimal.New(0, 1)
	for _, grant := range grants {
		if sumProbi.GreaterThanOrEqual(txInfo.Probi) {
			// 5. Fail if there are leftover grants
			return nil, errors.New("More grants included than are needed to fulfill included transaction")
		}
		if *grant.AltCurrency != altcurrency.BAT {
			return nil, errors.New("All grants must be in BAT")
		}
		sumProbi = sumProbi.Add(grant.Probi)
	}

	if txInfo.Probi.GreaterThan(balance.SpendableProbi.Add(sumProbi)) {
		return nil, errors.New("wallet does not have enough funds to cover transaction")
	}

	// should be reasonable since we limit the redeem endpoint to a maximum of 1 simultaneous in-flight request
	ugpBalance, err := grantWallet.GetBalance(refreshBalance)
	if err != nil {
		return nil, err
	}

	if sumProbi.GreaterThan(ugpBalance.SpendableProbi) {
		safeMode = true
		return nil, errors.New("ugp wallet lacks enough funds to fulfill grants")
	}

	var submitID string
	if testSubmit {
		var submitInfo *wallet.TransactionInfo
		// TODO remove this once we can retrieve publicKey info from uphold
		// NOTE We check the signature on the included transaction by submitting it but not confirming it
		submitInfo, err = userWallet.SubmitTransaction(req.Transaction, false)
		if err != nil {
			if wallet.IsInvalidSignature(err) {
				return nil, errors.New("the included transaction was signed with the wrong publicKey")
			} else if !wallet.IsInsufficientBalance(err) {
				return nil, errors.New("error while test submitting the included transaction: " + err.Error())
			}
		}
		submitID = submitInfo.ID
	}

	kvDatastore, err := datastore.GetKvDatastore(ctx)
	if err != nil {
		return nil, err
	}
	defer utils.PanicCloser(kvDatastore)
	// 6. Iterate through grants and check that:
	for _, grant := range grants {
		claimedID, err := kvDatastore.Get("grant:" + grant.GrantID.String() + ":claim")
		if err != nil {
			errMsg := "Attempt to redeem grant without previous claim or with expired claim"
			log.Error(errMsg)
			log.Error(grant.GrantID.String())
			return nil, errors.New(errMsg)
		}
		// the grant was previously claimed for this wallet
		if req.WalletInfo.ProviderID != claimedID {
			log.Error("Attempt to redeem previously claimed by another wallet!!!")
			return nil, errors.New("Grant claim does not match provided wallet")
		}

		// the grant is mature
		if time.Now().Unix() < grant.MaturityTimestamp {
			return nil, errors.New("Grant is not yet redeemable as it is immature")
		}

		// the grant is not expired
		if time.Now().Unix() > grant.ExpiryTimestamp {
			return nil, errors.New("Grant is expired")
		}

		redeemedGrants, err := datastore.GetSetDatastore(ctx, "promotion:"+grant.PromotionID.String()+":grants")
		if err != nil {
			return nil, err
		}
		defer utils.PanicCloser(redeemedGrants)
		redeemedWallets, err := datastore.GetSetDatastore(ctx, "promotion:"+grant.PromotionID.String()+":wallets")
		if err != nil {
			return nil, err
		}
		defer utils.PanicCloser(redeemedWallets)

		result, err := redeemedGrants.Add(grant.GrantID.String())
		if err != nil {
			return nil, err
		}
		if !result {
			// a) this wallet has not yet redeemed a grant for the given promotionId
			log.Error("Attempt to redeem previously redeemed grant!!!")
			return nil, fmt.Errorf("grant %s has already been redeemed", grant.GrantID)
		}

		result, err = redeemedWallets.Add(req.WalletInfo.ProviderID)
		if err != nil {
			return nil, err
		}
		if !result {
			// b) this grant has not yet been redeemed by any wallet
			log.Error("Attempt to redeem multiple grants from one promotion by the same wallet!!!")
			return nil, fmt.Errorf("Wallet %s has already redeemed a grant from this promotion", req.WalletInfo.ProviderID)
		}

		redeemedGrantsCounter.With(prometheus.Labels{"promotionId": grant.PromotionID.String()}).Inc()
	}

	var redeemTxInfo wallet.TransactionInfo
	{
		tmp := altcurrency.BAT
		redeemTxInfo.AltCurrency = &tmp
	}
	redeemTxInfo.Probi = sumProbi
	redeemTxInfo.Destination = req.WalletInfo.ProviderID
	redeemTxInfo.ID = submitID
	return &redeemTxInfo, nil
}

// GetPromotionGrantsDatastore creates a datastore for a grant claim
func GetPromotionGrantsDatastore(ctx context.Context, promotionID uuid.UUID) (datastore.SetLikeDatastore, error) {
	return datastore.GetSetDatastore(ctx, "promotion:"+promotionID.String()+":grants")
}

// GetRedeemedIDs returns the redemption status for given grants under a provider id
func GetRedeemedIDs(ctx context.Context, Grants []string) ([]string, error) {

	// 1. Check grant signatures and decode
	grants, err := DecodeGrants(Grants)
	if err != nil {
		return nil, err
	}
	grantCount := len(grants)
	results := make([]string, 0, grantCount)

	for _, grant := range grants {
		grantID := grant.GrantID.String()
		redeemedGrants, err := GetPromotionGrantsDatastore(ctx, grant.PromotionID)
		if err != nil {
			return nil, err
		}
		defer utils.PanicCloser(redeemedGrants)

		grantRedeemed, err := redeemedGrants.Contains(grantID)
		if err != nil {
			return nil, err
		}
		if grantRedeemed {
			results = append(results, grantID)
		}
	}

	return results, nil
}

// Redeem the grants in the included response
func (req *RedeemGrantsRequest) Redeem(ctx context.Context) (*wallet.TransactionInfo, error) {
	log := lg.Log(ctx)

	if safeMode {
		return nil, errors.New("Grant redemption has been disabled due to fail-safe condition")
	}

	grantFulfillmentInfo, err := req.VerifyAndConsume(ctx)
	if err != nil {
		return nil, err
	}

	submitID := grantFulfillmentInfo.ID

	userWallet, err := provider.GetWallet(req.WalletInfo)
	if err != nil {
		safeMode = true
		log.Errorf("Could not get wallet %s from info after successful VerifyAndConsume", req.WalletInfo.ProviderID)
		raven.CaptureMessage("Could not get wallet after successful VerifyAndConsume", map[string]string{"providerID": req.WalletInfo.ProviderID})
		return nil, err
	}

	// fund user wallet with probi from grants
	_, err = grantWallet.Transfer(*grantFulfillmentInfo.AltCurrency, grantFulfillmentInfo.Probi, grantFulfillmentInfo.Destination)
	if err != nil {
		safeMode = true
		log.Errorf("Could not fund wallet %s after successful VerifyAndConsume", req.WalletInfo.ProviderID)
		raven.CaptureMessage("Could not fund wallet after successful VerifyAndConsume", map[string]string{"providerID": req.WalletInfo.ProviderID})
		return nil, err
	}

	// confirm settlement transaction previously sent to wallet provider
	var settlementInfo *wallet.TransactionInfo
	for tries := 5; tries >= 0; tries-- {
		if tries == 0 {
			safeMode = true
			log.Errorf("Could not submit settlement txn for wallet %s after successful VerifyAndConsume", req.WalletInfo.ProviderID)
			raven.CaptureMessage("Could not submit settlement txn after successful VerifyAndConsume", map[string]string{"providerID": req.WalletInfo.ProviderID})
			return nil, err
		}
		// NOTE VerifyAndConsume (by way of VerifyTransaction) guards against transactions that seek to exploit parser differences
		// such as including additional fields that are not understood by this wallet provider implementation but may
		// be understood by the upstream wallet provider.
		settlementInfo, err = userWallet.ConfirmTransaction(submitID)
		if err == nil {
			break
		}
	}

	return settlementInfo, nil
}
