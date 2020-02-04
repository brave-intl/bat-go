package paypal

import (
	"encoding/csv"
	"encoding/json"
	"io/ioutil"
	"os"
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

// WriteCSV writes out a csv
func WriteCSV(out string, rows [][]string) error {
	file, err := os.Create(out)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	writer := csv.NewWriter(file)
	defer writer.Flush()
	return WriteCSVRows(writer, rows)
}

// WriteCSVRows writes rows into a csv writer
func WriteCSVRows(writer *csv.Writer, rows [][]string) error {
	for _, row := range rows {
		err := writer.Write(row)
		if err != nil {
			return err
		}
	}
	return nil
}
