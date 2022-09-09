package vault

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path"

	rootcmd "github.com/brave-intl/bat-go/cmd"

	cmdutils "github.com/brave-intl/bat-go/cmd"

	"github.com/brave-intl/bat-go/libs/closers"
	appctx "github.com/brave-intl/bat-go/libs/context"
	vaultsigner "github.com/brave-intl/bat-go/tools/vault/signer"
	"github.com/hashicorp/vault/api"
	"github.com/keybase/go-crypto/openpgp"
	"github.com/keybase/go-crypto/openpgp/packet"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// InitCmd initializes the vault server
	InitCmd = &cobra.Command{
		Use:   "init",
		Short: "initializes the vault server",
		Run:   rootcmd.Perform("init vault", Initialize),
	}
)

func init() {
	VaultCmd.AddCommand(
		InitCmd,
	)

	initBuilder := cmdutils.NewFlagBuilder(InitCmd)

	// key-shares -> the number of shares to split the master key into: default 5
	initBuilder.Flag().Uint("key-shares", 5,
		"the number of shares to split the master key into").
		Bind("key-shares")

	// key-threshold -> the number of shares needed to unseal: default 3
	initBuilder.Flag().Uint("key-threshold", 3,
		"number of shares needed to unseal").
		Bind("key-threshold")
}

// Initialize initializes the vault server
func Initialize(command *cobra.Command, args []string) error {
	gpgKeyFiles := args
	secretShares := viper.GetUint("key-shares")
	secretThreshold := viper.GetUint("key-threshold")
	logger, err := appctx.GetLogger(command.Context())
	cmdutils.Must(err)

	if len(gpgKeyFiles) == 0 {
		return errors.New("a gpg file was not passed")
	} else if len(gpgKeyFiles) != int(secretShares) {
		return errors.New("a gpg public key file must be passed for every unseal share")
	}

	var entityList openpgp.EntityList
	gpgKeys := []string{}

	for i := 0; i < len(gpgKeyFiles); i++ {
		f, err := os.Open(gpgKeyFiles[i])
		if err != nil {
			return err
		}
		defer closers.Panic(command.Context(), f)

		// Vault only accepts keys in binary format, so we normalize the format
		var entity openpgp.EntityList

		// Try to read the input file in armored format
		entity, err = openpgp.ReadArmoredKeyRing(f)
		if err != nil {
			// On failure try to read it in binary format
			_, err = f.Seek(0, 0)
			if err != nil {
				return err
			}
			entity, err = openpgp.ReadKeyRing(f)
			if err != nil {
				return err
			}
		}
		if len(entity) > 1 {
			return errors.New("your gpg public key files should only contain a single public key")
		}

		buf := new(bytes.Buffer)
		err = entity[0].Serialize(buf)
		if err != nil {
			return err
		}
		entityList = append(entityList, entity[0])
		gpgKeys = append(gpgKeys, base64.StdEncoding.EncodeToString(buf.Bytes()))
	}

	wrappedClient, err := vaultsigner.Connect()
	if err != nil {
		return err
	}

	req := api.InitRequest{}

	req.PGPKeys = gpgKeys
	req.SecretShares = int(secretShares)
	req.SecretThreshold = int(secretThreshold)

	if secretShares > 1 && secretThreshold == 1 {
		// Vault does not support this case but we can workaround and encrypt the single share to all keys
		req.PGPKeys = []string{}
		req.SecretShares = 1
	}

	resp, err := wrappedClient.Client.Sys().Init(&req)
	if err != nil {
		return err
	}

	logger.Info().Msg("success, vault has been initialized")

	if secretShares > 1 && secretThreshold == 1 {
		// We need to encrypt the single returned share to all keys ourselves
		key := resp.Keys[0]

		logger.Info().Msgf("Writing share-0.gpg for all identities\n")
		out, err := os.OpenFile("share-0.gpg", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}
		defer closers.Panic(command.Context(), out)

		encOut, err := openpgp.Encrypt(out, entityList, nil, &openpgp.FileHints{IsBinary: true}, nil)
		if err != nil {
			return err
		}
		defer closers.Panic(command.Context(), encOut)

		_, err = encOut.Write([]byte(key))
		if err != nil {
			return err
		}
	} else {
		// Vault has encrypted the shares for us
		var b []byte
		for i := range resp.KeysB64 {
			b, err = base64.StdEncoding.DecodeString(resp.KeysB64[i])
			if err != nil {
				return err
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
					keys := entityList.KeysById(p.KeyId, nil)
					if len(keys) == 1 {
						for k := range keys[0].Entity.Identities {
							logger.Info().Msgf("Writing share-%d.gpg for %s\n", i, k)
						}
					}
				}
			}

			err = ioutil.WriteFile(fmt.Sprintf("share-%d.gpg", i), b, 0600)
			if err != nil {
				return err
			}
		}
	}

	usr, err := user.Current()
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(path.Join(usr.HomeDir, ".vault-token"), []byte(resp.RootToken), 0600)
	if err != nil {
		return err
	}

	logger.Info().Msg("Done! Note that the root token has been written to ~/.vault-token")
	return nil
}
