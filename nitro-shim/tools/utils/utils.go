package utils

import (
	"fmt"
	"log"
	"os"
)

// NewLogger creates and returns a new logger with the given log prefix.
func NewLogger(name string) *log.Logger {
	return log.New(os.Stderr, name, log.Ldate|log.Ltime|log.LUTC|log.Lshortfile)
}

// ReadConfigFromEnv reads our configuration from environment variables.  If
// any of those variables isn't set, the function returns an error.
func ReadConfigFromEnv(cfg map[string]string) error {
	var exists bool
	var value string

	for envVar := range cfg {
		value, exists = os.LookupEnv(envVar)
		if !exists {
			return fmt.Errorf("environment variable %q not set", envVar)
		}
		cfg[envVar] = value
	}

	return nil
}
