package formatters

import (
	"encoding/json"

	log "github.com/sirupsen/logrus"
)

// CliFormatter is a logrus formatter which prints non-debug messages without tags and pretty-prints json
type CliFormatter struct {
	log.TextFormatter
}

// Format the log entry
func (f *CliFormatter) Format(entry *log.Entry) ([]byte, error) {
	jmsg := json.RawMessage{}
	err := json.Unmarshal([]byte(entry.Message), &jmsg)
	if err == nil {
		tmp, err := json.MarshalIndent(jmsg, "", "    ")
		if err == nil {
			entry.Message = string(tmp)
		}
	}

	if entry.Level != log.DebugLevel {
		return []byte(entry.Message), nil
	}
	return f.TextFormatter.Format(entry)
}
