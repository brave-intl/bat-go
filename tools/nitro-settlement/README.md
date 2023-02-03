# Settlement CLI

The settlement CLI tooling allows settlement operators to enqueue
validate, and authorize custodian transactions.

### Setup Redis Locally
```bash
docker-compose -f docker-compose.redis.yml up -d # to start up the local redis cluster
```

## Commands

Available commands are:

1. `prepare`
2. `bootstrap`
3. `authorize`
3. `validate`

### Prepare

The prepare command parses the payout report, and enqueues the transactions in 
a new per payout stream identified by `--payout-id` parameter.  Tool uses redis streams
to connect to `--redis-addrs` and `--redis-user` as well as `REDIS_PASS` env variable
to connect to redis to submit transactions for preparation.

```bash
REDIS_PASS= go run main.go prepare --report combined.json --payout-id 20230202_1 \
    --redis-addrs 127.0.0.1:6380,127.0.0.1:6383,127.0.0.1:6381,127.0.0.1:6382 --redis-user redis
```



