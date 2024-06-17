# Testing nitro payments

## The current way

There is no way to test the code locally. A docker build with changes has to be
built locally, pushed to the registry and deployed in the staging environment.
The procedure to do is the following:

1. Build the image:

        cd .../bat-go
        make docker-reproducible

2. Tag and push it (replace debug-my-test with a branch name or similar):

        docker tag bat-go-repro:latest 168134825737.dkr.ecr.us-west-2.amazonaws.com/brave-intl/bat-go/dev/payments:debug-my-test
        docker push 168134825737.dkr.ecr.us-west-2.amazonaws.com/brave-intl/bat-go/dev/payments:debug-my-test

3. In the payment-ops repo on the master branch replace the image tag in `sandbox/dev/patches/web-deployment.yaml` in two places, in the `command` item for the `name: nitro-shim` container and in the `image` item for the `nitro-utils` container.

4. Commit `sandbox/dev/patches/web-deployment.yaml` and push the master branch of `payment-ops`.

5. Wait until Kubernetes deploys the payment using the following command:

         watch -n5 kubectl --context=bsg-production --namespace payment-staging get pods

6. Run the ipython shell and execute the bootstrap commands:

        aws-vault exec settlements-stg-developer-role -- ipython --profile-dir=ipython-profile
        ...
        %bootstrap ...

7. Test the service.

## Requirements for test environment for development

The above is very time-consuming and require quite a few manual actions. In
theory all the steps can be automated reducing the above sequence to a single
command, but still it will be time-consuming and prevents testing in parallel.
Thus we are in need for a better testing environment.

### Goals

1. Quick testing after recompiling of a Go executable. In particular there
   should be a single command that prepares the testing environment within
   seconds after the code is compiled.
    - There should be an option to skip bootstrapping and authorization phases.

2. Independent testing by several developers. Running the tests by one developer
   should not affect the tests by another.

3. Arm CPU. It should be possible to test on Arm Mac.

### Non-goals

1. Fully offline testing. In particular, the tests may use cloud services and
   the need to maintain Brave VPN connection.

2. Support for debuggers. As long as recompiling/restart is fast, logging and
   the state of databases should be enough to debug the problems.

3. Quick setup. It is OK if to setup the testing environment for new developer
   it should be necessary to interact with IT and to follow a guide/guides.

4. Native Windows support. On Windows platform the developer is expected to use
   WSL (Windows subsystem for Linux) so the test setup and scripts can assume a
   POSIX-compatible system.

5. Resemblance to the production environment. It is not necessary to provide the
   same experience of working with production Kubernetes cluster during
   development testing.

## Suggested setup

To satisfy the goals above I suggest to use Docker Compose to run the worker,
the service, the redis database and the local S3 provided by LocalStack while
continuing to use QLDB from AWS cloud with each developer having access to
independent database instance.

Realistically if one wants to run QLDB locally, then the only option is to use
[LocalStack](https://docs.localstack.cloud/user-guide/aws/qldb/). As their QLDB
compatibility layer is a paid option that requires per-developer license and the
amount of data during testing is not going to be significant to incur
non-trivial cost increase, we can continue to use QLDB and avoid dealing with
LocalStack licensing.

An alternative for Docker Compose is to use minikube and its local Kubernetes
cluster. Although it will bring the testing environment closer to the
production, setting up and maintaining minikube setup requires much more efforts
compared with Docker Compose. For example, transferring a new locally built
Docker image requires tagging and pushing into the local minikube registry.
Also, it will not be possible to re-use the service definitions

To support the option of avoiding the need for bootstrap and authorization after
deploying a new executable an extra service will be provided that feeds the
necessary keys on restart.

### Question:

- Is it really OK to use Docker compose or minikube is preferable long-term as
  it brings the local testing environment closer to production?

- The current setup uses NGINX as SSL proxy for REDIS DB. Is it necessary in the
  test environment?

- What to do with the ipython shell? Shall it gain a mode to work in the test
  environment which uses the same commands but connects to docker compose? An
  alternative is to provide separated test commands..

- Should the setup include a compilation support so a containers can be used to
  compile the executable which then will be used in another container? This
  avoids the need to recompile the Docker image so a restart of the container is
  enough to switch to the new executable. However, this further depart the dev
  setup from the production.
