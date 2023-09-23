/*
Validate performs validation of the attested report provided, and performs checks against the original
report as well.

Validate will take attested report and prepared report from command line and it is expecting valid JSON serialized transactions.

Usage:

validate [flags]

The flags are:

	-v
		verbose logging enabled
	-ar
		Location on file system of the attested transaction report for signing
	-pr
		Location on file system of the original prepared report
*/
package main

import (
	"flag"
	"log"
	"os"

	"github.com/brave-intl/bat-go/tools/payments"
)

func main() {
	// command line flags
	key := flag.String(
		"k", "test/private.pem",
		"the operator's key file location (ed25519 private key) in PEM format")

	attestedReportFilename := flag.String(
		"ar", "",
		"the location on disk of the attested report")

	preparedReportFilename := flag.String(
		"pr", "",
		"the location on disk of the original payout report")

	verbose := flag.Bool(
		"v", false,
		"view verbose logging")

	flag.Parse()

	if *verbose {
		// print out the configuration
		log.Printf("Operator Key File Location: %s\n", *key)
	}

	// FIXME verify attesation as we pull responses off the stream

	attestedReportFile, err := os.Open(*attestedReportFilename)
	if err != nil {
		log.Fatalf("failed to open attested report file: %v\n", err)
	}
	defer attestedReportFile.Close()

	// parse the attested report
	attestedReport := payments.AttestedReport{}
	if err := payments.ReadReport(&attestedReport, attestedReportFile); err != nil {
		log.Fatalf("failed to read attested report: %v\n", err)
	}

	preparedReportFile, err := os.Open(*preparedReportFilename)
	if err != nil {
		log.Fatalf("failed to open prepared report file: %v\n", err)
	}
	defer preparedReportFile.Close()

	// parse the original prepared report
	preparedReport := payments.PreparedReport{}
	if err := payments.ReadReport(&preparedReport, preparedReportFile); err != nil {
		log.Fatalf("failed to read prepared report: %v\n", err)
	}

	if *verbose {
		log.Printf("attested report stats: %d transactions; %s total bat\n",
			len(attestedReport), attestedReport.SumBAT())
		log.Printf("prepared report stats: %d transactions; %s total bat\n",
			len(preparedReport), preparedReport.SumBAT())
	}

	// compare performs automated checks to validate reports
	if err := payments.Compare(preparedReport, attestedReport); err != nil {
		log.Fatalf("failed to compare reports: %v\n", err)
	}

	if *verbose {
		log.Println("completed report validation")
	}
}
