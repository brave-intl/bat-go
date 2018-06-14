package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/brave-intl/bat-go/utils/pindialer"
)

func main() {
	log.SetFlags(0)

	flag.Usage = func() {
		log.Printf("A helper for fetching tls fingerprint info for pinning.\n\n")
		log.Printf("Usage:\n\n")
		log.Printf("        %s HOST:PORT...", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if len(flag.Args()) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	for _, address := range flag.Args() {
		fmt.Println("Dialing", address)
		c, err := tls.Dial("tcp", address, nil)
		if err != nil {
			log.Fatalln(err)
		}
		prints, err := pindialer.GetFingerprints(c)
		if err != nil {
			log.Fatalln(err)
		}
		for key, value := range prints {
			fmt.Println("Issuer:", key, "Fingerprint:", value)
		}
	}
}
