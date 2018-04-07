FROM golang:1.10 as builder
RUN curl -fsSL -o /usr/local/bin/dep https://github.com/golang/dep/releases/download/v0.4.1/dep-linux-amd64 && chmod +x /usr/local/bin/dep
WORKDIR /go/src/github.com/brave-intl/bat-go/
COPY Gopkg.toml Gopkg.lock ./
RUN dep ensure -vendor-only
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -o grant-server ./bin/grant-server

FROM alpine:3.6
RUN apk add --update ca-certificates # Certificates for SSL
COPY --from=builder /go/src/github.com/brave-intl/bat-go/grant-server /bin/
EXPOSE 3333
CMD ["/bin/grant-server"]
