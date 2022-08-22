package settlement

import (
	"encoding/json"
	"io/ioutil"

	"github.com/brave-intl/bat-go/libs/custodian"
)

// ReadFiles reads a series of files
func ReadFiles(filesPaths []string) (*[]custodian.Transaction, error) {
	var allPayouts []custodian.Transaction
	for _, filePath := range filesPaths {
		var transactionList []custodian.Transaction
		bytes, err := ioutil.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(bytes, &transactionList)
		if err != nil {
			return nil, err
		}
		allPayouts = append(allPayouts, transactionList...)
	}
	return &allPayouts, nil
}
