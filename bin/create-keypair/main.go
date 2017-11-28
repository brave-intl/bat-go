package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"log"

	"golang.org/x/crypto/ed25519"
)

var envVarOut = flag.Bool("env", false, "Output in env var form [ENV=VALUE]")

func main() {
	flag.Parse()

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		log.Fatalln(err)
	}
	publicKeyHex := hex.EncodeToString([]byte(publicKey))
	if *envVarOut {
		fmt.Printf("export GRANT_SIGNATOR_PUBLIC_KEY=%s\n", publicKeyHex)
	} else {
		fmt.Println("publicKey: ", publicKeyHex)
	}

	privateKeyHex := hex.EncodeToString([]byte(privateKey))
	if *envVarOut {
		fmt.Printf("export GRANT_SIGNATOR_PRIVATE_KEY=%s\n", privateKeyHex)
	} else {
		fmt.Println("privateKey: ", privateKeyHex)
	}
}
