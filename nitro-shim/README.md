# Nitro Shim

This container definition will allow the nitro shim container to build
and run nitro images on an enclave.

## Build

docker build -t nitro-shim:latest .
docker tag nitro-shim:latest <account>.dkr.ecr.us-west-2.amazonaws.com/brave-intl/nitro-shim:latest
docker login ...
docker push <account>.dkr.ecr.us-west-2.amazonaws.com/brave-intl/nitro-shim:latest

## Usage

Inside Kubernetes you can specify the shim container to build and launch a given reproducible docker
image inside an enclave with the `build.sh` command shown below:

```bash
/enclave/build.sh <account>.dkr.ecr.us-west-2.amazonaws.com/brave-intl/bat-go/master:repro-<tag> run
```

The above command will build a `.eif` image using this docker image and run in an enclave
