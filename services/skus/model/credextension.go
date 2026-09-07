package model

import (
	"time"
)

const (
	// ExtensionMaxLimitCeiling Hard sanity ceiling enforced by DB CHECK on max_active_batches_tlv2_creds.
	ExtensionMaxLimitCeiling = 1000

	ExtensionCodeMalformedBody       = "malformed_body"
	ExtensionCodeOrderNotFound       = "order_not_found"
	ExtensionCodeOrderNotPaid        = "order_not_paid"
	ExtensionCodeUnsupportedCredType = "unsupported_cred_type"
	ExtensionCodeConflict            = "extension_conflict"

	// Deprecated: will be replaced by ExtensionCodeInvalidLimitX
	ExtensionCodeInvalidLimit = "extension_invalid_limit"

	ExtensionCodeInvalidLimitX = "invalid_limit"
	ExtensionCodeRateLimited   = "rate_limited"
	ExtensionCodeMaxPerItem    = "max_per_item"
	ExtensionCodeNotAtLimit    = "not_at_limit"
	ExtensionNotSupported      = "extension_not_supported"

	ErrNoExtensionPolicy    Error = "model: extension: no policy"
	ErrExtensionRateLimited Error = "model: extension: rate limited"
	ErrExtensionMaxPerItem  Error = "model: extension: max per item reached"
	ErrExtensionNotAtLimit  Error = "model: extension: not at limit"
	ErrExtensionStateItem   Error = "model: cred extension item nil"
)

type CredExtensionPolicy struct {
	SlotsPerGrant      int
	MinIntervalSeconds int
	MaxPerItem         int
}

type CredExtensionPolicies map[string]CredExtensionPolicy

func NewPoliciesBySKUVnt() CredExtensionPolicies {
	origin := CredExtensionPolicy{
		SlotsPerGrant:      3,
		MinIntervalSeconds: 30 * 24 * 60 * 60, // 30 Days.
		MaxPerItem:         5,
	}

	return CredExtensionPolicies{
		"brave-origin-premium-perpetual-license": origin,
	}
}

func (s CredExtensionPolicies) GetPolicy(item *OrderItem) (CredExtensionPolicy, error) {
	if !item.IsCredTLV2() {
		return CredExtensionPolicy{}, ErrUnsupportedCredType
	}

	p, ok := s[item.SKUVnt]
	if !ok {
		return CredExtensionPolicy{}, ErrNoExtensionPolicy
	}

	return p, nil
}

type ExtensionState struct {
	Item          *OrderItem
	ActiveBatches int
	Limit         int
}

func (s ExtensionState) AtLimit() bool {
	return s.ActiveBatches >= s.Limit
}

type CredExtension struct {
	pol CredExtensionPolicy
	st  ExtensionState
	now time.Time
}

func NewCredExtension(pol CredExtensionPolicy, st ExtensionState, now time.Time) CredExtension {
	return CredExtension{pol: pol, st: st, now: now}
}

type ExtensionGrant struct {
	NextMaxActiveBatchLimit int
	NextNumSelfExt          int
}

func (x CredExtension) AtLimit() bool {
	return x.st.Item != nil && x.st.AtLimit()
}

func (x CredExtension) CanExtend() bool {
	_, err := x.Grant()

	return err == nil
}

func (x CredExtension) Grant() (ExtensionGrant, error) {
	if x.st.Item == nil {
		return ExtensionGrant{}, ErrExtensionStateItem
	}

	switch {
	case x.st.Item.NumSelfExtensions >= x.pol.MaxPerItem:
		return ExtensionGrant{}, ErrExtensionMaxPerItem

	case x.rateLimited():
		return ExtensionGrant{}, ErrExtensionRateLimited

	case !x.st.AtLimit():
		return ExtensionGrant{}, ErrExtensionNotAtLimit

	default:
		return ExtensionGrant{
			NextMaxActiveBatchLimit: x.st.Limit + x.pol.SlotsPerGrant,
			NextNumSelfExt:          x.st.Item.NumSelfExtensions + 1,
		}, nil
	}
}

func (x CredExtension) rateLimited() bool {
	if x.st.Item.LastSelfExtensionAt == nil {
		return false
	}

	nextAllowed := x.st.Item.LastSelfExtensionAt.Add(time.Duration(x.pol.MinIntervalSeconds) * time.Second)

	return x.now.Before(nextAllowed)
}
