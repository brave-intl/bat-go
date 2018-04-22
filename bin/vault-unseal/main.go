package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"text/tabwriter"

	"github.com/hashicorp/vault/api"
	"golang.org/x/crypto/ssh/terminal"
)

func main() {
	log.SetFlags(0)

	/* #nosec */
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "A helper for using pipe output to unseal vault.\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n\n")
		fmt.Fprintf(os.Stderr, "        gpg -d share-0.gpg | %s\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	config := &api.Config{}
	err := config.ReadEnvironment()

	var client *api.Client
	if err != nil {
		client, err = api.NewClient(config)
	} else {
		client, err = api.NewClient(nil)
		if err != nil {
			log.Fatalln(err)
		}
		err = client.SetAddress("http://127.0.0.1:8200")
	}
	if err != nil {
		log.Fatalln(err)
	}

	fi, err := os.Stdin.Stat()
	if err != nil {
		log.Fatalln(err)
	}

	var b []byte

	if (fi.Mode() & os.ModeNamedPipe) == 0 {
		fmt.Print("Please enter your unseal key: ")
		b, err = terminal.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
	} else {
		reader := bufio.NewReader(os.Stdin)
		b, err = ioutil.ReadAll(reader)
	}
	if err != nil {
		log.Fatalln(err)
	}

	status, err := client.Sys().Unseal(string(b))
	if err != nil {
		log.Fatalln(err)
	}

	/* #nosec */
	{
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
		fmt.Fprintln(w, "Key\tValue")
		fmt.Fprintln(w, "---\t-----")
		fmt.Fprintf(w, "Seal Type\t%s\n", status.Type)
		fmt.Fprintf(w, "Sealed\t%t\n", status.Sealed)
		fmt.Fprintf(w, "Total Shares\t%d\n", status.N)
		fmt.Fprintf(w, "Threshold\t%d\n", status.T)

		if status.Sealed {
			fmt.Fprintf(w, "Unseal Progress\t%d/%d\n", status.Progress, status.T)
			fmt.Fprintf(w, "Unseal Nonce\t%s\n", status.Nonce)
		}
		err = w.Flush()
		if err != nil {
			log.Fatalln(err)
		}
	}
}
