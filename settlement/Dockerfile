FROM golang:1.15 as builder

ARG VERSION
ARG BUILD_TIME
ARG COMMIT

WORKDIR /src/
COPY go.mod go.sum ./
RUN go mod download
COPY . ./

# bat-go cli tool
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-w -s -X cmd.version=${VERSION} -X cmd.buildTime=${BUILD_TIME} -X cmd.commit=${COMMIT}" \
    -o bat-go main.go

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-w -s -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.commit=${COMMIT}" \
    -o settlement-submit ./bin/settlement-submit

# the artifact settlement-tools image
FROM alpine:3.6 as artifact
RUN apk add --update ca-certificates # Certificates for SSL
COPY --from=builder /src/bat-go /bin/
COPY --from=builder /src/settlement-submit/ /bin/
CMD ["./bat-go"]
