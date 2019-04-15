FROM golang:1.12 as builder

WORKDIR /src/bat-go/
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -o grant-server ./bin/grant-server

FROM alpine:3.6
RUN apk add --update ca-certificates # Certificates for SSL
COPY --from=builder /src/bat-go/grant-server /bin/
EXPOSE 3333
CMD ["/bin/grant-server"]
