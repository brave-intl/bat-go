package payments

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TODO stronger assertion
func TestReadReport(t *testing.T) {
	f, err := os.Open("./test/attested-report-s3-download")
	assert.NoError(t, err)

	var report AttestedReport
	err = ReadAttestedReport(&report, f)
	assert.NoError(t, err)
}
