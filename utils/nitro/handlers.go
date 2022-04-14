package nitro

import (
	"fmt"
	"net/http"
)

// EnclaveHealthCheck - status check handler for nitro enclave service
func EnclaveHealthCheck(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "OK\n")
}
