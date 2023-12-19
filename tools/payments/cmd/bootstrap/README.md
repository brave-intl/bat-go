# Using Bootstrap

```bash
aws-vault exec <appropriate iam role> -- go run main.go -b <appropriate s3 bucket> -v \
    -p <private key file> -e <appropriate environment> -pcr2 <appropriate pcr2 value> \
    <encrypted shamir share file decryptable by -p flag>
```
