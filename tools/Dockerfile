FROM golang:1.25-alpine as builder

# put certs in builder image
RUN apk update
RUN apk add -U --no-cache ca-certificates && update-ca-certificates
RUN apk add make
RUN apk add build-base
RUN apk add git
RUN apk add bash

ARG VERSION
ARG BUILD_TIME
ARG COMMIT

WORKDIR /src/
COPY . ./
RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux GOTOOLCHAIN=local go build \
    -ldflags "-w -s -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.commit=${COMMIT}" \
    -o bat-go main.go

FROM alpine:3.22 as base
# put certs in artifact from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /src/bat-go /bin/

FROM base as artifact
COPY --from=builder /src/migrations/ /migrations/
EXPOSE 3333
CMD ["bat-go", "serve", "grant", "--enable-job-workers", "true"]
