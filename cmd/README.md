Below is the command structure for bat-go microservices using cobra

```
./bat-go
  - serve
    - rewards
      - rest
        example:
            serve rewards rest \
              --ratios-token "abc" --ratios-service "123" --environment "local" \
              --base-currency "USD" --address ":4321"
      - grpc
        example:
            serve rewards grpc \
              --ratios-token "abc" --ratios-service "123" --environment "local" \
              --base-currency "USD" --address ":4321"
  - settlement
    - paypal
      - transform
      - transform
      - email
```
