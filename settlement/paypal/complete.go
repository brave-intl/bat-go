package paypal

import (
	"errors"
	"fmt"
)

// CompleteSettlement marks the settlement file as complete
func CompleteSettlement(args CompleteArgs) error {
	fmt.Println("RUNNING: complete")
	if args.In == "" {
		return errors.New("the 'in' flag must be set")
	}
	if args.Out == "./paypal-settlement" {
		// use a file with extension if none is passed
		args.Out = "./paypal-settlement-complete.json"
	}
	payouts, err := ReadFiles(args.In)
	if err != nil {
		return err
	}
	for i, payout := range *payouts {
		payout.Status = "complete"
		(*payouts)[i] = payout
	}
	err = WriteTransactions(args.Out, payouts)
	if err != nil {
		return err
	}
	return nil
}
