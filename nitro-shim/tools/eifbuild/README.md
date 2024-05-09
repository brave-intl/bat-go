# pre-requisites

build and install eif_build ( requires local rust toolchain. )
```
cargo install --git https://github.com/aws/aws-nitro-enclaves-image-format --example eif_build
```

download the aws-nitro-enclaves-cli repo ( contains binary blobs needed to build eif images. )
```
mkdir -p third_party && git clone https://github.com/aws/aws-nitro-enclaves-cli --depth 1 third_party/aws-nitro-enclaves-cli
```

build eifbuild.
```
make
```

# reproducing PCR values for a particular docker image

```
# set any environment variables that will be passed to the enclave
# export FOO="BAR"
# ...

# also set EIF_PASS_ENV and EIF_COMMAND to the same value as defined in the pod
# export EIF_PASS_ENV="FOO"
# export EIF_COMMAND="BAZ"

./eifbuild -pass-env $PASS_ENV -docker-uri 168134825737.dkr.ecr.us-west-2.amazonaws.com/brave-intl/bat-go/dev/payments:v1.0.3-434-g06a3557d-dirty1696629608 -output-file test.eif -blobs-path ./third_party/aws-nitro-enclaves-cli/blobs/x86_64 -- $EIF_COMMAND
```
