package settlement

import (
	"encoding/json"
	"io/ioutil"
)

// ReadFiles reads a series of files
func ReadFiles(filesPaths []string) (*[]Transaction, error) {
	var allPayouts []Transaction
	for _, filePath := range filesPaths {
		var transactionList []Transaction
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
