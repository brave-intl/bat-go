package nitro

import (
	"fmt"
	"net/http"

	"github.com/brave-intl/bat-go/libs/logging"
)

// EnclaveHealthCheck - status check handler for nitro enclave service
func EnclaveHealthCheck(w http.ResponseWriter, r *http.Request) {
	logging.Logger(r.Context(), "health-check").Debug().Msg("in health-check handler")
	fmt.Fprintf(w, "OK\n")
}
