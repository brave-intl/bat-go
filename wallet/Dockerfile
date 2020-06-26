FROM golang:1.14.4 as builder

ARG VERSION
ARG BUILD_TIME
ARG COMMIT

WORKDIR /src/
COPY go.mod go.sum ./
RUN go mod download
COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-w -s -X cmd.version=${VERSION} -X cmd.buildTime=${BUILD_TIME} -X cmd.commit=${COMMIT}" \
    -o bat-go main.go

FROM alpine:3.6 as artifact
RUN apk add --update ca-certificates # Certificates for SSL
COPY --from=builder /src/bat-go /bin/
EXPOSE 3333
CMD ["/bin/bat-go", "serve", "wallet", "rest"]
