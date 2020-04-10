package main

import (
	"flag"
	"fmt"
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
	tc.Parse(*config)

	fmt.Printf("tc: %+v\n", tc)

	for _, token := range tc.Tokens {
		// generate token
		t, err := token.Generate(*secret)
		if err != nil {
			fmt.Println("unable to generate token: ", err)
			continue
		}

		fmt.Printf("%s: %s\n", token.ID, t)
	}
}
