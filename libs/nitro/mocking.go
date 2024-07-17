package nitro

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

var enclaveMocking = os.Getenv("NITRO_ENCLAVE_MOCKING") != ""

func EnclaveMocking() bool {
	return enclaveMocking
}

func mockTestPCR(pcrIndex int) []byte {
	// Create arbitrary but easily recognizable values in hex
	if pcrIndex < 0 || pcrIndex > 8 {
		panic("Invalid mocking PCR index")
	}
	testHex := fmt.Sprintf("abc%d", pcrIndex)
	testHex = strings.Repeat(testHex, PCRByteLength*2/4)
	pcr, err := hex.DecodeString(testHex)
	if err != nil {
		panic(err)
	}
	return pcr
}

func ReadMockingSecretsFile(fileName string) string {
	dir := "payment-test/secrets/"
	bytes, err := os.ReadFile(dir + fileName)
	if err != nil {
		panic(err)
	}
	return string(bytes)
}
