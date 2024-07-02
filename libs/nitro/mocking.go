package nitro

import "os"

var enclaveMocking = os.Getenv("NITRO_ENCLAVE_MOCKING") != ""

func EnclaveMocking() bool {
	return enclaveMocking
}
