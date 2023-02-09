package ratios

import (
	"context"
	"strings"

	"github.com/brave-intl/bat-go/libs/clients/coingecko"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/shopspring/decimal"
)

var (
	special = map[string]string{
		"dai":   "dai",
		"imx":   "immutable-x",
		"abat":  "aave-bat",
		"abusd": "aave-busd",
		"adai":  "aave-dai",
		"adx":   "adex",
		"aenj":  "aave-enj",
		"aknc":  "aave-knc",
		"alink": "aave-link",
		"amana": "aave-mana",
		"amkr":  "aave-mkr",
		"aren":  "aave-ren",
		"art":   "maecenas",
		"asnx":  "aave-snx",
		"ast":   "airswap",
		"asusd": "aave-susd",
		"atusd": "aave-tusd",
		"ausdc": "aave-usdc",
		"ausdt": "aave-usdt",
		"awbtc": "aave-wbtc",
		"ayfi":  "aave-yfi",
		"azrx":  "aave-zrx",
		"bat":   "basic-attention-token",
		"bcp":   "bitcashpay-old",
		"bee":   "bee-coin",
		"bnb":   "binancecoin",
		"boa":   "bosagora",
		"bob":   "bobs_repair",
		"booty": "candybooty",
		"box":   "box-token",
		"btu":   "btu-protocol",
		"can":   "canyacoin",
		"cat":   "bitclave",
		"comp":  "compound-governance-token",
		"cor":   "coreto",
		"cvp":   "concentrated-voting-power",
		"cwbtc": "compound-wrapped-btc",
		"data":  "streamr",
		"dg":    "degate",
		"drc":   "dracula-token",
		"dream": "dreamteam",
		"drt":   "domraider",
		"duck":  "dlp-duck-token",
		"edg":   "edgeless",
		"ert":   "eristica",
		"eth":   "ethereum",
		"flx":   "reflexer-ungovernance-token",
		"frm":   "ferrum-network",
		"ftm":   "fantom",
		"game":  "gamecredits",
		"gen":   "daostack",
		"get":   "get-token",
		"gold":  "dragonereum-gold",
		"grt":   "the-graph",
		"gtc":   "gitcoin",
		"hex":   "hex",
		"hgt":   "hellogold",
		"hot":   "holotoken",
		"ieth":  "iethereum",
		"imp":   "ether-kingdoms-token",
		"inx":   "infinitx",
		"iotx":  "iotex",
		"isla":  "insula",
		"jet":   "jetcoin",
		"key":   "selfkey",
		"land":  "landshare",
		"like":  "likecoin",
		"link":  "chainlink",
		"lnk":   "chainlink",
		"luna":  "wrapped-terra",
		"mana":  "decentraland",
		"mdx":   "mandala-exchange-token",
		"mm":    "million",
		"mta":   "meta",
		"musd":  "musd",
		"muso":  "mirrored-united-states-oil-fund",
		"nct":   "polyswarm",
		"ndx":   "ndex",
		"oil":   "oiler",
		"one":   "menlo-one",
		"ousd":  "origin-dollar",
		"pla":   "playdapp",
		"plat":  "dash-platinum",
		"play":  "herocoin",
		"pmon":  "polychain-monsters",
		"poly":  "polymath",
		"prt":   "portion",
		"rai":   "rai",
		"rbc":   "rubic",
		"ren":   "republic-protocol",
		"rfr":   "refereum",
		"sand":  "san-diego-coin",
		"sbtc":  "sbtc",
		"sdt":   "stabledoc-token",
		"seth":  "seth",
		"sgtv2": "sharedstake-governance-token",
		"sig":   "signal-token",
		"soul":  "cryptosoul",
		"space": "spacelens",
		"spn":   "sapien",
		"spnd":  "spendcoin",
		"star":  "filestar",
		"steth": "staked-ether",
		"susd":  "stabilize-usd",
		"swt":   "swarm-city",
		"tbtc":  "tbtc",
		"thor":  "thor",
		"time":  "chronobank",
		"top":   "top-network",
		"uni":   "uniswap",
		"usdn":  "neutrino",
		"usdp":  "paxos-standard",
		"usdx":  "usdx",
		"ust":   "wrapped-ust",
		"val":   "sora-validator-token",
		"wings": "wings",
		"xor":   "sora",
		"yld":   "yield",
		"zap":   "zap",
	}
)

func (s *Service) initializeCoingeckoCurrencies(ctx context.Context) (context.Context, error) {
	var (
		idToSymbol            = map[string]string{}
		symbolToID            = map[string]string{}
		contractToID          = map[string]string{}
		supportedVsCurrencies = map[string]bool{}
	)

	tmp, err := s.coingecko.FetchSupportedVsCurrencies(ctx)
	if err != nil {
		return ctx, err
	}
	for _, supported := range *tmp {
		supportedVsCurrencies[supported] = true
	}

	list, err := s.coingecko.FetchCoinList(ctx, true)
	if err != nil {
		return ctx, err
	}

	for _, coin := range *list {
		if specialID, ok := special[coin.Symbol]; ok && specialID != coin.ID {
			continue
		}

		if coin.ID != "link" {
			idToSymbol[coin.ID] = coin.Symbol
		}
		symbolToID[coin.Symbol] = coin.ID

		if len(coin.Platforms.Ethereum) > 0 {
			contractToID[strings.ToLower(coin.Platforms.Ethereum)] = coin.ID
		}
	}

	ctx = context.WithValue(ctx, appctx.CoingeckoIDToSymbolCTXKey, idToSymbol)
	ctx = context.WithValue(ctx, appctx.CoingeckoSymbolToIDCTXKey, symbolToID)
	ctx = context.WithValue(ctx, appctx.CoingeckoContractToIDCTXKey, contractToID)
	ctx = context.WithValue(ctx, appctx.CoingeckoSupportedVsCurrenciesCTXKey, supportedVsCurrencies)

	return ctx, nil
}

func mapSimplePriceResponse(ctx context.Context, resp coingecko.SimplePriceResponse, duration CoingeckoDuration, coinIDs CoingeckoCoinList, vsCurrencies CoingeckoVsCurrencyList) coingecko.SimplePriceResponse {
	out := map[string]map[string]decimal.Decimal{}

	for k, v := range resp {
		innerOut := map[string]decimal.Decimal{}
		for kk, rate := range v {
			var foundCurrency bool
			for _, vv := range []CoingeckoVsCurrency(vsCurrencies) {
				if strings.HasPrefix(kk, string(vv)) {
					foundCurrency = true
					break
				}
			}
			if !foundCurrency {
				// exclude non-matching currencies
				continue
			}

			if strings.HasSuffix(kk, "_24h_change") {
				if duration != CoingeckoDuration("1d") {
					// skip key if duration is mismatched
					continue
				}
				kk = strings.ReplaceAll(kk, "_24h_change", "_timeframe_change")
			}
			innerOut[kk] = rate
		}

		for _, vv := range []CoingeckoCoin(coinIDs) {
			if vv.String() == k {
				out[vv.input] = innerOut
			}
		}
	}

	return coingecko.SimplePriceResponse(out)
}
