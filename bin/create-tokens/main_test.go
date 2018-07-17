// +build integration

package main

import (
	"errors"
	"os"
	"testing"
)

func init() {
	os.Setenv("ENV", "production")
}

func TestCreateTokens(t *testing.T) {
	err := subTestCreateTokens("n")
	if err == nil {
		t.Error(err)
	}
	err = subTestCreateTokens("y")
	if err != nil {
		t.Error(err)
	}
}

func subTestCreateTokens(input string) error {
	context, err := BuildContext()
	if err != nil {
		return err
	}
	signer, err := CreateSigner()
	if err != nil {
		return err
	}
	// not using ReceiveInput so we don't have to pass context
	continues, err := CheckInput(input)
	if err != nil {
		return err
	}
	if continues != true {
		return errors.New("rejected")
	}

	err = CreateTokens(signer, context)
	if err != nil {
		return err
	}
	return nil
}
