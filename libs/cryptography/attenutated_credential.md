# Creation of Merchant Attenuated Credentials

Below is the process for creation of merchant credentials for merchant HTTP Signatures,
primarily useful for the SKUs credential validation endpoint.

1. [ ] create merchant shared key
```
    b := make([]byte, 24)
    rand.Read(b)
    secretPlaintext := "secret-token:" + base64.RawURLEncoding.EncodeToString(b)
```
2. [ ] encrypt merchant shared key
```
	var byteEncryptionKey [32]byte
	copy(byteEncryptionKey[:], []byte(os.Getenv("ENCRYPTION_KEY"))) // the encryption key set for the environment
	encryptedBytes, nonce, err := EncryptMessage(byteEncryptionKey, []byte(secretPlaintext))
```
3. [ ] upload merchant encrypted shared key and secret to the SKUs database `api_keys` table
```
    insert into api_keys (name, merchant_id, encrypted_secret_key, nonce) values
        (
            'Name of Merchant Key', 'Merchant ID (i.e. "brave.com"),
            string(encryptedBytes), string(nonce)
        );
    -- save key id created for next step
```
4. [ ] attenuate merchant key
```
    keyID := // the id from the db insert to the api_keys table
    secretKey := secretPlaintext // from the first step, the generated random secret

    caveats := map[string]string {
        "merchant": "brave.com", // brave.com in this case is the "merchant_id" in the db from prior step
    }

	keyID, secretKey, err := Attenuate(keyID, secretKey, caveats)
```
5. [ ] securely deliver attenuated keyID/keySecret to merchant
```
    The caller will need keyID and secretKey from the last step, deliver them securely
```
