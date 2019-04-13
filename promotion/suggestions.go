package promotion

import (
	"encoding/base64"
	"encoding/json"
)

// Suggestion encapsulates information from the user about where /how they want to contribute
type Suggestion struct {
	Type    string `json:"type" valid:"in(auto-contribute|oneoff-tip|recurring-tip)"`
	Channel string `json:"channel"`
}

// Base64Decode unmarshalls the suggestion from a string.
func (s *Suggestion) Base64Decode(text string) error {
	var bytes []byte
	bytes, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return err
	}

	err = json.Unmarshal(bytes, s)
	return err
}
