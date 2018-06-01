FROM golang:1.10 as builder

ENV DEP_VERSION 0.4.1
ENV DEP_SHA256SUM 31144e465e52ffbc0035248a10ddea61a09bf28b00784fd3fdd9882c8cbb2315

RUN curl -fsSL -o /usr/local/bin/dep https://github.com/golang/dep/releases/download/v$DEP_VERSION/dep-linux-amd64
RUN echo "$DEP_SHA256SUM  /usr/local/bin/dep" | shasum -a 256 -c
RUN chmod +x /usr/local/bin/dep
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
