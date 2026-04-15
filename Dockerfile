FROM golang:1.25-alpine AS builder

# Put certs in builder image.
RUN apk update
RUN apk add -U --no-cache ca-certificates && update-ca-certificates
RUN apk add make build-base git bash

ARG VERSION
ARG BUILD_TIME
ARG COMMIT

WORKDIR /src
COPY . ./

RUN --mount=type=cache,target=/go/pkg/mod \
    cd main && go mod download && CGO_ENABLED=0 GOOS=linux GOTOOLCHAIN=local go build \
    -ldflags "-w -s -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.commit=${COMMIT}" \
    -o bat-go main.go

FROM alpine:3.23 AS base

# Put certs in artifact from builder.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /src/main/bat-go /bin/

FROM base AS artifact
COPY --from=builder /src/migrations/ /migrations/
USER nobody
EXPOSE 3333
CMD ["bat-go", "serve", "grant", "--enable-job-workers", "true"]
