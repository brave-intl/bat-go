
# SKU Service

### Getting Started

1. Begin with development setup steps 1 through 4 from the [bat-go readme](https://github.com/brave-intl/bat-go/blob/master/README.md)

2. To bring up an environment to exercise the API run `make docker-up-dev`. Once the containers have started, the API will be available at `localhost:3333`. `curl localhost:3333/health-check` will provide a health check


### Development cycle

1. Optionally once the steps are complete run `export TEST_TAGS=integration; make docker-test` to make sure everything builds and all the tests pass

2. Write code and run unit tests locally, no need to bring up the environment for example, to run a unit test in SKUs navigate to the skus directory and run `go test -run TestService_uniqBatchesTxTime`

3. Write integration tests then bring up environment using `make docker-dev` at the command prompt run the specific test you have written for example,

    ```
    cd services && export GODEBUG=x509ignoreCN=0; go test -count=1 -tags integration -timeout 1m -v -run ControllersTestSuite/TestWebhook_Radom ./skus/...
    ```

4. Optionally before pushing code for review run `export TEST_TAGS=integration; make docker-test` with all new code and make sure it will pass when it hits CI

5. Commit and push
