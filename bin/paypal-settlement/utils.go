package main

import (
	"encoding/json"
	"io/ioutil"
	"strings"

	"github.com/brave-intl/bat-go/settlement"
)

// ReadFiles reads a series of files
func ReadFiles(input string) (*[]settlement.Transaction, error) {
	var allPayouts []settlement.Transaction
	files := strings.Split(input, ",")
	for _, file := range files {
		var batPayouts []settlement.Transaction
		bytes, err := ioutil.ReadFile(file)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(bytes, &batPayouts)
		if err != nil {
			return nil, err
		}
		allPayouts = append(allPayouts, batPayouts...)
	}
	return &allPayouts, nil
}
