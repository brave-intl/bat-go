package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"text/tabwriter"

	"github.com/brave-intl/bat-go/utils/vaultsigner"
	"golang.org/x/crypto/ssh/terminal"
)

func fprintf(w io.Writer, format string, a ...interface{}) {
	_, err := fmt.Fprintf(w, format, a...)
	if err != nil {
		panic(err)
	}
}

func main() {
	log.SetFlags(0)

	flag.Usage = func() {
		log.Printf("A helper for using pipe output to unseal vault.\n\n")
		log.Printf("Usage:\n\n")
		log.Printf("        gpg -d share-0.gpg | %s\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	client, err := vaultsigner.Connect()
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

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fprintf(w, "Key\tValue\n")
	fprintf(w, "---\t-----\n")
	fprintf(w, "Seal Type\t%s\n", status.Type)
	fprintf(w, "Sealed\t%t\n", status.Sealed)
	fprintf(w, "Total Shares\t%d\n", status.N)
	fprintf(w, "Threshold\t%d\n", status.T)

	if status.Sealed {
		fprintf(w, "Unseal Progress\t%d/%d\n", status.Progress, status.T)
		fprintf(w, "Unseal Nonce\t%s\n", status.Nonce)
	}
	err = w.Flush()
	if err != nil {
		log.Fatalln(err)
	}
}
