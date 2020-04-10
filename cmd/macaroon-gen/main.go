package main

import (
	"flag"
	"fmt"
	"os"
)

var (
	config = flag.String("config", "example.yaml", "file name describing tokens to create")
	secret = flag.String("secret", "mysecret", "the signing secret")
)

func init() {
	flag.Parse()
}

func main() {
	// new config
	var tc = new(TokenConfig)
	// parse config file
	if err := tc.Parse(*config); err != nil {
		fmt.Println("unable to parse token config: ", err)
		os.Exit(1)
	}

	for _, token := range tc.Tokens {
		// generate token
		t, err := token.Generate(*secret)
		if err != nil {
			fmt.Println("unable to generate token: ", err)
			continue
		}

		fmt.Printf("%s: \n%s\n\n", token.ID, t)
	}
}
