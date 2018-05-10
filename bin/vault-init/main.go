package main

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path"

	"github.com/brave-intl/bat-go/utils"
	"github.com/hashicorp/vault/api"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/packet"
)

var secretShares = flag.Uint("key-shares", 5, "number of total unseal shares")
var secretThreshold = flag.Uint("key-threshold", 3, "number of shares needed to unseal")

func main() {
	log.SetFlags(0)

	flag.Usage = func() {
		log.Printf("A helper for quickly initializing vault.\n\n")
		log.Printf("Usage:\n\n")
		log.Printf("        %s GPG_PUB_KEY_FILE...\n\n", os.Args[0])
		log.Printf("  %s initializes vault and writes the encrypted unseal shares to disk.\n", os.Args[0])
		log.Printf("  Each share is written as a separate sequental file. Furthermore the initial root\n")
		log.Printf("  token is saved to ~/.vault-token, meaning the initial `vault login` can be skipped.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	gpgKeyFiles := flag.Args()

	if len(gpgKeyFiles) == 0 {
		flag.Usage()
		os.Exit(1)
	} else if len(gpgKeyFiles) != int(*secretShares) {
		log.Printf("ERROR: A gpg public key file must be passed for every unseal share\n\n")
		flag.Usage()
		os.Exit(1)
	}

	var entityList openpgp.EntityList
	gpgKeys := []string{}

	for i := 0; i < len(gpgKeyFiles); i++ {
		f, err := os.Open(gpgKeyFiles[i])
		if err != nil {
			log.Fatalln(err)
		}
		defer utils.PanicCloser(f)

		// Vault only accepts keys in binary format, so we normalize the format
		var entity openpgp.EntityList

		// Try to read the input file in armored format
		entity, err = openpgp.ReadArmoredKeyRing(f)
		if err != nil {
			// On failure try to read it in binary format
			_, err = f.Seek(0, 0)
			if err != nil {
				log.Fatalln(err)
			}
			entity, err = openpgp.ReadKeyRing(f)
			if err != nil {
				log.Fatalln(err)
			}
		}
		if len(entity) > 1 {
			log.Fatalln("Your gpg public key files should only contain a single public key")
		}

		buf := new(bytes.Buffer)
		err = entity[0].Serialize(buf)
		if err != nil {
			log.Fatalln(err)
		}
		entityList = append(entityList, entity[0])
		gpgKeys = append(gpgKeys, base64.StdEncoding.EncodeToString(buf.Bytes()))
	}

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

	req := api.InitRequest{}

	req.PGPKeys = gpgKeys
	req.SecretShares = int(*secretShares)
	req.SecretThreshold = int(*secretThreshold)

	resp, err := client.Sys().Init(&req)
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Printf("Success, vault has been initialized\n\n")

	var b []byte
	for i := range resp.KeysB64 {
		b, err = base64.StdEncoding.DecodeString(resp.KeysB64[i])
		if err != nil {
			log.Fatalln(err)
		}

		// Parse the resulting encrypted files to print corresponding key for each
		buf := bytes.NewBuffer(b)
		packets := packet.NewReader(buf)
		var p packet.Packet
		for {
			p, err = packets.Next()
			if err != nil {
				break
			}
			switch p := p.(type) {
			case *packet.EncryptedKey:
				keys := entityList.KeysById(p.KeyId)
				if len(keys) == 1 {
					for k := range keys[0].Entity.Identities {
						fmt.Printf("Writing share-%d.gpg for %s\n", i, k)
					}
				}
			}
		}

		err = ioutil.WriteFile(fmt.Sprintf("share-%d.gpg", i), b, 0600)
		if err != nil {
			log.Fatalln(err)
		}
	}

	usr, err := user.Current()
	if err != nil {
		log.Fatalln(err)
	}

	err = ioutil.WriteFile(path.Join(usr.HomeDir, ".vault-token"), []byte(resp.RootToken), 0600)
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println("Done! Note that the root token has been written to ~/.vault-token")
}
