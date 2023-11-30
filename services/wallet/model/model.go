package model

import "errors"

var ErrNoWalletCustodian = errors.New("model: no linked wallet custodian")

type Error string

func (e Error) Error() string {
	return string(e)
}
