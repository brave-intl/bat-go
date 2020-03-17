package requestutils

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/brave-intl/bat-go/utils/closers"
)

var payloadLimit10MB = int64(1024 * 1024 * 10)

// ReadWithLimit reads an io reader with a limit and closes
func ReadWithLimit(body io.Reader, limit int64) ([]byte, error) {
	defer closers.Panic(body.(io.Closer))
	return ioutil.ReadAll(io.LimitReader(body, limit))
}

// Read an io reader
func Read(body io.Reader) ([]byte, error) {
	jsonString, err := ReadWithLimit(body, payloadLimit10MB)
	if err != nil {
		return nil, fmt.Errorf("error reading body: %w", err)
	}
	return jsonString, nil
}

// ReadJSON reads a request body according to an interface and limits the size to 10MB
func ReadJSON(body io.Reader, intr interface{}) error {
	jsonString, err := Read(body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(jsonString, &intr)
	if err != nil {
		return fmt.Errorf("error unmarshalling body: %w", err)
	}
	return nil
}
