# Key Management policy for managing key list to authorize BAT rewards transactions

This is a minimal key management policy for managing keys used to perform various roles for the Payments service in order to automate the monthly brave rewards payments to users. These authorization keys are used by members of the Brave team who perform various roles (right now we have a single role of operator) in order to manage the service and generate and approve the batches of transactions to be processed. The keys are used to submit a REST call to the payments service signed by these keys using HTTP Signatures. In the payments service we compile with the code a Map of the authorized keys so that we can be certain that the keys have not been modified and these integrity checks are being done via a nitro enclave deployment.

## Currently active authorization keys

| Github Username | email | Public Key                              | Environment | 
|-------------------------|---------|----------------------------------------|------------------
| Sneagan | jegan+settlements@brave.com | ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDfcr9jUEu9D9lSpUnPwT1cCggCe48kZw1bJt+CXYSnh | Staging |
| jeremytieman | jtieman+nitro@brave.com | ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIK1fxpURIUAJNRqosAnPPXnKjpUBGGOKgkUOXmviJfFx | Staging |
| evq |evq+settlements@brave.com | ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIA91/jZI+hcisdAURdqgdAKyetA4b2mVJIypfEtTyXW+ | Staging |
| Sneagan | jegan+settlements@brave.com | ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMKhViUd6Nwd8qre0go7Qc6Wa6Q7A3GiWj7q/GMF/NzV | Dev |
| kdenhartog | kdenhartog+settlement+dev@brave.com | ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIEY/3VGKsrH5dp3mK5PJIHVkUMWpsmUhZkrLuZTf7Sqr | Dev |

- This table should match the keys [currently active](../operators.go) in the codebase so we can track who owns which key.

## Key Management Policies

### Key Creation Procedure

- Generate key locally
- Store the key securely. See the [Key Storage](#key-storage) section for additional details
- Set reminder in calendar around expiration date to update key
- Submit a PR to add the new key to the codebase and the table above

#### Key Generation command

`ssh-keygen -t ed25519 -C "example+settlement+dev@brave.com"`

#### Key storage

Keep the key either in a hardware security module like a Yubikey if it can be easily integrated to conduct HTTP Signatures. Otherwise this private key should stored in a keychain or on a separate USB drive which is not accessible via the network when not in use and encrypted with a strong password. Given the ease of updating this key via Github, we shouldn't plan to back this key up as it's easier to update the key out than to manage backups.

#### Key Distribution

- Keys shouldn't be distributed we should just create new keys and add them to the repository here instead.

### Key Update Procedure

- Generate a new key using the [Key Creation Procedure](#key-creation-procedure)
- Submit a PR to remove the old key from the codebase and add the key one.
    - Also update the table in this document to accurately reflect the removal so we know who is managing each key

##### Key update because of compromise

If the reason for a key update event is being conducted is because it's presumed to be compromised we should immediately update the static codebase to remove the key. We should then begin the redeployment process to remove the ability for that key to be used for any additional batch transactions. In this case because the authorization endpoint is easy to update and the key generation process is easy to conduct as well, we should not wait to update the key and instead should immediately follow the [Key update procedure](#key-update-procedure) above. Additionally, an incident response should be conducted to review the logs and identify if the compromised key has been used and to determine a remediation plan. This incident response should include at least someone from the payment-ops team, someone from the security team, and someone from the infrastructure team.

##### Key update because of expiration

On the date when a key has hit it's expiration period, the key should follow the [key update procedure](#key-update-procedure) highlighted above.

### Key Removal Procedure

In the event that a key needs to be removed such as when a member of the payment-ops team is being off-boarded from the company, the following procedure should occur.

- Submit a PR to remove the key from the codebase and from the table above