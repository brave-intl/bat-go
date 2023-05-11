# Creation of Merchant Attenuated Keys

In order to create attenuated merchant keys run the command below replacing the encryption-key and
attenuation parameters with valid inputs based on the merchant and environment where these values
will be used.

```bash
go run main.go merchant create-key --encryption-key lol --attenuation '{"merchant":"brave.com"}'
9:01AM INF encrypted secrets for insertion into database encryptedMerchantNonce=6b297ded477dad5a628d3f2b042eb7a2d415c7e60fa8617f encryptedMerchantSecret=ce16d9539acdd1f5b3767ace17a63032c3b8a4505b22d3cf917b3bdc3db38c329f21bea46fae6dbbb2a601325d4d5ded48f36559ffd619e261a57e4bc6 keyID=ce16d9539acdd1f5b3767ace17a63032c3b8a4505b22d3cf917b3bdc3db38c329f21bea46fae6dbbb2a601325d4d5ded48f36559ffd619e261a57e4bc6
9:01AM INF attenuated merchant keys aKeyID=5fc33254-a4f8-4424-8186-8d8e1787e00a:eyJtZXJjaGFudCI6ImJyYXZlLmNvbSJ9 aKeySecret=secret-token:wjOtYCQypY5ky1AM_co1lTXNJdOe3Q_waNnnfdyl5u3eOKHCKL-galY9Wklf merchantSecret=secret-token:a0Ojj4_TBtX_aXSowKByJ-NVND9kf2t4
```

In the result-set you will see the attenuated merchant `aKeyID` and `aKeySecret` which need to be securely
delivered to the merchant.  You will also see the `encryptedMerchantSecret` and `encryptedMerchantNonce` and
`keyID` which need to be inserted into the environment's database `api_keys` table.


